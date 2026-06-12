package server

import "net/http"

func registerSystemRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/ping", handlePing)
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/version", handleVersion)
}

func handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_, _ = w.Write([]byte("pong"))
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_, _ = w.Write([]byte("ok"))
}

func handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_, _ = w.Write([]byte("go-mini-gateway v0.1.0"))
}
