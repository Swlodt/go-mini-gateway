package server

import (
	"context"
	"errors"
	"fmt"
	"go-mini-gateway/internal/concurrency"
	"go-mini-gateway/internal/config"
	"go-mini-gateway/internal/proxy"
	"go-mini-gateway/internal/ratelimit"
	"log"
	"net/http"
	"strings"
	"time"
)

type Server struct {
	httpServer    *http.Server
	proxyHandlers []*proxy.Handler
	rateLimiters  []*ratelimit.TokenBucket
}

type routeRegisterResult struct {
	proxyHandlers []*proxy.Handler
	rateLimiters  []*ratelimit.TokenBucket
}

func New(cfg *config.Config) (*Server, error) {
	requestTimeout, err := cfg.RequestTimeoutDuration()
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/ping", handlePing)
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/version", handleVersion)

	routeResult, err := registerRoutes(mux, cfg.Routes)
	if err != nil {
		return nil, err
	}

	var handler http.Handler = mux

	handler = timeoutMiddleware(requestTimeout)(handler)

	if cfg.Server.MaxConcurrency > 0 {
		globalConcurrencyLimiter := concurrency.NewLimiter("global", cfg.Server.MaxConcurrency)
		handler = concurrency.Middleware("global", globalConcurrencyLimiter)(handler)
	}

	if cfg.Server.RateLimitRPS > 0 {
		globalLimiter := ratelimit.NewTokenBucket("global", cfg.Server.RateLimitRPS, cfg.Server.RateLimitBurst)
		routeResult.rateLimiters = append(routeResult.rateLimiters, globalLimiter)
		handler = ratelimit.Middleware("global", globalLimiter)(handler)
	}

	handler = accessLogMiddleware(handler)

	server := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return &Server{
		httpServer:    server,
		proxyHandlers: routeResult.proxyHandlers,
		rateLimiters:  routeResult.rateLimiters,
	}, nil
}

func registerRoutes(mux *http.ServeMux, routes []config.RouteConfig) (*routeRegisterResult, error) {
	result := &routeRegisterResult{
		proxyHandlers: make([]*proxy.Handler, 0, len(routes)),
		rateLimiters:  make([]*ratelimit.TokenBucket, 0),
	}

	for _, route := range routes {
		proxyHandler, err := proxy.New(proxy.Options{
			RouteID:     route.ID,
			Target:      route.Target,
			StripPrefix: route.StripPrefix,
		})
		if err != nil {
			return nil, fmt.Errorf("create proxy handler for route %q failed: %w", route.ID, err)
		}

		var routeHandler http.Handler = proxyHandler

		if route.MaxConcurrency > 0 {
			limitName := "route:" + route.ID
			routeConcurrencyLimiter := concurrency.NewLimiter(limitName, route.MaxConcurrency)
			routeHandler = concurrency.Middleware(limitName, routeConcurrencyLimiter)(routeHandler)
		}

		if route.RateLimitRPS > 0 {
			limiterName := "route:" + route.ID
			routeLimiter := ratelimit.NewTokenBucket(limiterName, route.RateLimitRPS, route.RateLimitBurst)

			result.rateLimiters = append(result.rateLimiters, routeLimiter)
			routeHandler = ratelimit.Middleware(limiterName, routeLimiter)(routeHandler)
		}

		prefix := route.Prefix

		log.Printf(
			"register route id=%s prefix=%s stripPrefix=%s target=%s rateLimitRPS=%d rateLimitBurst=%d maxConcurrency=%d",
			route.ID,
			prefix,
			route.StripPrefix,
			route.Target,
			route.RateLimitRPS,
			route.RateLimitBurst,
			route.MaxConcurrency,
		)

		mux.Handle(prefix, routeHandler)
		// 让 /api 也能匹配，而不是只匹配 /api/
		exactPath := strings.TrimSuffix(prefix, "/")
		if exactPath != "" && exactPath != prefix {
			mux.Handle(exactPath, routeHandler)
		}

		result.proxyHandlers = append(result.proxyHandlers, proxyHandler)
	}
	return result, nil
}

func (s *Server) Start() error {
	log.Printf("Gateway listening on %s", s.httpServer.Addr)
	err := s.httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	log.Printf("gateway shutting down")
	err := s.httpServer.Shutdown(ctx)
	s.CloseResource()
	return err
}

func (s *Server) Close() error {
	log.Printf("gateway force closing")
	err := s.httpServer.Close()
	s.CloseResource()
	return err
}

func (s *Server) CloseResource() {
	for _, proxyHandler := range s.proxyHandlers {
		proxyHandler.CloseIdleConnections()
	}
	for _, limiter := range s.rateLimiters {
		limiter.Close()
	}
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
