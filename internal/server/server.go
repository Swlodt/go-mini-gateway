package server

import (
	"errors"
	"fmt"
	"go-mini-gateway/internal/proxy"
	"log"
	"net/http"
	"time"
)

type Server struct {
	httpServer *http.Server
}

func New(addr string) (*Server, error) {
	mux := http.NewServeMux()

	mux.HandleFunc("/ping", handlePing)
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/version", handleVersion)

	proxyHandler, err := proxy.New("http://127.0.0.1:8081")
	if err != nil {
		return nil, err
	}

	mux.Handle("/api/", proxyHandler)

	server := &http.Server{
		Addr:              addr,
		Handler:           loggingMiddleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	return &Server{
		httpServer: server,
	}, nil
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

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		duration := time.Since(start)
		log.Printf("%s %s cost=%s", r.Method, r.URL.Path, duration)
	})
}

func (s *Server) Addr() string {
	if s == nil || s.httpServer == nil {
		return ""
	}
	return fmt.Sprint(s.httpServer.Addr)
}
