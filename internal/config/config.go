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
	Routes []RouteConfig `json:"routes"`
}

type ServerConfig struct {
	Addr            string `json:"addr"`
	RequestTimeout  string `json:"requestTimeout"`
	ShutdownTimeout string `json:"shutdownTimeout"`
}

type RouteConfig struct {
	ID          string `json:"id"`
	Prefix      string `json:"prefix"`
	StripPrefix string `json:"StripPrefix"`
	Target      string `json:"target"`
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
	for i := range cfg.Routes {
		cfg.Routes[i].ID = strings.TrimSpace(cfg.Routes[i].ID)
		cfg.Routes[i].Prefix = normalizePrefix(cfg.Routes[i].Prefix)
		cfg.Routes[i].StripPrefix = normalizeStripPrefix(cfg.Routes[i].StripPrefix)
		cfg.Routes[i].Target = strings.TrimRight(strings.TrimSpace(cfg.Routes[i].Target), "/")
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
	}

	return nil
}
