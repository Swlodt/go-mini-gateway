package server

import (
	"context"
	"encoding/json"
	"go-mini-gateway/internal/config"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestGatewayConfig(mainAddr string, adminAddr string, target string) *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			Addr:            mainAddr,
			RequestTimeout:  "300ms",
			ShutdownTimeout: "2s",
			RateLimitRPS:    0,
			RateLimitBurst:  0,
			MaxConcurrency:  0,
		},
		Admin: config.AdminConfig{
			Enabled:             true,
			Addr:                adminAddr,
			Token:               "test-token",
			MetricsRequireToken: false,
		},
		Routes: []config.RouteConfig{
			{
				ID:          "demo",
				Prefix:      "/api/",
				StripPrefix: "/api",
				Target:      target,
			},
		},
	}
}

func freeLocalAddr(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen free port failed: %v", err)
	}
	defer func(listener net.Listener) {
		err := listener.Close()
		if err != nil {

		}
	}(listener)

	return listener.Addr().String()
}

func startTestGateway(t *testing.T, cfg *config.Config) *Server {
	t.Helper()

	srv, err := New(cfg)
	if err != nil {
		t.Fatalf("server.New() error: %v", err)
	}

	errCh := make(chan error, 1)

	go func() {
		errCh <- srv.Start()
	}()

	waitHTTPReady(t, "http://"+cfg.Server.Addr+"/ping")

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			t.Logf("gateway shutdown error: %v", err)
			_ = srv.Close()
		}

		select {
		case err := <-errCh:
			if err != nil {
				t.Logf("gateway start returned error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Logf("gateway start did not return in time")
		}
	})

	return srv
}

func waitHTTPReady(t *testing.T, url string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)

	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("server not ready: %s", url)
}

func TestGatewayProxyAndStripPrefix(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hello" {
			t.Fatalf("backend path got %q, want /hello", r.URL.Path)
		}

		if r.URL.RawQuery != "name=sw" {
			t.Fatalf("backend query got %q, want name=sw", r.URL.RawQuery)
		}

		w.Header().Set("X-Backend", "demo-backend")
		_, _ = w.Write([]byte("hello from backend"))
	}))
	defer backend.Close()

	mainAddr := freeLocalAddr(t)
	adminAddr := freeLocalAddr(t)

	cfg := newTestGatewayConfig(mainAddr, adminAddr, backend.URL)
	startTestGateway(t, cfg)

	resp, err := http.Get("http://" + mainAddr + "/api/hello?name=sw")
	if err != nil {
		t.Fatalf("request gateway failed: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if got := resp.Header.Get("X-Backend"); got != "demo-backend" {
		t.Fatalf("X-Backend got %q, want demo-backend", got)
	}

	if got := resp.Header.Get("X-Gateway"); got != "go-mini-gateway" {
		t.Fatalf("X-Gateway got %q, want go-mini-gateway", got)
	}
}

func TestGatewayProxyHeadersAndTraceID(t *testing.T) {
	var gotGateway string
	var gotRoute string
	var gotTraceID string
	var gotForwardedFor string
	var gotForwardedHost string
	var gotForwardedProto string

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotGateway = r.Header.Get("X-Gateway")
		gotRoute = r.Header.Get("X-Gateway-Route")
		gotTraceID = r.Header.Get("X-Trace-ID")
		gotForwardedFor = r.Header.Get("X-Forwarded-For")
		gotForwardedHost = r.Header.Get("X-Forwarded-Host")
		gotForwardedProto = r.Header.Get("X-Forwarded-Proto")

		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()

	mainAddr := freeLocalAddr(t)
	adminAddr := freeLocalAddr(t)

	cfg := newTestGatewayConfig(mainAddr, adminAddr, backend.URL)
	startTestGateway(t, cfg)

	req, err := http.NewRequest(http.MethodGet, "http://"+mainAddr+"/api/hello", nil)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}

	req.Header.Set("X-Trace-ID", "trace-from-client")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request gateway failed: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	if gotGateway != "go-mini-gateway" {
		t.Fatalf("X-Gateway got %q, want go-mini-gateway", gotGateway)
	}

	if gotRoute != "demo" {
		t.Fatalf("X-Gateway-Route got %q, want demo", gotRoute)
	}

	if gotTraceID != "trace-from-client" {
		t.Fatalf("X-Trace-ID got %q, want trace-from-client", gotTraceID)
	}

	if gotForwardedFor == "" {
		t.Fatalf("X-Forwarded-For should not be empty")
	}

	if gotForwardedHost == "" {
		t.Fatalf("X-Forwarded-Host should not be empty")
	}

	if gotForwardedProto == "" {
		t.Fatalf("X-Forwarded-Proto should not be empty")
	}
}

func TestGatewayGeneratesTraceID(t *testing.T) {
	var gotTraceID string

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTraceID = r.Header.Get("X-Trace-ID")
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()

	mainAddr := freeLocalAddr(t)
	adminAddr := freeLocalAddr(t)

	cfg := newTestGatewayConfig(mainAddr, adminAddr, backend.URL)
	startTestGateway(t, cfg)

	resp, err := http.Get("http://" + mainAddr + "/api/hello")
	if err != nil {
		t.Fatalf("request gateway failed: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	if gotTraceID == "" {
		t.Fatalf("X-Trace-ID should be generated")
	}
}

func TestGatewayPreservesBackendStatus(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "backend busy", http.StatusServiceUnavailable)
	}))
	defer backend.Close()

	mainAddr := freeLocalAddr(t)
	adminAddr := freeLocalAddr(t)

	cfg := newTestGatewayConfig(mainAddr, adminAddr, backend.URL)
	startTestGateway(t, cfg)

	resp, err := http.Get("http://" + mainAddr + "/api/error")
	if err != nil {
		t.Fatalf("request gateway failed: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status got %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
}

func TestGatewayReturns502WhenBackendUnavailable(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	target := backend.URL
	backend.Close()

	mainAddr := freeLocalAddr(t)
	adminAddr := freeLocalAddr(t)

	cfg := newTestGatewayConfig(mainAddr, adminAddr, target)
	startTestGateway(t, cfg)

	resp, err := http.Get("http://" + mainAddr + "/api/hello")
	if err != nil {
		t.Fatalf("request gateway failed: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status got %d, want %d", resp.StatusCode, http.StatusBadGateway)
	}
}

func TestGatewayReturns504WhenBackendSlow(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Second)
		_, _ = w.Write([]byte("slow"))
	}))
	defer backend.Close()

	mainAddr := freeLocalAddr(t)
	adminAddr := freeLocalAddr(t)

	cfg := newTestGatewayConfig(mainAddr, adminAddr, backend.URL)
	cfg.Server.RequestTimeout = "100ms"

	startTestGateway(t, cfg)

	resp, err := http.Get("http://" + mainAddr + "/api/slow")
	if err != nil {
		t.Fatalf("request gateway failed: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("status got %d, want %d", resp.StatusCode, http.StatusGatewayTimeout)
	}
}

func TestGatewayRouteRateLimit(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()

	mainAddr := freeLocalAddr(t)
	adminAddr := freeLocalAddr(t)

	cfg := newTestGatewayConfig(mainAddr, adminAddr, backend.URL)
	cfg.Routes[0].RateLimitRPS = 1
	cfg.Routes[0].RateLimitBurst = 1

	startTestGateway(t, cfg)

	resp1, err := http.Get("http://" + mainAddr + "/api/hello")
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	_ = resp1.Body.Close()

	resp2, err := http.Get("http://" + mainAddr + "/api/hello")
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp2.Body)

	if resp2.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second status got %d, want %d", resp2.StatusCode, http.StatusTooManyRequests)
	}
}

func TestGatewayRouteConcurrencyLimit(t *testing.T) {
	backendEntered := make(chan struct{})
	releaseBackend := make(chan struct{})

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-backendEntered:
		default:
			close(backendEntered)
		}

		<-releaseBackend
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()

	mainAddr := freeLocalAddr(t)
	adminAddr := freeLocalAddr(t)

	cfg := newTestGatewayConfig(mainAddr, adminAddr, backend.URL)
	cfg.Routes[0].MaxConcurrency = 1
	cfg.Server.RequestTimeout = "2s"

	startTestGateway(t, cfg)

	firstDone := make(chan struct{})

	go func() {
		resp, err := http.Get("http://" + mainAddr + "/api/slow")
		if err == nil {
			_ = resp.Body.Close()
		}
		close(firstDone)
	}()

	select {
	case <-backendEntered:
	case <-time.After(time.Second):
		t.Fatalf("first request did not enter backend")
	}

	resp2, err := http.Get("http://" + mainAddr + "/api/slow")
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	_ = resp2.Body.Close()

	if resp2.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("second status got %d, want %d", resp2.StatusCode, http.StatusServiceUnavailable)
	}

	close(releaseBackend)

	select {
	case <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("first request did not finish")
	}
}

func TestGatewayHealthCheckUnhealthy(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}

		_, _ = w.Write([]byte("business ok"))
	}))
	defer backend.Close()

	mainAddr := freeLocalAddr(t)
	adminAddr := freeLocalAddr(t)

	cfg := newTestGatewayConfig(mainAddr, adminAddr, backend.URL)
	cfg.Routes[0].HealthCheck = config.HealthCheckConfig{
		Enabled:  true,
		Path:     "/health",
		Interval: "50ms",
		Timeout:  "20ms",
	}

	startTestGateway(t, cfg)

	waitUntil(t, time.Second, func() bool {
		resp, err := http.Get("http://" + mainAddr + "/api/hello")
		if err != nil {
			return false
		}
		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {

			}
		}(resp.Body)

		return resp.StatusCode == http.StatusServiceUnavailable
	})

	resp, err := http.Get("http://" + mainAddr + "/api/hello")
	if err != nil {
		t.Fatalf("request gateway failed: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status got %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
}

func waitUntil(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if condition() {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("condition not met within %s", timeout)
}

func TestGatewayAdminServerSeparated(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()

	mainAddr := freeLocalAddr(t)
	adminAddr := freeLocalAddr(t)

	cfg := newTestGatewayConfig(mainAddr, adminAddr, backend.URL)
	startTestGateway(t, cfg)

	resp, err := http.Get("http://" + mainAddr + "/admin/routes")
	if err != nil {
		t.Fatalf("request main admin path failed: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("main /admin/routes status got %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	resp, err = http.Get("http://" + mainAddr + "/metrics")
	if err != nil {
		t.Fatalf("request main metrics failed: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("main /metrics status got %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	resp, err = http.Get("http://" + adminAddr + "/admin/routes")
	if err != nil {
		t.Fatalf("request admin routes failed: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("admin /admin/routes without token got %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	req, err := http.NewRequest(http.MethodGet, "http://"+adminAddr+"/admin/routes", nil)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	req.Header.Set(adminTokenHeader, "test-token")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request admin routes with token failed: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin /admin/routes with token got %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestGatewayAdminRoutesResponse(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()

	mainAddr := freeLocalAddr(t)
	adminAddr := freeLocalAddr(t)

	cfg := newTestGatewayConfig(mainAddr, adminAddr, backend.URL)
	startTestGateway(t, cfg)

	req, err := http.NewRequest(http.MethodGet, "http://"+adminAddr+"/admin/routes", nil)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	req.Header.Set(adminTokenHeader, "test-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request admin routes failed: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var routes []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&routes); err != nil {
		t.Fatalf("decode routes failed: %v", err)
	}

	if len(routes) != 1 {
		t.Fatalf("routes length got %d, want 1", len(routes))
	}

	if routes[0]["id"] != "demo" {
		t.Fatalf("route id got %v, want demo", routes[0]["id"])
	}

	if routes[0]["prefix"] != "/api/" {
		t.Fatalf("route prefix got %v, want /api/", routes[0]["prefix"])
	}
}

func TestGatewayMetricsRecordsRoute(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()

	mainAddr := freeLocalAddr(t)
	adminAddr := freeLocalAddr(t)

	cfg := newTestGatewayConfig(mainAddr, adminAddr, backend.URL)
	startTestGateway(t, cfg)

	resp, err := http.Get("http://" + mainAddr + "/api/hello")
	if err != nil {
		t.Fatalf("request gateway failed: %v", err)
	}
	_ = resp.Body.Close()

	req, err := http.NewRequest(http.MethodGet, "http://"+adminAddr+"/admin/metrics", nil)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	req.Header.Set(adminTokenHeader, "test-token")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request admin metrics failed: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode metrics failed: %v", err)
	}

	routes, ok := body["routes"].(map[string]any)
	if !ok {
		t.Fatalf("routes field missing or invalid")
	}

	demo, ok := routes["demo"].(map[string]any)
	if !ok {
		t.Fatalf("demo route metrics missing")
	}

	requests, ok := demo["requests"].(float64)
	if !ok {
		t.Fatalf("demo.requests missing or invalid")
	}

	if requests < 1 {
		t.Fatalf("demo.requests got %f, want >= 1", requests)
	}
}

func TestGatewayPprofEnabled(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()

	mainAddr := freeLocalAddr(t)
	adminAddr := freeLocalAddr(t)

	cfg := newTestGatewayConfig(mainAddr, adminAddr, backend.URL)
	cfg.Admin.PprofEnabled = true

	startTestGateway(t, cfg)

	req, err := http.NewRequest(http.MethodGet, "http://"+adminAddr+"/debug/pprof/", nil)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	req.Header.Set(adminTokenHeader, "test-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request pprof failed: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status got %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestGatewayPprofDisabled(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()

	mainAddr := freeLocalAddr(t)
	adminAddr := freeLocalAddr(t)

	cfg := newTestGatewayConfig(mainAddr, adminAddr, backend.URL)
	cfg.Admin.PprofEnabled = false

	startTestGateway(t, cfg)

	req, err := http.NewRequest(http.MethodGet, "http://"+adminAddr+"/debug/pprof/", nil)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	req.Header.Set(adminTokenHeader, "test-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request pprof failed: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status got %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestGatewayRoundRobinUpstreams(t *testing.T) {
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend-ID", "backend-1")
		_, _ = w.Write([]byte("backend-1"))
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend-ID", "backend-2")
		_, _ = w.Write([]byte("backend-2"))
	}))
	defer backend2.Close()

	mainAddr := freeLocalAddr(t)
	adminAddr := freeLocalAddr(t)

	cfg := newTestGatewayConfig(mainAddr, adminAddr, "")
	cfg.Routes[0].Target = backend1.URL
	cfg.Routes[0].Upstreams = []config.UpstreamConfig{
		{
			ID:  "backend-1",
			URL: backend1.URL,
		},
		{
			ID:  "backend-2",
			URL: backend2.URL,
		},
	}

	startTestGateway(t, cfg)

	counts := map[string]int{}
	upstreamHeaders := map[string]int{}
	for i := 0; i < 6; i++ {
		resp, err := http.Get("http://" + mainAddr + "/api/hello")
		if err != nil {
			t.Fatalf("request gateway failed: %v", err)
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatalf("read response body failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status got %d, want %d", resp.StatusCode, http.StatusOK)
		}

		counts[string(body)]++
		upstreamHeaders[resp.Header.Get("X-Gateway-Upstream")]++
	}

	if counts["backend-1"] != 3 {
		t.Fatalf("backend-1 count got %d, want 3, counts=%v", counts["backend-1"], counts)
	}

	if counts["backend-2"] != 3 {
		t.Fatalf("backend-2 count got %d, want 3, counts=%v", counts["backend-2"], counts)
	}

	if upstreamHeaders["backend-1"] != 3 {
		t.Fatalf("X-Gateway-Upstream backend-1 count got %d, want 3, headers=%v", upstreamHeaders["backend-1"], upstreamHeaders)
	}

	if upstreamHeaders["backend-2"] != 3 {
		t.Fatalf("X-Gateway-Upstream backend-2 count got %d, want 3, headers=%v", upstreamHeaders["backend-2"], upstreamHeaders)
	}
}

func TestGatewayPassiveHealthSkipsFailedUpstream(t *testing.T) {
	backend1Requests := 0
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backend1Requests++
		w.Header().Set("X-Backend-ID", "backend-1")
		http.Error(w, "backend-1 failed", http.StatusInternalServerError)
	}))
	defer backend1.Close()

	backend2Requests := 0
	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backend2Requests++
		w.Header().Set("X-Backend-ID", "backend-2")
		_, _ = w.Write([]byte("backend-2"))
	}))
	defer backend2.Close()

	mainAddr := freeLocalAddr(t)
	adminAddr := freeLocalAddr(t)

	cfg := newTestGatewayConfig(mainAddr, adminAddr, "")
	cfg.Routes[0].Target = backend1.URL
	cfg.Routes[0].Upstreams = []config.UpstreamConfig{
		{
			ID:  "backend-1",
			URL: backend1.URL,
		},
		{
			ID:  "backend-2",
			URL: backend2.URL,
		},
	}
	cfg.Routes[0].PassiveHealth = config.PassiveHealthConfig{
		Enabled:           true,
		FailureThreshold:  1,
		SuccessThreshold:  1,
		UnhealthyDuration: time.Hour.String(),
	}

	startTestGateway(t, cfg)

	resp, err := http.Get("http://" + mainAddr + "/api/hello")
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("first status got %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
	if got := resp.Header.Get("X-Gateway-Upstream"); got != "backend-1" {
		t.Fatalf("first X-Gateway-Upstream got %q, want backend-1", got)
	}

	for i := 0; i < 5; i++ {
		resp, err := http.Get("http://" + mainAddr + "/api/hello")
		if err != nil {
			t.Fatalf("request %d failed: %v", i+2, err)
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatalf("read response body failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d status got %d, want %d", i+2, resp.StatusCode, http.StatusOK)
		}
		if got := resp.Header.Get("X-Gateway-Upstream"); got != "backend-2" {
			t.Fatalf("request %d X-Gateway-Upstream got %q, want backend-2", i+2, got)
		}
		if string(body) != "backend-2" {
			t.Fatalf("request %d body got %q, want backend-2", i+2, string(body))
		}
	}

	if backend1Requests != 1 {
		t.Fatalf("backend1 requests got %d, want 1", backend1Requests)
	}
	if backend2Requests != 5 {
		t.Fatalf("backend2 requests got %d, want 5", backend2Requests)
	}
}

func TestGatewayPassiveHealthNoAvailableUpstream(t *testing.T) {
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "backend-1 failed", http.StatusInternalServerError)
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "backend-2 failed", http.StatusInternalServerError)
	}))
	defer backend2.Close()

	mainAddr := freeLocalAddr(t)
	adminAddr := freeLocalAddr(t)

	cfg := newTestGatewayConfig(mainAddr, adminAddr, "")
	cfg.Routes[0].Target = backend1.URL
	cfg.Routes[0].Upstreams = []config.UpstreamConfig{
		{
			ID:  "backend-1",
			URL: backend1.URL,
		},
		{
			ID:  "backend-2",
			URL: backend2.URL,
		},
	}
	cfg.Routes[0].PassiveHealth = config.PassiveHealthConfig{
		Enabled:           true,
		FailureThreshold:  1,
		SuccessThreshold:  1,
		UnhealthyDuration: time.Hour.String(),
	}

	startTestGateway(t, cfg)

	for i := 0; i < 2; i++ {
		resp, err := http.Get("http://" + mainAddr + "/api/hello")
		if err != nil {
			t.Fatalf("request %d failed: %v", i+1, err)
		}
		_ = resp.Body.Close()

		if resp.StatusCode != http.StatusInternalServerError {
			t.Fatalf("request %d status got %d, want %d", i+1, resp.StatusCode, http.StatusInternalServerError)
		}
	}

	resp, err := http.Get("http://" + mainAddr + "/api/hello")
	if err != nil {
		t.Fatalf("third request failed: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("third status got %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
}

func TestGatewayCircuitBreakerSkipsOpenUpstream(t *testing.T) {
	backend1Requests := 0
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backend1Requests++
		w.Header().Set("X-Backend-ID", "backend-1")
		http.Error(w, "backend-1 failed", http.StatusInternalServerError)
	}))
	defer backend1.Close()

	backend2Requests := 0
	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backend2Requests++
		w.Header().Set("X-Backend-ID", "backend-2")
		_, _ = w.Write([]byte("backend-2"))
	}))
	defer backend2.Close()

	mainAddr := freeLocalAddr(t)
	adminAddr := freeLocalAddr(t)

	cfg := newTestGatewayConfig(mainAddr, adminAddr, "")
	cfg.Routes[0].Target = backend1.URL
	cfg.Routes[0].Upstreams = []config.UpstreamConfig{
		{
			ID:  "backend-1",
			URL: backend1.URL,
		},
		{
			ID:  "backend-2",
			URL: backend2.URL,
		},
	}
	cfg.Routes[0].CircuitBreaker = config.CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    1,
		OpenDuration:        time.Hour.String(),
		HalfOpenMaxRequests: 1,
	}

	startTestGateway(t, cfg)

	resp, err := http.Get("http://" + mainAddr + "/api/hello")
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("first status got %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
	if got := resp.Header.Get("X-Gateway-Upstream"); got != "backend-1" {
		t.Fatalf("first X-Gateway-Upstream got %q, want backend-1", got)
	}

	for i := 0; i < 5; i++ {
		resp, err := http.Get("http://" + mainAddr + "/api/hello")
		if err != nil {
			t.Fatalf("request %d failed: %v", i+2, err)
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatalf("read response body failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d status got %d, want %d", i+2, resp.StatusCode, http.StatusOK)
		}
		if got := resp.Header.Get("X-Gateway-Upstream"); got != "backend-2" {
			t.Fatalf("request %d X-Gateway-Upstream got %q, want backend-2", i+2, got)
		}
		if string(body) != "backend-2" {
			t.Fatalf("request %d body got %q, want backend-2", i+2, string(body))
		}
	}

	if backend1Requests != 1 {
		t.Fatalf("backend1 requests got %d, want 1", backend1Requests)
	}
	if backend2Requests != 5 {
		t.Fatalf("backend2 requests got %d, want 5", backend2Requests)
	}
}

func TestGatewayCircuitBreakerNoAvailableUpstream(t *testing.T) {
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "backend-1 failed", http.StatusInternalServerError)
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "backend-2 failed", http.StatusInternalServerError)
	}))
	defer backend2.Close()

	mainAddr := freeLocalAddr(t)
	adminAddr := freeLocalAddr(t)

	cfg := newTestGatewayConfig(mainAddr, adminAddr, "")
	cfg.Routes[0].Target = backend1.URL
	cfg.Routes[0].Upstreams = []config.UpstreamConfig{
		{
			ID:  "backend-1",
			URL: backend1.URL,
		},
		{
			ID:  "backend-2",
			URL: backend2.URL,
		},
	}
	cfg.Routes[0].CircuitBreaker = config.CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    1,
		OpenDuration:        time.Hour.String(),
		HalfOpenMaxRequests: 1,
	}

	startTestGateway(t, cfg)

	for i := 0; i < 2; i++ {
		resp, err := http.Get("http://" + mainAddr + "/api/hello")
		if err != nil {
			t.Fatalf("request %d failed: %v", i+1, err)
		}
		_ = resp.Body.Close()

		if resp.StatusCode != http.StatusInternalServerError {
			t.Fatalf("request %d status got %d, want %d", i+1, resp.StatusCode, http.StatusInternalServerError)
		}
	}

	resp, err := http.Get("http://" + mainAddr + "/api/hello")
	if err != nil {
		t.Fatalf("third request failed: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("third status got %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
}

func TestGatewayCircuitBreakerHalfOpenRecovery(t *testing.T) {
	backendRequests := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendRequests++
		if backendRequests == 1 {
			http.Error(w, "first request failed", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer backend.Close()

	mainAddr := freeLocalAddr(t)
	adminAddr := freeLocalAddr(t)

	cfg := newTestGatewayConfig(mainAddr, adminAddr, backend.URL)
	cfg.Routes[0].CircuitBreaker = config.CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    1,
		OpenDuration:        "50ms",
		HalfOpenMaxRequests: 1,
	}

	startTestGateway(t, cfg)

	resp, err := http.Get("http://" + mainAddr + "/api/hello")
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("first status got %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}

	resp, err = http.Get("http://" + mainAddr + "/api/hello")
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("second status got %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
	if backendRequests != 1 {
		t.Fatalf("backend requests got %d, want 1 before half-open trial", backendRequests)
	}

	time.Sleep(80 * time.Millisecond)

	resp, err = http.Get("http://" + mainAddr + "/api/hello")
	if err != nil {
		t.Fatalf("third request failed: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read response body failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("third status got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if string(body) != "recovered" {
		t.Fatalf("third body got %q, want recovered", string(body))
	}

	resp, err = http.Get("http://" + mainAddr + "/api/hello")
	if err != nil {
		t.Fatalf("fourth request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fourth status got %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestGatewayActiveHealthSkipsUnhealthyFirstUpstream(t *testing.T) {
	backend1BusinessRequests := 0
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			http.Error(w, "backend-1 not ready", http.StatusServiceUnavailable)
			return
		}
		backend1BusinessRequests++
		w.Header().Set("X-Backend-ID", "backend-1")
		_, _ = w.Write([]byte("backend-1"))
	}))
	defer backend1.Close()

	backend2BusinessRequests := 0
	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte("ok"))
			return
		}
		backend2BusinessRequests++
		w.Header().Set("X-Backend-ID", "backend-2")
		_, _ = w.Write([]byte("backend-2"))
	}))
	defer backend2.Close()

	mainAddr := freeLocalAddr(t)
	adminAddr := freeLocalAddr(t)

	cfg := newTestGatewayConfig(mainAddr, adminAddr, "")
	cfg.Routes[0].Target = backend1.URL
	cfg.Routes[0].Upstreams = []config.UpstreamConfig{
		{ID: "backend-1", URL: backend1.URL},
		{ID: "backend-2", URL: backend2.URL},
	}
	cfg.Routes[0].HealthCheck = config.HealthCheckConfig{
		Enabled:  true,
		Path:     "/health",
		Interval: "30ms",
		Timeout:  "10ms",
	}

	startTestGateway(t, cfg)

	waitUntil(t, time.Second, func() bool {
		upstreams := fetchAdminRouteUpstreams(t, adminAddr)
		backend1Health := upstreams["backend-1"]
		backend2Health := upstreams["backend-2"]
		return backend1Health != nil && backend1Health["checked"] == true && backend1Health["healthy"] == false &&
			backend2Health != nil && backend2Health["checked"] == true && backend2Health["healthy"] == true
	})

	for i := 0; i < 5; i++ {
		resp, err := http.Get("http://" + mainAddr + "/api/hello")
		if err != nil {
			t.Fatalf("request %d failed: %v", i+1, err)
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatalf("read response body failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d status got %d, want %d", i+1, resp.StatusCode, http.StatusOK)
		}
		if got := resp.Header.Get("X-Gateway-Upstream"); got != "backend-2" {
			t.Fatalf("request %d X-Gateway-Upstream got %q, want backend-2", i+1, got)
		}
		if string(body) != "backend-2" {
			t.Fatalf("request %d body got %q, want backend-2", i+1, string(body))
		}
	}

	if backend1BusinessRequests != 0 {
		t.Fatalf("backend1 business requests got %d, want 0", backend1BusinessRequests)
	}
	if backend2BusinessRequests != 5 {
		t.Fatalf("backend2 business requests got %d, want 5", backend2BusinessRequests)
	}
}

func fetchAdminRouteUpstreams(t *testing.T, adminAddr string) map[string]map[string]any {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, "http://"+adminAddr+"/admin/routes", nil)
	if err != nil {
		t.Fatalf("new admin routes request failed: %v", err)
	}
	req.Header.Set(adminTokenHeader, "test-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request admin routes failed: %v", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin routes status got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var routes []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&routes); err != nil {
		t.Fatalf("decode admin routes failed: %v", err)
	}
	if len(routes) == 0 {
		t.Fatalf("admin routes is empty")
	}

	rawUpstreams, ok := routes[0]["upstreams"].([]any)
	if !ok {
		t.Fatalf("upstreams field missing or invalid: %#v", routes[0]["upstreams"])
	}

	result := make(map[string]map[string]any, len(rawUpstreams))
	for _, raw := range rawUpstreams {
		upstream, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("upstream item invalid: %#v", raw)
		}
		id, ok := upstream["id"].(string)
		if !ok || id == "" {
			t.Fatalf("upstream id missing or invalid: %#v", upstream)
		}
		activeHealth, ok := upstream["activeHealth"].(map[string]any)
		if !ok {
			continue
		}
		result[id] = activeHealth
	}

	return result
}
