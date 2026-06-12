package server

import (
	"go-mini-gateway/internal/concurrency"
	"go-mini-gateway/internal/health"
	"go-mini-gateway/internal/proxy"
	"go-mini-gateway/internal/ratelimit"
)

type routeDTO struct {
	ID                 string                   `json:"id"`
	Prefix             string                   `json:"prefix"`
	StripPrefix        string                   `json:"stripPrefix"`
	Target             string                   `json:"target"`
	Upstreams          []proxy.UpstreamSnapshot `json:"upstreams"`
	RateLimitRPS       int                      `json:"rateLimitRPS"`
	RateLimitBurst     int                      `json:"rateLimitBurst"`
	MaxConcurrency     int                      `json:"maxConcurrency"`
	HealthCheckEnabled bool                     `json:"healthCheckEnabled"`
	HealthCheckPath    string                   `json:"healthCheckPath,omitempty"`
}

type healthDTO struct {
	RouteID   string              `json:"routeId"`
	Target    string              `json:"target"`
	Checked   bool                `json:"checked"`
	Healthy   bool                `json:"healthy"`
	Upstreams []upstreamHealthDTO `json:"upstreams,omitempty"`
}

type upstreamHealthDTO struct {
	ID            string `json:"id"`
	URL           string `json:"url"`
	Name          string `json:"name"`
	Path          string `json:"path,omitempty"`
	Interval      string `json:"interval,omitempty"`
	Timeout       string `json:"timeout,omitempty"`
	Checked       bool   `json:"checked"`
	Healthy       bool   `json:"healthy"`
	LastCheckedAt string `json:"lastCheckedAt,omitempty"`
	LastReason    string `json:"lastReason,omitempty"`
}

type statusDTO struct {
	Global globalStatusDTO  `json:"global"`
	Routes []routeStatusDTO `json:"routes"`
}

type globalStatusDTO struct {
	RateLimit   *ratelimit.SnapShot   `json:"rateLimit,omitempty"`
	Concurrency *concurrency.Snapshot `json:"concurrency,omitempty"`
}

type routeStatusDTO struct {
	ID          string                `json:"id"`
	RateLimit   *ratelimit.SnapShot   `json:"rateLimit,omitempty"`
	Concurrency *concurrency.Snapshot `json:"concurrency,omitempty"`
	Health      *health.Snapshot      `json:"health,omitempty"`
}
