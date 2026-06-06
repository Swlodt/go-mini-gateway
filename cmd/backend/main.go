package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
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

	server := &http.Server{
		Addr:    ":8081",
		Handler: mux,
	}

	log.Printf("backend listening on %s", server.Addr)

	if err := server.ListenAndServe(); err != nil && errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("backend server failed: %v", err)
	}

}
