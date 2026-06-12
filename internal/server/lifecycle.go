package server

import (
	"context"
	"errors"
	"fmt"
	"go-mini-gateway/internal/health"
	"go-mini-gateway/internal/proxy"
	"go-mini-gateway/internal/ratelimit"
	"log"
	"net/http"
)

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

	var result error
	for _, target := range s.activeHTTPServers() {
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
		{name: "main", server: s.httpServer},
	}

	if s.adminHTTPServer != nil {
		servers = append(servers, httpServerTarget{name: "admin", server: s.adminHTTPServer})
	}

	return servers
}
