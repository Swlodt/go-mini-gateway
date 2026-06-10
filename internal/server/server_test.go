package server

//
//import (
//	"net/http"
//	"net/http/httptest"
//	"testing"
//
//	"go-mini-gateway/internal/config"
//)
//
//func TestRegisterRoutesAppliesRouteRateLimit(t *testing.T) {
//	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
//		w.WriteHeader(http.StatusOK)
//	}))
//	defer backend.Close()
//
//	tests := []struct {
//		name string
//		path string
//	}{
//		{name: "prefix path", path: "/api/resource"},
//		{name: "exact path", path: "/api"},
//	}
//
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			mux := http.NewServeMux()
//			result, err := registerRoutes(mux, []config.RouteConfig{
//				{
//					ID:             "demo",
//					Prefix:         "/api/",
//					StripPrefix:    "/api",
//					Target:         backend.URL,
//					RateLimitRPS:   1,
//					RateLimitBurst: 1,
//				},
//			})
//			if err != nil {
//				t.Fatalf("registerRoutes() error = %v", err)
//			}
//			defer func() {
//				for _, limiter := range result.rateLimiters {
//					limiter.Close()
//				}
//				for _, handler := range result.proxyHandlers {
//					handler.CloseIdleConnections()
//				}
//			}()
//
//			first := httptest.NewRecorder()
//			mux.ServeHTTP(first, httptest.NewRequest(http.MethodGet, tt.path, nil))
//			if first.Code != http.StatusOK {
//				t.Fatalf("first request status = %d, want %d", first.Code, http.StatusOK)
//			}
//
//			second := httptest.NewRecorder()
//			mux.ServeHTTP(second, httptest.NewRequest(http.MethodGet, tt.path, nil))
//			if second.Code != http.StatusTooManyRequests {
//				t.Fatalf("second request status = %d, want %d", second.Code, http.StatusTooManyRequests)
//			}
//		})
//	}
//}
