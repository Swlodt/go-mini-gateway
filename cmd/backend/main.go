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

		_, _ = fmt.Fprintf(w, "Hello %s. response from backend", name)
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

	server := &http.Server{
		Addr:    ":8081",
		Handler: mux,
	}

	log.Printf("backend listening on %s", server.Addr)

	if err := server.ListenAndServe(); err != nil && errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("backend server failed: %v", err)
	}

}
