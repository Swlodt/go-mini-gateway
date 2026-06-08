package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
)

const adminTokenHeader = "X-Admin-Token"

func (s *Server) adminAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.adminToken == "" {
			http.Error(w, "admin token is not configured", http.StatusServiceUnavailable)
			return
		}

		actualToken := r.Header.Get(adminTokenHeader)

		if !constantTimeTokenEqual(actualToken, s.adminToken) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func constantTimeTokenEqual(actual string, expected string) bool {
	actualHash := sha256.Sum256([]byte(actual))
	expectedHash := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(actualHash[:], expectedHash[:]) == 1
}
