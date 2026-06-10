package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouteIDFromRequestFallback(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "/admin/routes", want: "admin"},
		{path: "/admin/health", want: "admin"},
		{path: "/metrics", want: "admin"},
		{path: "/ping", want: "system"},
		{path: "/health", want: "system"},
		{path: "/version", want: "system"},
		{path: "/api/hello", want: "unknown"},
		{path: "/not-exist", want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)

			got := routeIDFromRequest(req)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
