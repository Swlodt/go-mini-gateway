package server

import (
	"fmt"
	"go-mini-gateway/internal/config"
	"log"
	"net/http"
	"time"
)

type reloadResultDTO struct {
	Success    bool       `json:"success"`
	Message    string     `json:"message"`
	ReloadedAt string     `json:"reloadedAt"`
	Routes     []routeDTO `json:"routes,omitempty"`
}

func (s *Server) handleAdminReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	result, err := s.Reload()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, reloadResultDTO{
			Success:    false,
			Message:    err.Error(),
			ReloadedAt: time.Now().Format(time.RFC3339Nano),
		})
		return
	}

	writeJSON(w, http.StatusOK, reloadResultDTO{
		Success:    true,
		Message:    "config reloaded",
		ReloadedAt: time.Now().Format(time.RFC3339Nano),
		Routes:     result,
	})
}

func (s *Server) Reload() ([]routeDTO, error) {
	if s.configPath == "" {
		return nil, fmt.Errorf("config path is not configured; reload requires server.NewWithConfigPath")
	}

	newConfig, err := config.Load(s.configPath)
	if err != nil {
		return nil, fmt.Errorf("load config failed: %w", err)
	}

	if err := s.validateReloadConfig(newConfig); err != nil {
		return nil, err
	}

	newRuntime, err := buildRuntime(newConfig, s.metricsRegistry)
	if err != nil {
		return nil, fmt.Errorf("build new runtime failed: %w", err)
	}

	s.mu.Lock()
	oldProxyHandlers := s.proxyHandlers
	oldRateLimiters := s.rateLimiters
	oldHealthCheckers := s.healthCheckers

	s.routes = newRuntime.routes
	s.proxyHandlers = newRuntime.proxyHandlers
	s.rateLimiters = newRuntime.rateLimiters
	s.healthCheckers = newRuntime.healthCheckers
	s.globalRateLimiter = newRuntime.globalRateLimiter
	s.globalConcurrencyLimiter = newRuntime.globalConcurrencyLimiter
	s.currentConfig = newConfig
	s.mainHandler.Store(newRuntime.handler)

	routes := s.routeDTOsLocked()
	s.mu.Unlock()

	closeRuntimeResources(oldProxyHandlers, oldRateLimiters, oldHealthCheckers)

	log.Printf("config reloaded: path=%s routes=%d", s.configPath, len(newRuntime.routes))
	return routes, nil
}

func (s *Server) validateReloadConfig(newConfig *config.Config) error {
	s.mu.RLock()
	oldHTTPAddr := ""
	if s.httpServer != nil {
		oldHTTPAddr = s.httpServer.Addr
	}
	oldAdminEnabled := s.adminEnabled
	oldAdminAddr := ""
	if s.adminHTTPServer != nil {
		oldAdminAddr = s.adminHTTPServer.Addr
	}
	oldAdminToken := s.adminToken
	oldMetricsRequireToken := s.metricsRequireToken
	oldPprofEnabled := s.pprofEnabled
	oldShutdownTimeout := ""
	if s.currentConfig != nil {
		oldShutdownTimeout = s.currentConfig.Server.ShutdownTimeout
	}
	s.mu.RUnlock()

	if newConfig.Addr() != oldHTTPAddr {
		return fmt.Errorf("server.addr change requires restart: current=%q new=%q", oldHTTPAddr, newConfig.Addr())
	}

	if newConfig.Server.ShutdownTimeout != oldShutdownTimeout {
		return fmt.Errorf("server.shutdownTimeout change requires restart: current=%q new=%q", oldShutdownTimeout, newConfig.Server.ShutdownTimeout)
	}

	if newConfig.Admin.Enabled != oldAdminEnabled {
		return fmt.Errorf("admin.enabled change requires restart")
	}

	newAdminAddr := ""
	if newConfig.Admin.Enabled {
		newAdminAddr = newConfig.Admin.Addr
	}
	if newAdminAddr != oldAdminAddr {
		return fmt.Errorf("admin.addr change requires restart: current=%q new=%q", oldAdminAddr, newAdminAddr)
	}

	if newConfig.Admin.Token != oldAdminToken {
		return fmt.Errorf("admin.token change requires restart")
	}

	if newConfig.Admin.MetricsRequireToken != oldMetricsRequireToken {
		return fmt.Errorf("admin.metricsRequireToken change requires restart")
	}

	if newConfig.Admin.PprofEnabled != oldPprofEnabled {
		return fmt.Errorf("admin.pprofEnabled change requires restart")
	}

	return nil
}

func (s *Server) routeDTOsLocked() []routeDTO {
	routes := make([]routeDTO, 0, len(s.routes))
	for _, route := range s.routes {
		upstreams := route.upstreams
		if route.proxyHandler != nil {
			upstreams = route.proxyHandler.UpstreamSnapshots()
		}

		routes = append(routes, routeDTO{
			ID:                 route.id,
			Prefix:             route.prefix,
			StripPrefix:        route.stripPrefix,
			Target:             route.target,
			Upstreams:          upstreams,
			RateLimitRPS:       route.rateLimitRPS,
			RateLimitBurst:     route.rateLimitBurst,
			MaxConcurrency:     route.maxConcurrency,
			HealthCheckEnabled: route.healthCheckEnabled,
			HealthCheckPath:    route.healthCheckPath,
		})
	}
	return routes
}
