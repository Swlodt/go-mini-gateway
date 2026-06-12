package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "" {
			name = "anonymous"
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Backend-Server", "demo-backend")

		traceID := r.Header.Get("X-Trace-ID")

		_, _ = fmt.Fprintf(w, "Hello %s. response from backend, traceID:%s", name, traceID)
	})

	mux.HandleFunc("/hello/v2", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "" {
			name = "anonymous"
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Backend-Server", "demo-backend")

		_, _ = fmt.Fprintf(
			w,
			"hello, %s. response from backend\n"+
				"host=%s\n"+
				"x-forwarded-for=%s\n"+
				"x-forwarded-host=%s\n"+
				"x-forwarded-proto=%s\n"+
				"x-gateway=%s\n",
			name,
			r.Host,
			r.Header.Get("X-Forwarded-For"),
			r.Header.Get("X-Forwarded-Host"),
			r.Header.Get("X-Forwarded-Proto"),
			r.Header.Get("X-Gateway"),
		)
	})

	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read request body failed", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprintf(w, "method=%s path=%s body=%s", r.Method, r.URL.Path, string(body))
	})

	mux.HandleFunc("/error", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "backend busy", http.StatusServiceUnavailable)
	})

	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprintf(w, "slow response from backend")
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprintln(w, "ok")
	})

	server := &http.Server{
		Addr:    ":8082",
		Handler: mux,
	}

	log.Printf("backend listening on %s", server.Addr)

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("backend server failed: %v", err)
	}

}
