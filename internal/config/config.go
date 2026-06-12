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
	defaultAdminAddr       = "127.0.0.1:9001"
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
	Enabled             bool   `json:"enabled"`
	Addr                string `json:"addr"`
	Token               string `json:"token"`
	MetricsRequireToken bool   `json:"metricsRequireToken"`
	PprofEnabled        bool   `json:"pprofEnabled"`
}

type RouteConfig struct {
	ID             string              `json:"id"`
	Prefix         string              `json:"prefix"`
	StripPrefix    string              `json:"stripPrefix"`
	Target         string              `json:"target"`
	Upstreams      []UpstreamConfig    `json:"upstreams"`
	RateLimitRPS   int                 `json:"rateLimitRPS"`
	RateLimitBurst int                 `json:"rateLimitBurst"`
	MaxConcurrency int                 `json:"maxConcurrency"`
	HealthCheck    HealthCheckConfig   `json:"healthCheck"`
	PassiveHealth  PassiveHealthConfig `json:"passiveHealth"`
}

type UpstreamConfig struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type HealthCheckConfig struct {
	Enabled  bool   `json:"enabled"`
	Path     string `json:"path"`
	Interval string `json:"interval"`
	Timeout  string `json:"timeout"`
}

type PassiveHealthConfig struct {
	Enabled           bool   `json:"enabled"`
	FailureThreshold  int    `json:"failureThreshold"`
	SuccessThreshold  int    `json:"successThreshold"`
	UnhealthyDuration string `json:"unhealthyDuration"`
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

	if err := expandConfigEnv(&cfg); err != nil {
		return nil, fmt.Errorf("expand config env failed: %w", err)
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
	cfg.Admin.Addr = strings.TrimSpace(cfg.Admin.Addr)
	cfg.Admin.Token = strings.TrimSpace(cfg.Admin.Token)

	if cfg.Admin.Enabled && cfg.Admin.Addr == "" {
		cfg.Admin.Addr = defaultAdminAddr
	}

	for i := range cfg.Routes {
		cfg.Routes[i].ID = strings.TrimSpace(cfg.Routes[i].ID)
		cfg.Routes[i].Prefix = normalizePrefix(cfg.Routes[i].Prefix)
		cfg.Routes[i].StripPrefix = normalizeStripPrefix(cfg.Routes[i].StripPrefix)
		cfg.Routes[i].Target = normalizeTargetURL(cfg.Routes[i].Target)

		for j := range cfg.Routes[i].Upstreams {
			cfg.Routes[i].Upstreams[j].ID = strings.TrimSpace(cfg.Routes[i].Upstreams[j].ID)
			if cfg.Routes[i].Upstreams[j].ID == "" {
				cfg.Routes[i].Upstreams[j].ID = fmt.Sprintf("upstream-%d", j+1)
			}
			cfg.Routes[i].Upstreams[j].URL = normalizeTargetURL(cfg.Routes[i].Upstreams[j].URL)
		}

		if len(cfg.Routes[i].Upstreams) == 0 && cfg.Routes[i].Target != "" {
			cfg.Routes[i].Upstreams = []UpstreamConfig{
				{
					ID:  "default",
					URL: cfg.Routes[i].Target,
				},
			}
		}

		if cfg.Routes[i].Target == "" && len(cfg.Routes[i].Upstreams) > 0 {
			cfg.Routes[i].Target = cfg.Routes[i].Upstreams[0].URL
		}

		if cfg.Routes[i].RateLimitRPS > 0 && cfg.Routes[i].RateLimitBurst <= 0 {
			cfg.Routes[i].RateLimitBurst = cfg.Routes[i].RateLimitRPS
		}

		if cfg.Routes[i].PassiveHealth.Enabled {
			if cfg.Routes[i].PassiveHealth.FailureThreshold <= 0 {
				cfg.Routes[i].PassiveHealth.FailureThreshold = 3
			}
			if cfg.Routes[i].PassiveHealth.SuccessThreshold <= 0 {
				cfg.Routes[i].PassiveHealth.SuccessThreshold = 1
			}
			if cfg.Routes[i].PassiveHealth.UnhealthyDuration == "" {
				cfg.Routes[i].PassiveHealth.UnhealthyDuration = "10s"
			}
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

func normalizeTargetURL(target string) string {
	return strings.TrimRight(strings.TrimSpace(target), "/")
}

func normalizePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "/"
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
		return "/"
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

		if len(route.Upstreams) == 0 {
			return fmt.Errorf("routes[%d] requires target or upstreams", i)
		}

		if route.Target == "" {
			return fmt.Errorf("routes[%d].target is required", i)
		}

		if err := validateTargetURL(fmt.Sprintf("routes[%d].target", i), route.Target); err != nil {
			return err
		}

		if err := validateUpstreams(fmt.Sprintf("routes[%d]", i), route.Upstreams); err != nil {
			return err
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
		if err := validatePassiveHealth(fmt.Sprintf("route[%d]", i), route.PassiveHealth); err != nil {
			return err
		}
	}

	return nil
}

func validateUpstreams(scope string, upstreams []UpstreamConfig) error {
	if len(upstreams) == 0 {
		return fmt.Errorf("%s.upstreams must not be empty", scope)
	}

	ids := make(map[string]struct{}, len(upstreams))
	for i, upstream := range upstreams {
		if upstream.ID == "" {
			return fmt.Errorf("%s.upstreams[%d].id is required", scope, i)
		}
		if _, exists := ids[upstream.ID]; exists {
			return fmt.Errorf("%s duplicate upstream id %q", scope, upstream.ID)
		}
		ids[upstream.ID] = struct{}{}

		if upstream.URL == "" {
			return fmt.Errorf("%s.upstreams[%d].url is required", scope, i)
		}
		if err := validateTargetURL(fmt.Sprintf("%s.upstreams[%d].url", scope, i), upstream.URL); err != nil {
			return err
		}
	}

	return nil
}

func validateTargetURL(scope string, rawURL string) error {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%s %q is invalid: %w", scope, rawURL, err)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return fmt.Errorf("%s %q must contain scheme and host", scope, rawURL)
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

func validatePassiveHealth(scope string, passiveHealth PassiveHealthConfig) error {
	if !passiveHealth.Enabled {
		return nil
	}

	if passiveHealth.FailureThreshold <= 0 {
		return fmt.Errorf("%s.passiveHealth.failureThreshold must be > 0", scope)
	}
	if passiveHealth.SuccessThreshold <= 0 {
		return fmt.Errorf("%s.passiveHealth.successThreshold must be > 0", scope)
	}
	if passiveHealth.FailureThreshold > 100000 {
		return fmt.Errorf("%s.passiveHealth.failureThreshold is too large: %d", scope, passiveHealth.FailureThreshold)
	}
	if passiveHealth.SuccessThreshold > 100000 {
		return fmt.Errorf("%s.passiveHealth.successThreshold is too large: %d", scope, passiveHealth.SuccessThreshold)
	}

	duration, err := passiveHealth.UnhealthyDurationDuration()
	if err != nil {
		return fmt.Errorf("%s.%w", scope, err)
	}
	if duration <= 0 {
		return fmt.Errorf("%s.passiveHealth.unhealthyDuration must be > 0", scope)
	}

	return nil
}

func validateAdmin(cfg *Config) error {
	if !cfg.Admin.Enabled {
		if cfg.Admin.MetricsRequireToken {
			return fmt.Errorf("admin.metricsRequireToken requires admin.enabled to be true")
		}

		return nil
	}

	if cfg.Admin.Addr == "" {
		return fmt.Errorf("admin.addr is required when admin.enabled is true")
	}

	if cfg.Admin.Token == "" {
		return fmt.Errorf("admin.token is required when admin.enabled is true")
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

func (p PassiveHealthConfig) UnhealthyDurationDuration() (time.Duration, error) {
	if p.UnhealthyDuration == "" {
		return 10 * time.Second, nil
	}
	d, err := time.ParseDuration(p.UnhealthyDuration)
	if err != nil {
		return 0, fmt.Errorf("invalid passiveHealth.unhealthyDuration %q: %w", p.UnhealthyDuration, err)
	}
	return d, nil
}
