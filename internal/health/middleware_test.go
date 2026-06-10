package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMiddlewareAllowsWhenHealthy(t *testing.T) {
	checker, err := NewChecker(Options{
		Name:     "demo",
		Target:   "http://localhost:8081",
		Path:     "/health",
		Interval: time.Second,
		Timeout:  time.Second,
	})
	if err != nil {
		t.Fatalf("NewChecker() error: %v", err)
	}

	checker.update(true, "test healthy")

	nextCalled := false

	handler := Middleware(checker)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/hello", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !nextCalled {
		t.Fatalf("next should be called")
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestMiddlewareRejectsWhenUnhealthy(t *testing.T) {
	checker, err := NewChecker(Options{
		Name:     "demo",
		Target:   "http://localhost:8081",
		Path:     "/health",
		Interval: time.Second,
		Timeout:  time.Second,
	})
	if err != nil {
		t.Fatalf("NewChecker() error: %v", err)
	}

	checker.update(false, "test unhealthy")

	nextCalled := false

	handler := Middleware(checker)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/hello", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if nextCalled {
		t.Fatalf("next should not be called")
	}

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status got %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	if got := rec.Header().Get("X-Gateway-Backend-Health"); got != "unhealthy" {
		t.Fatalf("X-Gateway-Backend-Health got %q, want unhealthy", got)
	}
}

func TestMiddlewareAllowsWhenCheckerNil(t *testing.T) {
	nextCalled := false

	handler := Middleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/hello", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !nextCalled {
		t.Fatalf("next should be called when checker is nil")
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d, want %d", rec.Code, http.StatusOK)
	}
}
