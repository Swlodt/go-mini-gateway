package proxy

import (
	"go-mini-gateway/internal/health"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync/atomic"
	"time"
)

const gatewayName = "go-mini-gateway"

type Options struct {
	RouteID        string
	Target         string
	Upstreams      []UpstreamOptions
	StripPrefix    string
	ActiveHealth   ActiveHealthOptions
	PassiveHealth  PassiveHealthOptions
	CircuitBreaker CircuitBreakerOptions
}

type UpstreamOptions struct {
	ID  string
	URL string
}

type ActiveHealthOptions struct {
	Enabled  bool
	Path     string
	Interval time.Duration
	Timeout  time.Duration
}

type PassiveHealthOptions struct {
	Enabled           bool
	FailureThreshold  int
	SuccessThreshold  int
	UnhealthyDuration time.Duration
}

type CircuitBreakerOptions struct {
	Enabled             bool
	FailureThreshold    int
	OpenDuration        time.Duration
	HalfOpenMaxRequests int
}

type PassiveHealthSnapshot struct {
	Enabled              bool   `json:"enabled"`
	Healthy              bool   `json:"healthy"`
	Available            bool   `json:"available"`
	ConsecutiveFailures  int    `json:"consecutiveFailures"`
	ConsecutiveSuccesses int    `json:"consecutiveSuccesses"`
	FailureThreshold     int    `json:"failureThreshold"`
	SuccessThreshold     int    `json:"successThreshold"`
	UnhealthyDuration    string `json:"unhealthyDuration"`
	RetryAfter           string `json:"retryAfter,omitempty"`
	LastFailureAt        string `json:"lastFailureAt,omitempty"`
	LastSuccessAt        string `json:"lastSuccessAt,omitempty"`
	LastReason           string `json:"lastReason,omitempty"`
}

type CircuitBreakerSnapshot struct {
	Enabled             bool   `json:"enabled"`
	State               string `json:"state"`
	Available           bool   `json:"available"`
	ConsecutiveFailures int    `json:"consecutiveFailures"`
	FailureThreshold    int    `json:"failureThreshold"`
	OpenDuration        string `json:"openDuration"`
	HalfOpenMaxRequests int    `json:"halfOpenMaxRequests"`
	HalfOpenInFlight    int    `json:"halfOpenInFlight"`
	HalfOpenSuccesses   int    `json:"halfOpenSuccesses"`
	NextAttemptAt       string `json:"nextAttemptAt,omitempty"`
	LastFailureAt       string `json:"lastFailureAt,omitempty"`
	LastSuccessAt       string `json:"lastSuccessAt,omitempty"`
	LastReason          string `json:"lastReason,omitempty"`
}

type UpstreamSnapshot struct {
	ID             string                  `json:"id"`
	URL            string                  `json:"url"`
	ActiveHealth   *health.Snapshot        `json:"activeHealth,omitempty"`
	PassiveHealth  *PassiveHealthSnapshot  `json:"passiveHealth,omitempty"`
	CircuitBreaker *CircuitBreakerSnapshot `json:"circuitBreaker,omitempty"`
}

type upstream struct {
	id             string
	url            *url.URL
	activeHealth   *health.Checker
	passiveHealth  *passiveHealthState
	circuitBreaker *circuitBreakerState
}

type Handler struct {
	routeID      string
	stripPrefix  string
	upstreams    []*upstream
	nextIndex    atomic.Uint64
	transport    *http.Transport
	reverseProxy *httputil.ReverseProxy
}
