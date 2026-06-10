package metrics

import (
	"strings"
	"testing"
	"time"
)

func TestRegistryRecordAndSnapshot(t *testing.T) {
	registry := NewRegistry()

	registry.Record(Record{
		RouteID:     "demo",
		StatusCode:  200,
		ByteWritten: 100,
		Latency:     10 * time.Millisecond,
	})

	registry.Record(Record{
		RouteID:     "demo",
		StatusCode:  503,
		ByteWritten: 20,
		Latency:     30 * time.Millisecond,
	})

	registry.Record(Record{
		RouteID:     "admin",
		StatusCode:  200,
		ByteWritten: 50,
		Latency:     5 * time.Millisecond,
	})

	snapshot := registry.Snapshot()

	if snapshot.Total.Requests != 3 {
		t.Fatalf("total requests got %d, want 3", snapshot.Total.Requests)
	}

	if snapshot.Total.BytesWritten != 170 {
		t.Fatalf("total bytes got %d, want 170", snapshot.Total.BytesWritten)
	}

	if snapshot.Total.StatusCodes["200"] != 2 {
		t.Fatalf("total status 200 got %d, want 2", snapshot.Total.StatusCodes["200"])
	}

	if snapshot.Total.StatusCodes["503"] != 1 {
		t.Fatalf("total status 503 got %d, want 1", snapshot.Total.StatusCodes["503"])
	}

	demo := snapshot.Routes["demo"]

	if demo.Requests != 2 {
		t.Fatalf("demo requests got %d, want 2", demo.Requests)
	}

	if demo.BytesWritten != 120 {
		t.Fatalf("demo bytes got %d, want 120", demo.BytesWritten)
	}

	if demo.StatusCodes["200"] != 1 {
		t.Fatalf("demo status 200 got %d, want 1", demo.StatusCodes["200"])
	}

	if demo.StatusCodes["503"] != 1 {
		t.Fatalf("demo status 503 got %d, want 1", demo.StatusCodes["503"])
	}

	admin := snapshot.Routes["admin"]

	if admin.Requests != 1 {
		t.Fatalf("admin requests got %d, want 1", admin.Requests)
	}

	if admin.StatusCodes["200"] != 1 {
		t.Fatalf("admin status 200 got %d, want 1", admin.StatusCodes["200"])
	}
}

func TestRegistryLatencySnapshot(t *testing.T) {
	registry := NewRegistry()

	registry.Record(Record{
		RouteID:    "demo",
		StatusCode: 200,
		Latency:    10 * time.Millisecond,
	})

	registry.Record(Record{
		RouteID:    "demo",
		StatusCode: 200,
		Latency:    30 * time.Millisecond,
	})

	snapshot := registry.Snapshot()
	demo := snapshot.Routes["demo"]

	if demo.AvgLatencyMs != 20 {
		t.Fatalf("avg latency got %f, want 20", demo.AvgLatencyMs)
	}

	if demo.MaxLatencyMs != 30 {
		t.Fatalf("max latency got %f, want 30", demo.MaxLatencyMs)
	}
}

func TestRegistryDefaultValues(t *testing.T) {
	registry := NewRegistry()

	registry.Record(Record{
		RouteID:    "",
		StatusCode: 0,
		Latency:    time.Millisecond,
	})

	snapshot := registry.Snapshot()

	unknown := snapshot.Routes["unknown"]

	if unknown.Requests != 1 {
		t.Fatalf("unknown requests got %d, want 1", unknown.Requests)
	}

	if unknown.StatusCodes["200"] != 1 {
		t.Fatalf("unknown status 200 got %d, want 1", unknown.StatusCodes["200"])
	}
}

func TestNilRegistry(t *testing.T) {
	var registry *Registry

	registry.Record(Record{
		RouteID:    "demo",
		StatusCode: 200,
	})

	snapshot := registry.Snapshot()

	if snapshot.Total.Requests != 0 {
		t.Fatalf("nil registry total requests got %d, want 0", snapshot.Total.Requests)
	}

	text := registry.PrometheusText()
	if text != "" {
		t.Fatalf("nil registry prometheus text got %q, want empty", text)
	}
}

func TestPrometheusText(t *testing.T) {
	registry := NewRegistry()

	registry.Record(Record{
		RouteID:     "demo",
		StatusCode:  200,
		ByteWritten: 100,
		Latency:     10 * time.Millisecond,
	})

	registry.Record(Record{
		RouteID:     "demo",
		StatusCode:  429,
		ByteWritten: 20,
		Latency:     2 * time.Millisecond,
	})

	text := registry.PrometheusText()

	assertContains(t, text, "# HELP gateway_http_requests_total Total number of HTTP requests.")
	assertContains(t, text, "# TYPE gateway_http_requests_total counter")
	assertContains(t, text, "gateway_http_requests_total 2")

	assertContains(t, text, `gateway_http_status_requests_total{status="200"} 1`)
	assertContains(t, text, `gateway_http_status_requests_total{status="429"} 1`)

	assertContains(t, text, `gateway_route_http_requests_total{route="demo"} 2`)
	assertContains(t, text, `gateway_route_http_status_requests_total{route="demo",status="200"} 1`)
	assertContains(t, text, `gateway_route_http_status_requests_total{route="demo",status="429"} 1`)

	assertContains(t, text, "gateway_uptime_seconds")
	assertContains(t, text, "gateway_http_bytes_written_total 120")
}

func TestPrometheusLabelEscaping(t *testing.T) {
	registry := NewRegistry()

	registry.Record(Record{
		RouteID:    `demo"with\chars`,
		StatusCode: 200,
		Latency:    time.Millisecond,
	})

	text := registry.PrometheusText()

	assertContains(t, text, `route="demo\"with\\chars"`)
}

func assertContains(t *testing.T, value string, wantSubstring string) {
	t.Helper()

	if !strings.Contains(value, wantSubstring) {
		t.Fatalf("expected %q to contain %q", value, wantSubstring)
	}
}
