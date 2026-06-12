package server

import "net/http"

func (s *Server) registerAdminRoutes(mux *http.ServeMux) {
	if s.adminEnabled {
		mux.Handle("/admin/routes", s.adminAuthMiddleware(http.HandlerFunc(s.handleAdminRoutes)))
		mux.Handle("/admin/health", s.adminAuthMiddleware(http.HandlerFunc(s.handleAdminHealth)))
		mux.Handle("/admin/stats", s.adminAuthMiddleware(http.HandlerFunc(s.handleAdminStats)))
		mux.Handle("/admin/metrics", s.adminAuthMiddleware(http.HandlerFunc(s.handleAdminMetrics)))
		mux.Handle("/admin/reload", s.adminAuthMiddleware(http.HandlerFunc(s.handleAdminReload)))
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

func (s *Server) handleAdminRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	routes := s.routeDTOsLocked()
	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, routes)
}

func (s *Server) handleAdminHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	items := make([]healthDTO, 0, len(s.routes))

	for _, route := range s.routes {
		upstreams := route.upstreams
		if route.proxyHandler != nil {
			upstreams = route.proxyHandler.UpstreamSnapshots()
		}

		item := healthDTO{
			RouteID: route.id,
			Target:  route.target,
			Healthy: true,
		}

		for _, upstream := range upstreams {
			if upstream.ActiveHealth == nil {
				continue
			}

			snapshot := upstream.ActiveHealth
			item.Checked = item.Checked || snapshot.Checked
			// Active health is fail-open before the first check, so an unchecked upstream is still treated as available.
			if snapshot.Checked && !snapshot.Healthy {
				// The route is unhealthy only when every active-health-enabled upstream is checked and unhealthy.
			}

			item.Upstreams = append(item.Upstreams, upstreamHealthDTO{
				ID:            upstream.ID,
				URL:           upstream.URL,
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

		if len(item.Upstreams) > 0 {
			item.Healthy = false
			for _, upstream := range item.Upstreams {
				if !upstream.Checked || upstream.Healthy {
					item.Healthy = true
					break
				}
			}
		}

		items = append(items, item)
	}

	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
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

	s.mu.RUnlock()

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
