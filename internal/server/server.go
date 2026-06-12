package server

import (
	"go-mini-gateway/internal/concurrency"
	"go-mini-gateway/internal/config"
	"go-mini-gateway/internal/health"
	"go-mini-gateway/internal/metrics"
	"go-mini-gateway/internal/proxy"
	"go-mini-gateway/internal/ratelimit"
	"net/http"
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
		// Admin APIs do not use business rate/concurrency limiters, but they are still logged and counted.
		adminHandler = accessLogMiddleware(adminHandler, metricsRegistry)

		srv.adminHTTPServer = &http.Server{
			Addr:              cfg.Admin.Addr,
			Handler:           adminHandler,
			ReadHeaderTimeout: 5 * time.Second,
		}
	}

	return srv, nil
}
