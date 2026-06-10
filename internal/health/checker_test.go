package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckerHealthy(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()

	checker, err := NewChecker(Options{
		Name:     "demo",
		Target:   backend.URL,
		Path:     "/health",
		Interval: time.Second,
		Timeout:  100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewChecker() error: %v", err)
	}

	checker.checkOnce()

	if !checker.IsHealthy() {
		t.Fatalf("checker should be healthy")
	}

	if !checker.Available() {
		t.Fatalf("checker should be available")
	}

	snapshot := checker.Snapshot()

	if snapshot.Name != "demo" {
		t.Fatalf("name got %q, want demo", snapshot.Name)
	}

	if snapshot.Target != backend.URL {
		t.Fatalf("target got %q, want %q", snapshot.Target, backend.URL)
	}

	if snapshot.Path != "/health" {
		t.Fatalf("path got %q, want /health", snapshot.Path)
	}

	if !snapshot.Checked {
		t.Fatalf("snapshot checked got false, want true")
	}

	if !snapshot.Healthy {
		t.Fatalf("snapshot healthy got false, want true")
	}

	if snapshot.LastReason == "" {
		t.Fatalf("last reason should not be empty")
	}

	if snapshot.LastCheckedAt == "" {
		t.Fatalf("last checked at should not be empty")
	}
}

func TestCheckerUnhealthyWhenStatusIsNot2xx(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}))
	defer backend.Close()

	checker, err := NewChecker(Options{
		Name:     "demo",
		Target:   backend.URL,
		Path:     "/health",
		Interval: time.Second,
		Timeout:  100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewChecker() error: %v", err)
	}

	checker.checkOnce()

	//if !checker.HasChecked() {
	//	t.Fatalf("checker should have checked")
	//}

	if checker.IsHealthy() {
		t.Fatalf("checker should be unhealthy")
	}

	if checker.Available() {
		t.Fatalf("checker should not be available")
	}

	snapshot := checker.Snapshot()

	if snapshot.Healthy {
		t.Fatalf("snapshot healthy got true, want false")
	}

	if snapshot.LastReason == "" {
		t.Fatalf("last reason should not be empty")
	}
}

func TestCheckerUnhealthyWhenBackendUnavailable(t *testing.T) {
	checker, err := NewChecker(Options{
		Name:     "demo",
		Target:   "http://127.0.0.1:1",
		Path:     "/health",
		Interval: time.Second,
		Timeout:  50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewChecker() error: %v", err)
	}

	checker.checkOnce()

	//if !checker.HasChecked() {
	//	t.Fatalf("checker should have checked")
	//}

	if checker.IsHealthy() {
		t.Fatalf("checker should be unhealthy")
	}

	if checker.Available() {
		t.Fatalf("checker should not be available")
	}
}

func TestCheckerAvailableBeforeFirstCheck(t *testing.T) {
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

	//if checker.HasChecked() {
	//	t.Fatalf("checker should not have checked")
	//}

	if !checker.Available() {
		t.Fatalf("checker should be available before first check")
	}
}

func TestNewCheckerErrors(t *testing.T) {
	tests := []struct {
		name    string
		options Options
	}{
		{
			name: "missing name",
			options: Options{
				Target:   "http://localhost:8081",
				Path:     "/health",
				Interval: time.Second,
				Timeout:  time.Second,
			},
		},
		{
			name: "invalid target",
			options: Options{
				Name:     "demo",
				Target:   "://bad",
				Path:     "/health",
				Interval: time.Second,
				Timeout:  time.Second,
			},
		},
		{
			name: "target without scheme",
			options: Options{
				Name:     "demo",
				Target:   "localhost:8081",
				Path:     "/health",
				Interval: time.Second,
				Timeout:  time.Second,
			},
		},
		{
			name: "invalid interval",
			options: Options{
				Name:     "demo",
				Target:   "http://localhost:8081",
				Path:     "/health",
				Interval: 0,
				Timeout:  time.Second,
			},
		},
		{
			name: "invalid timeout",
			options: Options{
				Name:     "demo",
				Target:   "http://localhost:8081",
				Path:     "/health",
				Interval: time.Second,
				Timeout:  0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewChecker(tt.options)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}
