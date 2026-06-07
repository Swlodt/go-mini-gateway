package server

import (
	"errors"
	"fmt"
	"go-mini-gateway/internal/config"
	"go-mini-gateway/internal/proxy"
	"log"
	"net/http"
	"strings"
	"time"
)

type Server struct {
	httpServer *http.Server
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

	if err := registerRoutes(mux, cfg.Routes); err != nil {
		return nil, err
	}

	var handler http.Handler = mux

	handler = timeoutMiddleware(requestTimeout)(handler)
	handler = accessLogMiddleware(handler)

	server := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return &Server{
		httpServer: server,
	}, nil
}

func registerRoutes(mux *http.ServeMux, routes []config.RouteConfig) error {
	for _, route := range routes {
		proxyHandler, err := proxy.New(proxy.Options{
			RouteID:     route.ID,
			Target:      route.Target,
			StripPrefix: route.StripPrefix,
		})
		if err != nil {
			return fmt.Errorf("create proxy handler for route %q failed: %w", route.ID, err)
		}

		prefix := route.Prefix

		log.Printf(
			"register route id=%s prefix=%s stripPrefix=%s target=%s",
			route.ID,
			route.Prefix,
			route.StripPrefix,
			route.Target,
		)

		mux.Handle(prefix, proxyHandler)

		// 让 /api 也能匹配，而不是只匹配 /api/
		exactPath := strings.TrimSuffix(prefix, "/")
		if exactPath != "" && exactPath != prefix {
			mux.Handle(exactPath, proxyHandler)
		}
	}
	return nil
}

func (s *Server) Start() error {
	log.Printf("Gateway listening on %s", s.httpServer.Addr)
	err := s.httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
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
