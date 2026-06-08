package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultAddr            = ":8080"
	defaultRequestTimeout  = 3 * time.Second
	defaultShutdownTimeout = 10 * time.Second
)

type Config struct {
	Server ServerConfig  `json:"server"`
	Admin  AdminConfig   `json:"admin"`
	Routes []RouteConfig `json:"routes"`
}

type ServerConfig struct {
	Addr            string `json:"addr"`
	RequestTimeout  string `json:"requestTimeout"`
	ShutdownTimeout string `json:"shutdownTimeout"`
	RateLimitRPS    int    `json:"rateLimitRPS"`
	RateLimitBurst  int    `json:"rateLimitBurst"`
	MaxConcurrency  int    `json:"maxConcurrency"`
}

type AdminConfig struct {
	Enable              bool   `json:"enable"`
	Token               string `json:"token"`
	MetricsRequireToken bool   `json:"metricsRequireToken"`
}

type RouteConfig struct {
	ID             string            `json:"id"`
	Prefix         string            `json:"prefix"`
	StripPrefix    string            `json:"StripPrefix"`
	Target         string            `json:"target"`
	RateLimitRPS   int               `json:"rateLimitRPS"`
	RateLimitBurst int               `json:"rateLimitBurst"`
	MaxConcurrency int               `json:"maxConcurrency"`
	HealthCheck    HealthCheckConfig `json:"healthCheck"`
}

type HealthCheckConfig struct {
	Enabled  bool   `json:"enabled"`
	Path     string `json:"path"`
	Interval string `json:"interval"`
	Timeout  string `json:"timeout"`
}

func (c *Config) Addr() string {
	if c.Server.Addr == "" {
		return defaultAddr
	}
	return c.Server.Addr
}

func (c *Config) RequestTimeoutDuration() (time.Duration, error) {
	if c.Server.RequestTimeout == "" {
		return defaultRequestTimeout, nil
	}
	timeout, err := time.ParseDuration(c.Server.RequestTimeout)
	if err != nil {
		return 0, fmt.Errorf("invalid server.requestTimeout %q: %w", c.Server.RequestTimeout, err)
	}
	return timeout, nil
}

func (c *Config) ShutdownTimeoutDuration() (time.Duration, error) {
	if c.Server.ShutdownTimeout == "" {
		return defaultShutdownTimeout, nil
	}
	timeout, err := time.ParseDuration(c.Server.ShutdownTimeout)
	if err != nil {
		return 0, fmt.Errorf("invalid server.shutdownTimeout %q: %w", c.Server.ShutdownTimeout, err)
	}
	return timeout, nil
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %q failed: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file %q failed: %w", path, err)
	}

	normalize(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func normalize(cfg *Config) {
	if cfg.Server.Addr == "" {
		cfg.Server.Addr = defaultAddr
	}
	if cfg.Server.RequestTimeout == "" {
		cfg.Server.RequestTimeout = defaultRequestTimeout.String()
	}
	if cfg.Server.ShutdownTimeout == "" {
		cfg.Server.ShutdownTimeout = defaultShutdownTimeout.String()
	}
	if cfg.Server.RateLimitRPS > 0 && cfg.Server.RateLimitBurst <= 0 {
		cfg.Server.RateLimitBurst = cfg.Server.RateLimitRPS
	}

	for i := range cfg.Routes {
		cfg.Routes[i].ID = strings.TrimSpace(cfg.Routes[i].ID)
		cfg.Routes[i].Prefix = normalizePrefix(cfg.Routes[i].Prefix)
		cfg.Routes[i].StripPrefix = normalizeStripPrefix(cfg.Routes[i].StripPrefix)
		cfg.Routes[i].Target = strings.TrimRight(strings.TrimSpace(cfg.Routes[i].Target), "/")

		if cfg.Routes[i].RateLimitRPS > 0 && cfg.Routes[i].RateLimitBurst <= 0 {
			cfg.Routes[i].RateLimitBurst = cfg.Routes[i].RateLimitRPS
		}

		if cfg.Routes[i].HealthCheck.Enabled {
			cfg.Routes[i].HealthCheck.Path = strings.TrimSpace(cfg.Routes[i].HealthCheck.Path)
			if cfg.Routes[i].HealthCheck.Path == "" {
				cfg.Routes[i].HealthCheck.Path = "/health"
			}
			if !strings.HasPrefix(cfg.Routes[i].HealthCheck.Path, "/") {
				cfg.Routes[i].HealthCheck.Path = "/" + cfg.Routes[i].HealthCheck.Path
			}
			if cfg.Routes[i].HealthCheck.Interval == "" {
				cfg.Routes[i].HealthCheck.Interval = "5s"
			}
			if cfg.Routes[i].HealthCheck.Timeout == "" {
				cfg.Routes[i].HealthCheck.Timeout = "1s"
			}
		}
	}
}

func normalizePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return ""
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}
	return prefix
}

func normalizeStripPrefix(stripPrefix string) string {
	stripPrefix = strings.TrimSpace(stripPrefix)
	if stripPrefix == "" {
		return ""
	}
	if !strings.HasPrefix(stripPrefix, "/") {
		stripPrefix = "/" + stripPrefix
	}
	if strings.HasSuffix(stripPrefix, "/") && stripPrefix != "/" {
		stripPrefix = strings.TrimRight(stripPrefix, "/")
	}
	return stripPrefix
}

func validate(cfg *Config) error {
	if cfg.Server.Addr == "" {
		return fmt.Errorf("server.addr is required")
	}
	if _, err := cfg.RequestTimeoutDuration(); err != nil {
		return err
	}
	if _, err := cfg.ShutdownTimeoutDuration(); err != nil {
		return err
	}
	if err := validateRateLimit("server", cfg.Server.RateLimitRPS, cfg.Server.RateLimitBurst); err != nil {
		return err
	}
	if err := validateMaxConcurrency("server", cfg.Server.MaxConcurrency); err != nil {
		return err
	}
	if err := validateAdmin(cfg); err != nil {
		return err
	}

	if len(cfg.Routes) == 0 {
		return fmt.Errorf("at least one route is required")
	}
	routeIDs := make(map[string]struct{})
	prefixes := make(map[string]struct{})

	for i, route := range cfg.Routes {
		if route.ID == "" {
			return fmt.Errorf("routes[%d].id is required", i)
		}

		if _, exists := routeIDs[route.ID]; exists {
			return fmt.Errorf("duplicate route id %q", route.ID)
		}
		routeIDs[route.ID] = struct{}{}

		if route.Prefix == "" {
			return fmt.Errorf("routes[%d].prefix is required", i)
		}

		if _, exists := prefixes[route.Prefix]; exists {
			return fmt.Errorf("duplicate route prefix %q", route.Prefix)
		}
		prefixes[route.Prefix] = struct{}{}

		if route.StripPrefix != "" && !strings.HasPrefix(route.Prefix, route.StripPrefix) {
			return fmt.Errorf(
				"routes[%d] stripPrefix %q must be prefix of route prefix %q",
				i,
				route.StripPrefix,
				route.Prefix,
			)
		}

		if route.Target == "" {
			return fmt.Errorf("routes[%d].target is required", i)
		}

		targetURL, err := url.Parse(route.Target)
		if err != nil {
			return fmt.Errorf("routes[%d].target %q is invalid: %w", i, route.Target, err)
		}
		if targetURL.Scheme == "" || targetURL.Host == "" {
			return fmt.Errorf("routes[%d].target %q must contain scheme and host", i, route.Target)
		}
		if err := validateRateLimit(fmt.Sprintf("route[%d]", i), route.RateLimitRPS, route.RateLimitBurst); err != nil {
			return err
		}
		if err := validateMaxConcurrency(fmt.Sprintf("route[%d]", i), route.MaxConcurrency); err != nil {
			return err
		}
		if err := validateHealthCheck(fmt.Sprintf("route[%d]", i), route.HealthCheck); err != nil {
			return err
		}
	}

	return nil
}

func validateRateLimit(scope string, rps int, burst int) error {
	if rps < 0 {
		return fmt.Errorf("%s.rateLimitRPS limit cannot be negative", scope)
	}
	if burst < 0 {
		return fmt.Errorf("%s.rateLimitBurst limit cannot be negative", scope)
	}
	if rps == 0 && burst > 0 {
		return fmt.Errorf("%s.rateLimitBurst requires rateLimitRPS > 0", scope)
	}
	if rps > 100000 {
		return fmt.Errorf("%s.rateLimitRPS is too large: %d", scope, rps)
	}
	if burst > 100000 {
		return fmt.Errorf("%s.rateLimitBurst is too large: %d", scope, burst)
	}
	return nil
}

func validateMaxConcurrency(scope string, maxConcurrency int) error {
	if maxConcurrency < 0 {
		return fmt.Errorf("%s.maxConcurrency cannot be negative", scope)
	}
	if maxConcurrency > 100000 {
		return fmt.Errorf("%s.maxConcurrency is too large: %d", scope, maxConcurrency)
	}
	return nil
}

func validateHealthCheck(scope string, healthCheck HealthCheckConfig) error {
	if !healthCheck.Enabled {
		return nil
	}
	if healthCheck.Path == "" {
		return fmt.Errorf("%s.healthCheck.path is required when health check is enabled", scope)
	}
	if !strings.HasPrefix(healthCheck.Path, "/") {
		return fmt.Errorf("%s.healthCheck.path must start with /", scope)
	}

	interval, err := healthCheck.IntervalDuration()
	if err != nil {
		return fmt.Errorf("%s.%w", scope, err)
	}
	timeout, err := healthCheck.TimeoutDuration()
	if err != nil {
		return fmt.Errorf("%s.%w", scope, err)
	}
	if interval <= 0 {
		return fmt.Errorf("%s.healthCheck.interval cannot be <= 0", scope)
	}
	if timeout <= 0 {
		return fmt.Errorf("%s.healthCheck.timeout cannot be <= 0", scope)
	}
	if timeout >= interval {
		return fmt.Errorf(
			"%s.healthCheck.timeout must be less than interval, timeout=%s interval=%s",
			scope,
			timeout,
			interval,
		)
	}
	return nil
}

func validateAdmin(cfg *Config) error {
	cfg.Admin.Token = strings.TrimSpace(cfg.Admin.Token)

	if cfg.Admin.Enable && cfg.Admin.Token == "" {
		return fmt.Errorf("admin.token is required when admin is enabled")
	}

	if cfg.Admin.MetricsRequireToken && cfg.Admin.Token == "" {
		return fmt.Errorf("admin.token is required when metricsRequireToken is enabled")
	}

	return nil
}

func (h HealthCheckConfig) IntervalDuration() (time.Duration, error) {
	if h.Interval == "" {
		return 5 * time.Second, nil
	}
	d, err := time.ParseDuration(h.Interval)
	if err != nil {
		return 0, fmt.Errorf("invalid healthCheck.interval %q: %w", h.Interval, err)
	}
	return d, nil
}

func (h HealthCheckConfig) TimeoutDuration() (time.Duration, error) {
	if h.Timeout == "" {
		return 1 * time.Second, nil
	}
	d, err := time.ParseDuration(h.Timeout)
	if err != nil {
		return 0, fmt.Errorf("invalid healthCheck.timeout %q: %w", h.Timeout, err)
	}
	return d, nil
}
