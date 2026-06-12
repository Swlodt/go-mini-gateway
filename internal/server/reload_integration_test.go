package server

import (
	"context"
	"encoding/json"
	"fmt"
	"go-mini-gateway/internal/config"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGatewayAdminReloadRoutes(t *testing.T) {
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("backend-1"))
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("backend-2"))
	}))
	defer backend2.Close()

	mainAddr := freeLocalAddr(t)
	adminAddr := freeLocalAddr(t)
	configPath := writeReloadTestConfig(t, mainAddr, adminAddr, backend1.URL)

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	startTestGatewayWithConfigPath(t, cfg, configPath)

	assertGatewayBody(t, mainAddr, "backend-1")

	writeReloadTestConfigToPath(t, configPath, mainAddr, adminAddr, backend2.URL)

	resp := postAdminReload(t, adminAddr)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		t.Fatalf("reload status got %d, want 200, body=%s", resp.StatusCode, string(body))
	}
	defer resp.Body.Close()

	var result reloadResultDTO
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode reload response failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("reload success got false, message=%s", result.Message)
	}
	if len(result.Routes) != 1 {
		t.Fatalf("reload routes got %d, want 1", len(result.Routes))
	}

	assertGatewayBody(t, mainAddr, "backend-2")
}

func TestGatewayAdminReloadRejectsNonReloadableAddrChange(t *testing.T) {
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("backend-1"))
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("backend-2"))
	}))
	defer backend2.Close()

	mainAddr := freeLocalAddr(t)
	adminAddr := freeLocalAddr(t)
	configPath := writeReloadTestConfig(t, mainAddr, adminAddr, backend1.URL)

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	startTestGatewayWithConfigPath(t, cfg, configPath)

	assertGatewayBody(t, mainAddr, "backend-1")

	newMainAddr := freeLocalAddr(t)
	writeReloadTestConfigToPath(t, configPath, newMainAddr, adminAddr, backend2.URL)

	resp := postAdminReload(t, adminAddr)
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		t.Fatalf("reload status got %d, want 400, body=%s", resp.StatusCode, string(body))
	}
	_ = resp.Body.Close()

	assertGatewayBody(t, mainAddr, "backend-1")
}

func startTestGatewayWithConfigPath(t *testing.T, cfg *config.Config, configPath string) *Server {
	t.Helper()

	srv, err := NewWithConfigPath(cfg, configPath)
	if err != nil {
		t.Fatalf("server.NewWithConfigPath() error: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	waitHTTPReady(t, "http://"+cfg.Server.Addr+"/ping")

	t.Cleanup(func() {
		shutdownTestGateway(t, srv, errCh)
	})

	return srv
}

func shutdownTestGateway(t *testing.T, srv *Server, errCh <-chan error) {
	t.Helper()

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
}

func writeReloadTestConfig(t *testing.T, mainAddr string, adminAddr string, target string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "gateway.json")
	writeReloadTestConfigToPath(t, path, mainAddr, adminAddr, target)
	return path
}

func writeReloadTestConfigToPath(t *testing.T, path string, mainAddr string, adminAddr string, target string) {
	t.Helper()

	content := fmt.Sprintf(`{
  "server": {
    "addr": %q,
    "requestTimeout": "300ms",
    "shutdownTimeout": "2s",
    "rateLimitRPS": 0,
    "rateLimitBurst": 0,
    "maxConcurrency": 0
  },
  "admin": {
    "enabled": true,
    "addr": %q,
    "token": "test-token",
    "metricsRequireToken": false,
    "pprofEnabled": false
  },
  "routes": [
    {
      "id": "demo",
      "prefix": "/api/",
      "stripPrefix": "/api",
      "target": %q,
      "healthCheck": {
        "enabled": false
      }
    }
  ]
}`, mainAddr, adminAddr, target)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
}

func postAdminReload(t *testing.T, adminAddr string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, "http://"+adminAddr+"/admin/reload", nil)
	if err != nil {
		t.Fatalf("new reload request failed: %v", err)
	}
	req.Header.Set(adminTokenHeader, "test-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("reload request failed: %v", err)
	}
	return resp
}

func assertGatewayBody(t *testing.T, mainAddr string, want string) {
	t.Helper()

	resp, err := http.Get("http://" + mainAddr + "/api/hello")
	if err != nil {
		t.Fatalf("request gateway failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body failed: %v", err)
	}
	if string(body) != want {
		t.Fatalf("body got %q, want %q", string(body), want)
	}
}
