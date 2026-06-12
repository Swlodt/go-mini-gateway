package config

import (
	"fmt"
	"os"
	"strings"
)

func expandConfigEnv(cfg *Config) error {
	var err error

	cfg.Server.Addr, err = expandEnvString(cfg.Server.Addr)
	if err != nil {
		return fmt.Errorf("expand server.addr failed: %w", err)
	}

	cfg.Server.RequestTimeout, err = expandEnvString(cfg.Server.RequestTimeout)
	if err != nil {
		return fmt.Errorf("expand server.requestTimeout failed: %w", err)
	}

	cfg.Server.ShutdownTimeout, err = expandEnvString(cfg.Server.ShutdownTimeout)
	if err != nil {
		return fmt.Errorf("expand server.shutdownTimeout failed: %w", err)
	}

	cfg.Admin.Addr, err = expandEnvString(cfg.Admin.Addr)
	if err != nil {
		return fmt.Errorf("expand admin.addr failed: %w", err)
	}

	cfg.Admin.Token, err = expandEnvString(cfg.Admin.Token)
	if err != nil {
		return fmt.Errorf("expand admin.token failed: %w", err)
	}

	for i := range cfg.Routes {
		cfg.Routes[i].ID, err = expandEnvString(cfg.Routes[i].ID)
		if err != nil {
			return fmt.Errorf("expand routes[%d].id failed: %w", i, err)
		}

		cfg.Routes[i].Prefix, err = expandEnvString(cfg.Routes[i].Prefix)
		if err != nil {
			return fmt.Errorf("expand routes[%d].prefix failed: %w", i, err)
		}

		cfg.Routes[i].StripPrefix, err = expandEnvString(cfg.Routes[i].StripPrefix)
		if err != nil {
			return fmt.Errorf("expand routes[%d].stripPrefix failed: %w", i, err)
		}

		cfg.Routes[i].Target, err = expandEnvString(cfg.Routes[i].Target)
		if err != nil {
			return fmt.Errorf("expand routes[%d].target failed: %w", i, err)
		}

		for j := range cfg.Routes[i].Upstreams {
			cfg.Routes[i].Upstreams[j].ID, err = expandEnvString(cfg.Routes[i].Upstreams[j].ID)
			if err != nil {
				return fmt.Errorf("expand routes[%d].upstreams[%d].id failed: %w", i, j, err)
			}

			cfg.Routes[i].Upstreams[j].URL, err = expandEnvString(cfg.Routes[i].Upstreams[j].URL)
			if err != nil {
				return fmt.Errorf("expand routes[%d].upstreams[%d].url failed: %w", i, j, err)
			}
		}

		cfg.Routes[i].HealthCheck.Path, err = expandEnvString(cfg.Routes[i].HealthCheck.Path)
		if err != nil {
			return fmt.Errorf("expand routes[%d].healthCheck.path failed: %w", i, err)
		}

		cfg.Routes[i].HealthCheck.Interval, err = expandEnvString(cfg.Routes[i].HealthCheck.Interval)
		if err != nil {
			return fmt.Errorf("expand routes[%d].healthCheck.interval failed: %w", i, err)
		}

		cfg.Routes[i].HealthCheck.Timeout, err = expandEnvString(cfg.Routes[i].HealthCheck.Timeout)
		if err != nil {
			return fmt.Errorf("expand routes[%d].healthCheck.timeout failed: %w", i, err)
		}
	}

	return nil
}

func expandEnvString(value string) (string, error) {
	var builder strings.Builder

	for i := 0; i < len(value); {
		if value[i] != '$' || i+1 >= len(value) || value[i+1] != '{' {
			builder.WriteByte(value[i])
			i++
			continue
		}

		end := strings.IndexByte(value[i+2:], '}')
		if end < 0 {
			return "", fmt.Errorf("invalid env expression %q: missing closing }", value[i:])
		}

		exprStart := i + 2
		exprEnd := i + 2 + end
		expr := value[exprStart:exprEnd]

		expanded, err := expandEnvExpression(expr)
		if err != nil {
			return "", err
		}

		builder.WriteString(expanded)

		i = exprEnd + 1
	}

	return builder.String(), nil
}

func expandEnvExpression(expr string) (string, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "", fmt.Errorf("empty env expression")
	}

	name, defaultValue, hasDefault := splitEnvExpression(expr)

	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("empty env variable name in expression %q", expr)
	}

	value, exists := os.LookupEnv(name)
	if exists && value != "" {
		return value, nil
	}

	if hasDefault {
		return defaultValue, nil
	}

	return "", fmt.Errorf("environment variable %s is required", name)
}

func splitEnvExpression(expr string) (name string, defaultValue string, hasDefault bool) {
	index := strings.IndexByte(expr, ':')
	if index < 0 {
		return expr, "", false
	}

	return expr[:index], expr[index+1:], true
}
