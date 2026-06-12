package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Setenv("TEST_GATEWAY_ADMIN_TOKEN", "dev-secret")
	t.Setenv("TEST_GATEWAY_BACKEND_URL", "http://localhost:8081")

	configContent := `{
  "server": {
    "addr": ":9000",
    "requestTimeout": "3s",
    "shutdownTimeout": "10s",
    "rateLimitRPS": 100,
    "rateLimitBurst": 100,
    "maxConcurrency": 100
  },
  "admin": {
    "enabled": true,
    "addr": "${TEST_GATEWAY_ADMIN_ADDR:127.0.0.1:9001}",
    "token": "${TEST_GATEWAY_ADMIN_TOKEN}",
    "metricsRequireToken": true
  },
  "routes": [
    {
      "id": "demo",
      "prefix": "/api/",
      "stripPrefix": "/api",
      "target": "${TEST_GATEWAY_BACKEND_URL}",
      "rateLimitRPS": 100,
      "rateLimitBurst": 100,
      "maxConcurrency": 10,
      "healthCheck": {
        "enabled": true,
        "path": "health",
        "interval": "3s",
        "timeout": "1s"
      }
    }
  ]
}`

	configPath := writeTempConfig(t, configContent)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Addr() != ":9000" {
		t.Fatalf("server addr got %q, want %q", cfg.Addr(), ":9000")
	}

	requestTimeout, err := cfg.RequestTimeoutDuration()
	if err != nil {
		t.Fatalf("RequestTimeoutDuration() error: %v", err)
	}
	if requestTimeout.String() != "3s" {
		t.Fatalf("request timeout got %s, want 3s", requestTimeout)
	}

	shutdownTimeout, err := cfg.ShutdownTimeoutDuration()
	if err != nil {
		t.Fatalf("ShutdownTimeoutDuration() error: %v", err)
	}
	if shutdownTimeout.String() != "10s" {
		t.Fatalf("shutdown timeout got %s, want 10s", shutdownTimeout)
	}

	if !cfg.Admin.Enabled {
		t.Fatalf("admin.enabled got false, want true")
	}

	if cfg.Admin.Addr != "127.0.0.1:9001" {
		t.Fatalf("admin.addr got %q, want %q", cfg.Admin.Addr, "127.0.0.1:9001")
	}

	if cfg.Admin.Token != "dev-secret" {
		t.Fatalf("admin.token got %q, want %q", cfg.Admin.Token, "dev-secret")
	}

	if !cfg.Admin.MetricsRequireToken {
		t.Fatalf("admin.metricsRequireToken got false, want true")
	}

	if len(cfg.Routes) != 1 {
		t.Fatalf("routes length got %d, want 1", len(cfg.Routes))
	}

	route := cfg.Routes[0]

	if route.ID != "demo" {
		t.Fatalf("route id got %q, want demo", route.ID)
	}

	if route.Prefix != "/api/" {
		t.Fatalf("route prefix got %q, want /api/", route.Prefix)
	}

	if route.StripPrefix != "/api" {
		t.Fatalf("route stripPrefix got %q, want /api", route.StripPrefix)
	}

	if route.Target != "http://localhost:8081" {
		t.Fatalf("route target got %q, want http://localhost:8081", route.Target)
	}

	if len(route.Upstreams) != 1 {
		t.Fatalf("route upstreams length got %d, want 1", len(route.Upstreams))
	}

	if route.Upstreams[0].ID != "default" {
		t.Fatalf("route upstream id got %q, want default", route.Upstreams[0].ID)
	}

	if route.Upstreams[0].URL != "http://localhost:8081" {
		t.Fatalf("route upstream url got %q, want http://localhost:8081", route.Upstreams[0].URL)
	}

	if route.HealthCheck.Path != "/health" {
		t.Fatalf("health path got %q, want /health", route.HealthCheck.Path)
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "gateway.json")

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp config failed: %v", err)
	}

	return path
}

func TestLoadWithUpstreams(t *testing.T) {
	t.Setenv("TEST_GATEWAY_BACKEND1", "http://localhost:8081")
	t.Setenv("TEST_GATEWAY_BACKEND2", "http://localhost:8082")

	configContent := `{
  "server": {
    "addr": ":9000"
  },
  "admin": {
    "enabled": false
  },
  "routes": [
    {
      "id": "demo",
      "prefix": "/api/",
      "stripPrefix": "/api",
      "upstreams": [
        {
          "id": "backend-1",
          "url": "${TEST_GATEWAY_BACKEND1}"
        },
        {
          "id": "backend-2",
          "url": "${TEST_GATEWAY_BACKEND2}"
        }
      ]
    }
  ]
}`

	cfg, err := Load(writeTempConfig(t, configContent))
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(cfg.Routes) != 1 {
		t.Fatalf("routes length got %d, want 1", len(cfg.Routes))
	}

	route := cfg.Routes[0]
	if route.Target != "http://localhost:8081" {
		t.Fatalf("route target got %q, want first upstream url", route.Target)
	}

	if len(route.Upstreams) != 2 {
		t.Fatalf("upstreams length got %d, want 2", len(route.Upstreams))
	}

	if route.Upstreams[0].ID != "backend-1" || route.Upstreams[0].URL != "http://localhost:8081" {
		t.Fatalf("first upstream got %+v", route.Upstreams[0])
	}

	if route.Upstreams[1].ID != "backend-2" || route.Upstreams[1].URL != "http://localhost:8082" {
		t.Fatalf("second upstream got %+v", route.Upstreams[1])
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name: "invalid json",
			content: `{
  "server": {
    "addr": ":9000"
  `,
		},
		{
			name: "missing required env",
			content: `{
  "server": {
    "addr": ":9000"
  },
  "admin": {
    "enabled": true,
    "addr": "127.0.0.1:9001",
    "token": "${TEST_GATEWAY_MISSING_TOKEN}"
  },
  "routes": [
    {
      "id": "demo",
      "prefix": "/api/",
      "target": "http://localhost:8081"
    }
  ]
}`,
		},
		{
			name: "admin enabled without token",
			content: `{
  "server": {
    "addr": ":9000"
  },
  "admin": {
    "enabled": true,
    "addr": "127.0.0.1:9001",
    "token": ""
  },
  "routes": [
    {
      "id": "demo",
      "prefix": "/api/",
      "target": "http://localhost:8081"
    }
  ]
}`,
		},
		{
			name: "duplicate route prefix",
			content: `{
  "server": {
    "addr": ":9000"
  },
  "admin": {
    "enabled": false
  },
  "routes": [
    {
      "id": "demo1",
      "prefix": "/api/",
      "target": "http://localhost:8081"
    },
    {
      "id": "demo2",
      "prefix": "/api/",
      "target": "http://localhost:8082"
    }
  ]
}`,
		},
		{
			name: "duplicate upstream id",
			content: `{
  "server": {
    "addr": ":9000"
  },
  "admin": {
    "enabled": false
  },
  "routes": [
    {
      "id": "demo",
      "prefix": "/api/",
      "upstreams": [
        {"id": "backend", "url": "http://localhost:8081"},
        {"id": "backend", "url": "http://localhost:8082"}
      ]
    }
  ]
}`,
		},
		{
			name: "invalid upstream url",
			content: `{
  "server": {
    "addr": ":9000"
  },
  "admin": {
    "enabled": false
  },
  "routes": [
    {
      "id": "demo",
      "prefix": "/api/",
      "upstreams": [
        {"id": "backend", "url": "localhost:8081"}
      ]
    }
  ]
}`,
		},
		{
			name: "invalid target",
			content: `{
  "server": {
    "addr": ":9000"
  },
  "admin": {
    "enabled": false
  },
  "routes": [
    {
      "id": "demo",
      "prefix": "/api/",
      "target": "localhost:8081"
    }
  ]
}`,
		},
		{
			name: "health check timeout greater than interval",
			content: `{
  "server": {
    "addr": ":9000"
  },
  "admin": {
    "enabled": false
  },
  "routes": [
    {
      "id": "demo",
      "prefix": "/api/",
      "target": "http://localhost:8081",
      "healthCheck": {
        "enabled": true,
        "path": "/health",
        "interval": "1s",
        "timeout": "2s"
      }
    }
  ]
}`,
		},
		{
			name: "invalid rate limit",
			content: `{
  "server": {
    "addr": ":9000",
    "rateLimitRPS": 0,
    "rateLimitBurst": 10
  },
  "admin": {
    "enabled": false
  },
  "routes": [
    {
      "id": "demo",
      "prefix": "/api/",
      "target": "http://localhost:8081"
    }
  ]
}`,
		},
		{
			name: "invalid max concurrency",
			content: `{
  "server": {
    "addr": ":9000",
    "maxConcurrency": -1
  },
  "admin": {
    "enabled": false
  },
  "routes": [
    {
      "id": "demo",
      "prefix": "/api/",
      "target": "http://localhost:8081"
    }
  ]
}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := writeTempConfig(t, tt.content)

			_, err := Load(configPath)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

func TestNormalizePrefix(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty",
			input: "",
			want:  "/",
		},
		{
			name:  "without leading slash",
			input: "api",
			want:  "/api/",
		},
		{
			name:  "with leading slash",
			input: "/api",
			want:  "/api/",
		},
		{
			name:  "with trailing slash",
			input: "/api/",
			want:  "/api/",
		},
		{
			name:  "root",
			input: "/",
			want:  "/",
		},
		{
			name:  "trim spaces",
			input: " api ",
			want:  "/api/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePrefix(tt.input)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeStripPrefix(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty",
			input: "",
			want:  "/",
		},
		{
			name:  "without leading slash",
			input: "api",
			want:  "/api",
		},
		{
			name:  "with leading slash",
			input: "/api",
			want:  "/api",
		},
		{
			name:  "with trailing slash",
			input: "/api/",
			want:  "/api",
		},
		{
			name:  "root",
			input: "/",
			want:  "/",
		},
		{
			name:  "trim spaces",
			input: " api ",
			want:  "/api",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeStripPrefix(tt.input)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateRateLimit(t *testing.T) {
	tests := []struct {
		name    string
		rps     int
		burst   int
		wantErr bool
	}{
		{
			name:  "disabled",
			rps:   0,
			burst: 0,
		},
		{
			name:  "valid",
			rps:   10,
			burst: 20,
		},
		{
			name:    "negative rps",
			rps:     -1,
			burst:   0,
			wantErr: true,
		},
		{
			name:    "negative burst",
			rps:     10,
			burst:   -1,
			wantErr: true,
		},
		{
			name:    "burst requires rps",
			rps:     0,
			burst:   10,
			wantErr: true,
		},
		{
			name:    "rps too large",
			rps:     100001,
			burst:   100001,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRateLimit("test", tt.rps, tt.burst)

			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}

			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateMaxConcurrency(t *testing.T) {
	tests := []struct {
		name           string
		maxConcurrency int
		wantErr        bool
	}{
		{
			name:           "disabled",
			maxConcurrency: 0,
		},
		{
			name:           "valid",
			maxConcurrency: 100,
		},
		{
			name:           "negative",
			maxConcurrency: -1,
			wantErr:        true,
		},
		{
			name:           "too large",
			maxConcurrency: 100001,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMaxConcurrency("test", tt.maxConcurrency)

			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}

			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateAdmin(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "disabled",
			cfg: Config{
				Admin: AdminConfig{
					Enabled: false,
				},
			},
		},
		{
			name: "enabled valid",
			cfg: Config{
				Admin: AdminConfig{
					Enabled: true,
					Addr:    "127.0.0.1:9001",
					Token:   "secret",
				},
			},
		},
		{
			name: "enabled without addr",
			cfg: Config{
				Admin: AdminConfig{
					Enabled: true,
					Token:   "secret",
				},
			},
			wantErr: true,
		},
		{
			name: "enabled without token",
			cfg: Config{
				Admin: AdminConfig{
					Enabled: true,
					Addr:    "127.0.0.1:9001",
				},
			},
			wantErr: true,
		},
		{
			name: "metrics require token but admin disabled",
			cfg: Config{
				Admin: AdminConfig{
					Enabled:             false,
					MetricsRequireToken: true,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAdmin(&tt.cfg)

			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}

			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateHealthCheck(t *testing.T) {
	tests := []struct {
		name        string
		healthCheck HealthCheckConfig
		wantErr     bool
	}{
		{
			name: "disabled",
			healthCheck: HealthCheckConfig{
				Enabled: false,
			},
		},
		{
			name: "enabled valid",
			healthCheck: HealthCheckConfig{
				Enabled:  true,
				Path:     "/health",
				Interval: "5s",
				Timeout:  "1s",
			},
		},
		{
			name: "enabled without path",
			healthCheck: HealthCheckConfig{
				Enabled:  true,
				Interval: "5s",
				Timeout:  "1s",
			},
			wantErr: true,
		},
		{
			name: "path without slash",
			healthCheck: HealthCheckConfig{
				Enabled:  true,
				Path:     "health",
				Interval: "5s",
				Timeout:  "1s",
			},
			wantErr: true,
		},
		{
			name: "invalid interval",
			healthCheck: HealthCheckConfig{
				Enabled:  true,
				Path:     "/health",
				Interval: "bad",
				Timeout:  "1s",
			},
			wantErr: true,
		},
		{
			name: "invalid timeout",
			healthCheck: HealthCheckConfig{
				Enabled:  true,
				Path:     "/health",
				Interval: "5s",
				Timeout:  "bad",
			},
			wantErr: true,
		},
		{
			name: "timeout equals interval",
			healthCheck: HealthCheckConfig{
				Enabled:  true,
				Path:     "/health",
				Interval: "5s",
				Timeout:  "5s",
			},
			wantErr: true,
		},
		{
			name: "timeout greater than interval",
			healthCheck: HealthCheckConfig{
				Enabled:  true,
				Path:     "/health",
				Interval: "1s",
				Timeout:  "5s",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHealthCheck("test", tt.healthCheck)

			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}

			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
