package server

import (
	"encoding/json"
	"go-mini-gateway/internal/concurrency"
	"go-mini-gateway/internal/health"
	"go-mini-gateway/internal/proxy"
	"go-mini-gateway/internal/ratelimit"
	"net/http"
	"net/http/pprof"
)

type routeDTO struct {
	ID                 string                   `json:"id"`
	Prefix             string                   `json:"prefix"`
	StripPrefix        string                   `json:"stripPrefix"`
	Target             string                   `json:"target"`
	Upstreams          []proxy.UpstreamSnapshot `json:"upstreams"`
	RateLimitRPS       int                      `json:"rateLimitRPS"`
	RateLimitBurst     int                      `json:"rateLimitBurst"`
	MaxConcurrency     int                      `json:"maxConcurrency"`
	HealthCheckEnabled bool                     `json:"healthCheckEnabled"`
	HealthCheckPath    string                   `json:"healthCheckPath,omitempty"`
}

type healthDTO struct {
	RouteID string `json:"routeId"`
	Target  string `json:"target"`

	Name          string `json:"name"`
	Path          string `json:"path,omitempty"`
	Interval      string `json:"interval,omitempty"`
	Timeout       string `json:"timeout,omitempty"`
	Checked       bool   `json:"checked"`
	Healthy       bool   `json:"healthy"`
	LastCheckedAt string `json:"lastCheckedAt,omitempty"`
	LastReason    string `json:"lastReason,omitempty"`
}

type statusDTO struct {
	Global globalStatusDTO  `json:"global"`
	Routes []routeStatusDTO `json:"routes"`
}

type globalStatusDTO struct {
	RateLimit   *ratelimit.SnapShot   `json:"rateLimit,omitempty"`
	Concurrency *concurrency.Snapshot `json:"concurrency,omitempty"`
}

type routeStatusDTO struct {
	ID          string                `json:"id"`
	RateLimit   *ratelimit.SnapShot   `json:"rateLimit,omitempty"`
	Concurrency *concurrency.Snapshot `json:"concurrency,omitempty"`
	Health      *health.Snapshot      `json:"health,omitempty"`
}

func (s *Server) registerAdminRoutes(mux *http.ServeMux) {
	if s.adminEnabled {
		mux.Handle("/admin/routes", s.adminAuthMiddleware(http.HandlerFunc(s.handleAdminRoutes)))
		mux.Handle("/admin/health", s.adminAuthMiddleware(http.HandlerFunc(s.handleAdminHealth)))
		mux.Handle("/admin/stats", s.adminAuthMiddleware(http.HandlerFunc(s.handleAdminStats)))
		mux.Handle("/admin/metrics", s.adminAuthMiddleware(http.HandlerFunc(s.handleAdminMetrics)))
		if s.pprofEnabled {
			s.registerPprofRoutes(mux)
		}
	}

	metricsHandler := http.HandlerFunc(s.handlePrometheusMetrics)
	if s.metricsRequireToken {
		mux.Handle("/metrics", s.adminAuthMiddleware(metricsHandler))
		return
	}
	mux.Handle("/metrics", metricsHandler)
}

func (s *Server) registerPprofRoutes(mux *http.ServeMux) {
	mux.Handle("/debug/pprof/", s.adminAuthMiddleware(http.HandlerFunc(pprof.Index)))
	mux.Handle("/debug/pprof/cmdline", s.adminAuthMiddleware(http.HandlerFunc(pprof.Cmdline)))
	mux.Handle("/debug/pprof/profile", s.adminAuthMiddleware(http.HandlerFunc(pprof.Profile)))
	mux.Handle("/debug/pprof/symbol", s.adminAuthMiddleware(http.HandlerFunc(pprof.Symbol)))
	mux.Handle("/debug/pprof/trace", s.adminAuthMiddleware(http.HandlerFunc(pprof.Trace)))

	mux.Handle("/debug/pprof/allocs", s.adminAuthMiddleware(pprof.Handler("allocs")))
	mux.Handle("/debug/pprof/block", s.adminAuthMiddleware(pprof.Handler("block")))
	mux.Handle("/debug/pprof/goroutine", s.adminAuthMiddleware(pprof.Handler("goroutine")))
	mux.Handle("/debug/pprof/heap", s.adminAuthMiddleware(pprof.Handler("heap")))
	mux.Handle("/debug/pprof/mutex", s.adminAuthMiddleware(pprof.Handler("mutex")))
	mux.Handle("/debug/pprof/threadcreate", s.adminAuthMiddleware(pprof.Handler("threadcreate")))
}

func (s *Server) handleAdminRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

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

	writeJSON(w, http.StatusOK, routes)
}

func (s *Server) handleAdminHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	items := make([]healthDTO, 0, len(s.routes))

	for _, route := range s.routes {
		if route.healthChecker == nil {
			items = append(items, healthDTO{
				RouteID: route.id,
				Target:  route.target,
				Checked: false,
				Healthy: true,
			})
			continue
		}

		snapshot := route.healthChecker.Snapshot()

		items = append(items, healthDTO{
			RouteID:       route.id,
			Target:        route.target,
			Name:          snapshot.Name,
			Path:          snapshot.Path,
			Interval:      snapshot.Interval,
			Timeout:       snapshot.Timeout,
			Checked:       snapshot.Checked,
			Healthy:       snapshot.Healthy,
			LastCheckedAt: snapshot.LastCheckedAt,
			LastReason:    snapshot.LastReason,
		})
	}

	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := statusDTO{
		Routes: make([]routeStatusDTO, 0, len(s.routes)),
	}

	if s.globalRateLimiter != nil {
		snapshot := s.globalRateLimiter.Snapshot()
		resp.Global.RateLimit = &snapshot
	}
	if s.globalConcurrencyLimiter != nil {
		snapshot := s.globalConcurrencyLimiter.Snapshot()
		resp.Global.Concurrency = &snapshot
	}

	for _, route := range s.routes {
		item := routeStatusDTO{
			ID: route.id,
		}
		if route.rateLimiter != nil {
			snapshot := route.rateLimiter.Snapshot()
			item.RateLimit = &snapshot
		}
		if route.concurrencyLimiter != nil {
			snapshot := route.concurrencyLimiter.Snapshot()
			item.Concurrency = &snapshot
		}
		if route.healthChecker != nil {
			snapshot := route.healthChecker.Snapshot()
			item.Health = &snapshot
		}
		resp.Routes = append(resp.Routes, item)
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAdminMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.metricsRegistry == nil {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	writeJSON(w, http.StatusOK, s.metricsRegistry.Snapshot())
}

func (s *Server) handlePrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	if s.metricsRegistry == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	_, _ = w.Write([]byte(s.metricsRegistry.PrometheusText()))
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(value); err != nil {
		return
	}
}
