package server

import (
	"go-mini-gateway/internal/concurrency"
	"go-mini-gateway/internal/config"
	"go-mini-gateway/internal/health"
	"go-mini-gateway/internal/metrics"
	"go-mini-gateway/internal/proxy"
	"go-mini-gateway/internal/ratelimit"
	"net/http"
)

type runtimeBundle struct {
	handler                  http.Handler
	routes                   []*routeRuntime
	proxyHandlers            []*proxy.Handler
	rateLimiters             []*ratelimit.TokenBucket
	healthCheckers           []*health.Checker
	globalRateLimiter        *ratelimit.TokenBucket
	globalConcurrencyLimiter *concurrency.Limiter
}

func buildRuntime(cfg *config.Config, metricsRegistry *metrics.Registry) (*runtimeBundle, error) {
	requestTimeout, err := cfg.RequestTimeoutDuration()
	if err != nil {
		return nil, err
	}

	mainMux := http.NewServeMux()
	registerSystemRoutes(mainMux)

	routeResult, err := registerRoutes(mainMux, cfg.Routes)
	if err != nil {
		return nil, err
	}

	var mainHandler http.Handler = mainMux
	mainHandler = timeoutMiddleware(requestTimeout)(mainHandler)

	var globalConcurrencyLimiter *concurrency.Limiter
	var globalRateLimiter *ratelimit.TokenBucket

	if cfg.Server.MaxConcurrency > 0 {
		globalConcurrencyLimiter = concurrency.NewLimiter("global", cfg.Server.MaxConcurrency)
		mainHandler = concurrency.Middleware("global", globalConcurrencyLimiter)(mainHandler)
	}

	if cfg.Server.RateLimitRPS > 0 {
		globalRateLimiter = ratelimit.NewTokenBucket("global", cfg.Server.RateLimitRPS, cfg.Server.RateLimitBurst)
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
