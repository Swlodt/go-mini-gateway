package server

import (
	"context"
	"errors"
	"fmt"
	"go-mini-gateway/internal/concurrency"
	"go-mini-gateway/internal/config"
	"go-mini-gateway/internal/health"
	"go-mini-gateway/internal/metrics"
	"go-mini-gateway/internal/proxy"
	"go-mini-gateway/internal/ratelimit"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Server struct {
	mu sync.RWMutex

	httpServer      *http.Server
	adminHTTPServer *http.Server
	mainHandler     *reloadableHandler
	configPath      string
	currentConfig   *config.Config

	routes                   []*routeRuntime
	proxyHandlers            []*proxy.Handler
	rateLimiters             []*ratelimit.TokenBucket
	healthCheckers           []*health.Checker
	globalRateLimiter        *ratelimit.TokenBucket
	globalConcurrencyLimiter *concurrency.Limiter
	metricsRegistry          *metrics.Registry

	adminEnabled        bool
	adminToken          string
	metricsRequireToken bool
	pprofEnabled        bool
}

type httpServerTarget struct {
	name   string
	server *http.Server
}

type runtimeBundle struct {
	handler                  http.Handler
	routes                   []*routeRuntime
	proxyHandlers            []*proxy.Handler
	rateLimiters             []*ratelimit.TokenBucket
	healthCheckers           []*health.Checker
	globalRateLimiter        *ratelimit.TokenBucket
	globalConcurrencyLimiter *concurrency.Limiter
}

type routeRegisterResult struct {
	routes         []*routeRuntime
	proxyHandlers  []*proxy.Handler
	rateLimiters   []*ratelimit.TokenBucket
	healthCheckers []*health.Checker
}

type routeRuntime struct {
	id             string
	prefix         string
	stripPrefix    string
	target         string
	upstreams      []proxy.UpstreamSnapshot
	rateLimitRPS   int
	rateLimitBurst int
	maxConcurrency int

	healthCheckEnabled bool
	healthCheckPath    string

	proxyHandler       *proxy.Handler
	rateLimiter        *ratelimit.TokenBucket
	concurrencyLimiter *concurrency.Limiter
	healthChecker      *health.Checker
}

func New(cfg *config.Config) (*Server, error) {
	return NewWithConfigPath(cfg, "")
}

func NewWithConfigPath(cfg *config.Config, configPath string) (*Server, error) {
	metricsRegistry := metrics.NewRegistry()

	runtime, err := buildRuntime(cfg, metricsRegistry)
	if err != nil {
		return nil, err
	}

	reloadableMainHandler := newReloadableHandler(runtime.handler)

	srv := &Server{
		mainHandler:              reloadableMainHandler,
		configPath:               configPath,
		currentConfig:            cfg,
		routes:                   runtime.routes,
		proxyHandlers:            runtime.proxyHandlers,
		rateLimiters:             runtime.rateLimiters,
		healthCheckers:           runtime.healthCheckers,
		globalRateLimiter:        runtime.globalRateLimiter,
		globalConcurrencyLimiter: runtime.globalConcurrencyLimiter,
		metricsRegistry:          metricsRegistry,

		adminEnabled:        cfg.Admin.Enabled,
		adminToken:          cfg.Admin.Token,
		metricsRequireToken: cfg.Admin.MetricsRequireToken,
		pprofEnabled:        cfg.Admin.PprofEnabled,
	}

	srv.httpServer = &http.Server{
		Addr:              cfg.Addr(),
		Handler:           reloadableMainHandler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	if cfg.Admin.Enabled {
		adminMux := http.NewServeMux()
		srv.registerAdminRoutes(adminMux)

		var adminHandler http.Handler = adminMux

		// 管理接口不走业务全局限流、不走业务全局并发限制。
		// 但仍然记录访问日志和 metrics。
		adminHandler = accessLogMiddleware(adminHandler, metricsRegistry)

		srv.adminHTTPServer = &http.Server{
			Addr:              cfg.Admin.Addr,
			Handler:           adminHandler,
			ReadHeaderTimeout: 5 * time.Second,
		}
	}

	return srv, nil
}

func buildRuntime(cfg *config.Config, metricsRegistry *metrics.Registry) (*runtimeBundle, error) {
	requestTimeout, err := cfg.RequestTimeoutDuration()
	if err != nil {
		return nil, err
	}

	mainMux := http.NewServeMux()

	mainMux.HandleFunc("/ping", handlePing)
	mainMux.HandleFunc("/health", handleHealth)
	mainMux.HandleFunc("/version", handleVersion)

	routeResult, err := registerRoutes(mainMux, cfg.Routes)
	if err != nil {
		return nil, err
	}

	var mainHandler http.Handler = mainMux

	mainHandler = timeoutMiddleware(requestTimeout)(mainHandler)

	var globalConcurrencyLimiter *concurrency.Limiter
	var globalRateLimiter *ratelimit.TokenBucket

	if cfg.Server.MaxConcurrency > 0 {
		globalConcurrencyLimiter = concurrency.NewLimiter(
			"global",
			cfg.Server.MaxConcurrency,
		)

		mainHandler = concurrency.Middleware("global", globalConcurrencyLimiter)(mainHandler)
	}

	if cfg.Server.RateLimitRPS > 0 {
		globalRateLimiter = ratelimit.NewTokenBucket(
			"global",
			cfg.Server.RateLimitRPS,
			cfg.Server.RateLimitBurst,
		)

		routeResult.rateLimiters = append(routeResult.rateLimiters, globalRateLimiter)

		mainHandler = ratelimit.Middleware("global", globalRateLimiter)(mainHandler)
	}

	mainHandler = accessLogMiddleware(mainHandler, metricsRegistry)

	return &runtimeBundle{
		handler:                  mainHandler,
		routes:                   routeResult.routes,
		proxyHandlers:            routeResult.proxyHandlers,
		rateLimiters:             routeResult.rateLimiters,
		healthCheckers:           routeResult.healthCheckers,
		globalRateLimiter:        globalRateLimiter,
		globalConcurrencyLimiter: globalConcurrencyLimiter,
	}, nil
}

func registerRoutes(mux *http.ServeMux, routes []config.RouteConfig) (*routeRegisterResult, error) {
	result := &routeRegisterResult{
		routes:         make([]*routeRuntime, 0, len(routes)),
		proxyHandlers:  make([]*proxy.Handler, 0, len(routes)),
		rateLimiters:   make([]*ratelimit.TokenBucket, 0),
		healthCheckers: make([]*health.Checker, 0),
	}

	for _, route := range routes {
		activeHealth, err := toProxyActiveHealthOptions(route.HealthCheck)
		if err != nil {
			return nil, fmt.Errorf("create active health options for route %q failed: %w", route.ID, err)
		}

		passiveHealth, err := toProxyPassiveHealthOptions(route.PassiveHealth)
		if err != nil {
			return nil, fmt.Errorf("create passive health options for route %q failed: %w", route.ID, err)
		}

		circuitBreaker, err := toProxyCircuitBreakerOptions(route.CircuitBreaker)
		if err != nil {
			return nil, fmt.Errorf("create circuit breaker options for route %q failed: %w", route.ID, err)
		}

		proxyHandler, err := proxy.New(proxy.Options{
			RouteID:        route.ID,
			Target:         route.Target,
			Upstreams:      toProxyUpstreamOptions(route.Upstreams),
			StripPrefix:    route.StripPrefix,
			ActiveHealth:   activeHealth,
			PassiveHealth:  passiveHealth,
			CircuitBreaker: circuitBreaker,
		})
		if err != nil {
			return nil, fmt.Errorf("create proxy handler for route %q failed: %w", route.ID, err)
		}

		runtimeRoute := &routeRuntime{
			id:                 route.ID,
			prefix:             route.Prefix,
			stripPrefix:        route.StripPrefix,
			target:             route.Target,
			upstreams:          proxyHandler.UpstreamSnapshots(),
			rateLimitRPS:       route.RateLimitRPS,
			rateLimitBurst:     route.RateLimitBurst,
			maxConcurrency:     route.MaxConcurrency,
			healthCheckEnabled: route.HealthCheck.Enabled,
			healthCheckPath:    route.HealthCheck.Path,
			proxyHandler:       proxyHandler,
		}

		var routeHandler http.Handler = proxyHandler

		if route.MaxConcurrency > 0 {
			limitName := "route:" + route.ID
			routeConcurrencyLimiter := concurrency.NewLimiter(limitName, route.MaxConcurrency)

			runtimeRoute.concurrencyLimiter = routeConcurrencyLimiter
			routeHandler = concurrency.Middleware(limitName, routeConcurrencyLimiter)(routeHandler)
		}

		if route.RateLimitRPS > 0 {
			limiterName := "route:" + route.ID
			routeLimiter := ratelimit.NewTokenBucket(limiterName, route.RateLimitRPS, route.RateLimitBurst)

			runtimeRoute.rateLimiter = routeLimiter
			result.rateLimiters = append(result.rateLimiters, routeLimiter)
			routeHandler = ratelimit.Middleware(limiterName, routeLimiter)(routeHandler)
		}

		prefix := route.Prefix

		routeHandler = withRouteID(route.ID, routeHandler)

		log.Printf(
			"register route id=%s prefix=%s stripPrefix=%s target=%s rateLimitRPS=%d "+
				"rateLimitBurst=%d maxConcurrency=%d healthCheckEnabled=%v healthCheckPath=%s",
			route.ID,
			route.Prefix,
			route.StripPrefix,
			route.Target,
			route.RateLimitRPS,
			route.RateLimitBurst,
			route.MaxConcurrency,
			route.HealthCheck.Enabled,
			route.HealthCheck.Path,
		)

		mux.Handle(prefix, routeHandler)
		// 让 /api 也能匹配，而不是只匹配 /api/
		exactPath := strings.TrimSuffix(prefix, "/")
		if exactPath != "" && exactPath != prefix {
			mux.Handle(exactPath, routeHandler)
		}

		result.routes = append(result.routes, runtimeRoute)
		result.proxyHandlers = append(result.proxyHandlers, proxyHandler)
	}
	return result, nil
}

func toProxyActiveHealthOptions(healthCheck config.HealthCheckConfig) (proxy.ActiveHealthOptions, error) {
	if !healthCheck.Enabled {
		return proxy.ActiveHealthOptions{}, nil
	}

	interval, err := healthCheck.IntervalDuration()
	if err != nil {
		return proxy.ActiveHealthOptions{}, err
	}
	timeout, err := healthCheck.TimeoutDuration()
	if err != nil {
		return proxy.ActiveHealthOptions{}, err
	}

	return proxy.ActiveHealthOptions{
		Enabled:  true,
		Path:     healthCheck.Path,
		Interval: interval,
		Timeout:  timeout,
	}, nil
}

func toProxyPassiveHealthOptions(passiveHealth config.PassiveHealthConfig) (proxy.PassiveHealthOptions, error) {
	if !passiveHealth.Enabled {
		return proxy.PassiveHealthOptions{}, nil
	}

	unhealthyDuration, err := passiveHealth.UnhealthyDurationDuration()
	if err != nil {
		return proxy.PassiveHealthOptions{}, err
	}

	return proxy.PassiveHealthOptions{
		Enabled:           passiveHealth.Enabled,
		FailureThreshold:  passiveHealth.FailureThreshold,
		SuccessThreshold:  passiveHealth.SuccessThreshold,
		UnhealthyDuration: unhealthyDuration,
	}, nil
}

func toProxyCircuitBreakerOptions(circuitBreaker config.CircuitBreakerConfig) (proxy.CircuitBreakerOptions, error) {
	if !circuitBreaker.Enabled {
		return proxy.CircuitBreakerOptions{}, nil
	}

	openDuration, err := circuitBreaker.OpenDurationDuration()
	if err != nil {
		return proxy.CircuitBreakerOptions{}, err
	}

	return proxy.CircuitBreakerOptions{
		Enabled:             circuitBreaker.Enabled,
		FailureThreshold:    circuitBreaker.FailureThreshold,
		OpenDuration:        openDuration,
		HalfOpenMaxRequests: circuitBreaker.HalfOpenMaxRequests,
	}, nil
}

func toProxyUpstreamOptions(upstreams []config.UpstreamConfig) []proxy.UpstreamOptions {
	if len(upstreams) == 0 {
		return nil
	}

	options := make([]proxy.UpstreamOptions, 0, len(upstreams))
	for _, upstream := range upstreams {
		options = append(options, proxy.UpstreamOptions{
			ID:  upstream.ID,
			URL: upstream.URL,
		})
	}
	return options
}

func (s *Server) Start() error {
	servers := s.activeHTTPServers()

	errCh := make(chan error, len(servers))

	for _, target := range servers {
		target := target

		go func() {
			log.Printf("%s server listening on %s", target.name, target.server.Addr)

			err := target.server.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("%s server stopped with error: %w", target.name, err)
				return
			}

			errCh <- nil
		}()
	}

	for range servers {
		if err := <-errCh; err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	log.Printf("gateway shutting down")

	servers := s.activeHTTPServers()
	errCh := make(chan error, len(servers))

	for _, target := range servers {
		target := target

		go func() {
			if err := target.server.Shutdown(ctx); err != nil {
				errCh <- fmt.Errorf("%s server shutdown failed: %w", target.name, err)
				return
			}

			errCh <- nil
		}()
	}

	var result error

	for range servers {
		result = errors.Join(result, <-errCh)
	}

	s.CloseResource()

	return result
}

func (s *Server) Close() error {
	log.Printf("gateway force closing")

	servers := s.activeHTTPServers()

	var result error

	for _, target := range servers {
		if err := target.server.Close(); err != nil {
			result = errors.Join(result, fmt.Errorf("%s server close failed: %w", target.name, err))
		}
	}

	s.CloseResource()

	return result
}

func (s *Server) CloseResource() {
	s.mu.RLock()
	proxyHandlers := append([]*proxy.Handler(nil), s.proxyHandlers...)
	rateLimiters := append([]*ratelimit.TokenBucket(nil), s.rateLimiters...)
	healthCheckers := append([]*health.Checker(nil), s.healthCheckers...)
	s.mu.RUnlock()

	closeRuntimeResources(proxyHandlers, rateLimiters, healthCheckers)
}

func closeRuntimeResources(proxyHandlers []*proxy.Handler, rateLimiters []*ratelimit.TokenBucket, healthCheckers []*health.Checker) {
	for _, proxyHandler := range proxyHandlers {
		proxyHandler.CloseIdleConnections()
	}
	for _, limiter := range rateLimiters {
		limiter.Close()
	}
	for _, checker := range healthCheckers {
		checker.Close()
	}
}

func (s *Server) activeHTTPServers() []httpServerTarget {
	servers := []httpServerTarget{
		{
			name:   "main",
			server: s.httpServer,
		},
	}

	if s.adminHTTPServer != nil {
		servers = append(servers, httpServerTarget{
			name:   "admin",
			server: s.adminHTTPServer,
		})
	}

	return servers
}

func handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_, _ = w.Write([]byte("pong"))
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_, _ = w.Write([]byte("ok"))
}

func handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
	_, _ = w.Write([]byte("go-mini-gateway v0.1.0"))
}
