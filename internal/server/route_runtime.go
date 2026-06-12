package server

import (
	"fmt"
	"go-mini-gateway/internal/concurrency"
	"go-mini-gateway/internal/config"
	"go-mini-gateway/internal/health"
	"go-mini-gateway/internal/proxy"
	"go-mini-gateway/internal/ratelimit"
	"log"
	"net/http"
	"strings"
)

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

func registerRoutes(mux *http.ServeMux, routes []config.RouteConfig) (*routeRegisterResult, error) {
	result := &routeRegisterResult{
		routes:         make([]*routeRuntime, 0, len(routes)),
		proxyHandlers:  make([]*proxy.Handler, 0, len(routes)),
		rateLimiters:   make([]*ratelimit.TokenBucket, 0),
		healthCheckers: make([]*health.Checker, 0),
	}

	for _, route := range routes {
		proxyHandler, err := newProxyHandler(route)
		if err != nil {
			return nil, err
		}

		runtimeRoute := newRouteRuntime(route, proxyHandler)
		var routeHandler http.Handler = proxyHandler

		routeHandler = applyRouteConcurrencyLimit(routeHandler, route, runtimeRoute)
		routeHandler = applyRouteRateLimit(routeHandler, route, runtimeRoute, result)
		routeHandler = withRouteID(route.ID, routeHandler)

		registerRoutePatterns(mux, route.Prefix, routeHandler)
		logRouteRegistration(route)

		result.routes = append(result.routes, runtimeRoute)
		result.proxyHandlers = append(result.proxyHandlers, proxyHandler)
	}

	return result, nil
}

func newProxyHandler(route config.RouteConfig) (*proxy.Handler, error) {
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

	return proxyHandler, nil
}

func newRouteRuntime(route config.RouteConfig, proxyHandler *proxy.Handler) *routeRuntime {
	return &routeRuntime{
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
}

func applyRouteConcurrencyLimit(next http.Handler, route config.RouteConfig, runtimeRoute *routeRuntime) http.Handler {
	if route.MaxConcurrency <= 0 {
		return next
	}

	limitName := "route:" + route.ID
	limiter := concurrency.NewLimiter(limitName, route.MaxConcurrency)
	runtimeRoute.concurrencyLimiter = limiter
	return concurrency.Middleware(limitName, limiter)(next)
}

func applyRouteRateLimit(next http.Handler, route config.RouteConfig, runtimeRoute *routeRuntime, result *routeRegisterResult) http.Handler {
	if route.RateLimitRPS <= 0 {
		return next
	}

	limiterName := "route:" + route.ID
	limiter := ratelimit.NewTokenBucket(limiterName, route.RateLimitRPS, route.RateLimitBurst)
	runtimeRoute.rateLimiter = limiter
	result.rateLimiters = append(result.rateLimiters, limiter)
	return ratelimit.Middleware(limiterName, limiter)(next)
}

func registerRoutePatterns(mux *http.ServeMux, prefix string, handler http.Handler) {
	mux.Handle(prefix, handler)

	// Make /api match the same handler as /api/.
	exactPath := strings.TrimSuffix(prefix, "/")
	if exactPath != "" && exactPath != prefix {
		mux.Handle(exactPath, handler)
	}
}

func logRouteRegistration(route config.RouteConfig) {
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
