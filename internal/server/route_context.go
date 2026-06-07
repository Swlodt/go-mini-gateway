package server

import (
	"context"
	"net/http"
	"strings"
)

type routeIDContextKey struct{}

type routeIDSetter interface {
	SetRouteID(routeID string)
}

type responseWriterUnwrapper interface {
	Unwrap() http.ResponseWriter
}

func withRouteID(routeID string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setRouteIDToResponseWriter(w, routeID)
		ctx := context.WithValue(r.Context(), routeIDContextKey{}, routeID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func setRouteIDToResponseWriter(w http.ResponseWriter, routeID string) {
	for w != nil {
		if setter, ok := w.(routeIDSetter); ok {
			setter.SetRouteID(routeID)
			return
		}
	}
	unwrapper, ok := w.(responseWriterUnwrapper)
	if !ok {
		return
	}
	w = unwrapper.Unwrap()
}

func routeIDFromRequest(r *http.Request) string {
	if r == nil {
		return "unknown"
	}

	if value := r.Context().Value(routeIDContextKey{}); value != nil {
		if routeID, ok := value.(string); ok && routeID != "" {
			return routeID
		}
	}

	path := r.URL.Path

	switch {
	case strings.HasPrefix(path, "/admin/"):
		return "admin"
	case path == "/ping" || path == "/health" || path == "/version":
		return "system"
	default:
		return "unknown"
	}
}
