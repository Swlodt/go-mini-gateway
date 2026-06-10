package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go-mini-gateway/internal/metrics"
)

func TestAccessLogMiddlewareRecordsMetrics(t *testing.T) {
	registry := metrics.NewRegistry()

	handler := accessLogMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	}), registry)

	req := httptest.NewRequest(http.MethodPost, "/not-exist", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status got %d, want %d", rec.Code, http.StatusCreated)
	}

	snapshot := registry.Snapshot()

	if snapshot.Total.Requests != 1 {
		t.Fatalf("total requests got %d, want 1", snapshot.Total.Requests)
	}

	if snapshot.Total.StatusCodes["201"] != 1 {
		t.Fatalf("status 201 got %d, want 1", snapshot.Total.StatusCodes["201"])
	}

	if snapshot.Total.BytesWritten != int64(len("created")) {
		t.Fatalf("bytes got %d, want %d", snapshot.Total.BytesWritten, len("created"))
	}

	unknown := snapshot.Routes["unknown"]
	if unknown.Requests != 1 {
		t.Fatalf("unknown requests got %d, want 1", unknown.Requests)
	}
}

func TestAccessLogMiddlewareRouteIDFallback(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantRouteID string
	}{
		{
			name:        "admin path",
			path:        "/admin/routes",
			wantRouteID: "admin",
		},
		{
			name:        "metrics path",
			path:        "/metrics",
			wantRouteID: "admin",
		},
		{
			name:        "system ping",
			path:        "/ping",
			wantRouteID: "system",
		},
		{
			name:        "system health",
			path:        "/health",
			wantRouteID: "system",
		},
		{
			name:        "system version",
			path:        "/version",
			wantRouteID: "system",
		},
		{
			name:        "unknown",
			path:        "/not-exist",
			wantRouteID: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := metrics.NewRegistry()

			handler := accessLogMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}), registry)

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			snapshot := registry.Snapshot()

			got := snapshot.Routes[tt.wantRouteID].Requests
			if got != 1 {
				t.Fatalf("route %s requests got %d, want 1", tt.wantRouteID, got)
			}
		})
	}
}

func TestAccessLogMiddlewareUsesRouteIDFromRecorder(t *testing.T) {
	registry := metrics.NewRegistry()

	var inner http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("rate limit exceeded"))
	})

	// 模拟真实链路：
	// accessLogMiddleware 在最外层；
	// withRouteID 在内层；
	// handler 返回 429。
	handler := accessLogMiddleware(withRouteID("demo", inner), registry)

	req := httptest.NewRequest(http.MethodGet, "/api/hello", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status got %d, want %d", rec.Code, http.StatusTooManyRequests)
	}

	snapshot := registry.Snapshot()

	demo := snapshot.Routes["demo"]
	if demo.Requests != 1 {
		t.Fatalf("demo requests got %d, want 1", demo.Requests)
	}

	if demo.StatusCodes["429"] != 1 {
		t.Fatalf("demo status 429 got %d, want 1", demo.StatusCodes["429"])
	}

	if unknown := snapshot.Routes["unknown"].Requests; unknown != 0 {
		t.Fatalf("unknown requests got %d, want 0", unknown)
	}
}

func TestStatusRecorderDefaultStatusOK(t *testing.T) {
	rec := httptest.NewRecorder()
	statusRec := newStatusRecorder(rec)

	_, _ = statusRec.Write([]byte("ok"))

	if statusRec.StatusCode() != http.StatusOK {
		t.Fatalf("status got %d, want %d", statusRec.StatusCode(), http.StatusOK)
	}

	if statusRec.BytesWritten() != 2 {
		t.Fatalf("bytes got %d, want 2", statusRec.BytesWritten())
	}
}

func TestStatusRecorderWriteHeaderOnlyOnce(t *testing.T) {
	rec := httptest.NewRecorder()
	statusRec := newStatusRecorder(rec)

	statusRec.WriteHeader(http.StatusCreated)
	statusRec.WriteHeader(http.StatusInternalServerError)

	if statusRec.StatusCode() != http.StatusCreated {
		t.Fatalf("status got %d, want %d", statusRec.StatusCode(), http.StatusCreated)
	}

	if rec.Code != http.StatusCreated {
		t.Fatalf("recorder status got %d, want %d", rec.Code, http.StatusCreated)
	}
}
