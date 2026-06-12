package proxy

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	gatewayName = "go-mini-gateway"
)

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

type selectedUpstreamContextKey struct{}

func New(options Options) (*Handler, error) {
	upstreams, err := buildUpstreams(options)
	if err != nil {
		return nil, err
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()

	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 20
	transport.MaxConnsPerHost = 100
	transport.IdleConnTimeout = 90 * time.Second
	transport.ResponseHeaderTimeout = 10 * time.Second
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ExpectContinueTimeout = 1 * time.Second

	h := &Handler{
		routeID:     options.RouteID,
		stripPrefix: options.StripPrefix,
		upstreams:   upstreams,
		transport:   transport,
	}

	rp := &httputil.ReverseProxy{
		Transport: h.transport,

		Rewrite: func(pr *httputil.ProxyRequest) {
			h.rewriteRequest(pr)
		},

		ModifyResponse: func(resp *http.Response) error {
			selected := selectedUpstreamFromContext(resp.Request.Context())
			if selected != nil {
				if resp.StatusCode >= http.StatusInternalServerError {
					selected.recordPassiveFailure(fmt.Sprintf("backend status=%d", resp.StatusCode))
				} else {
					selected.recordPassiveSuccess()
				}
			}

			resp.Header.Set("X-Gateway", gatewayName)
			resp.Header.Set("X-Gateway-Route", h.routeID)
			if selected != nil {
				resp.Header.Set("X-Gateway-Upstream", selected.id)
			} else if resp.Request != nil {
				if upstreamID := resp.Request.Header.Get("X-Gateway-Upstream"); upstreamID != "" {
					resp.Header.Set("X-Gateway-Upstream", upstreamID)
				}
			}
			return nil
		},

		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			selected := selectedUpstreamFromContext(r.Context())
			if selected != nil {
				selected.recordPassiveFailure(err.Error())
			}

			statusCode := statusCodeFromProxyError(err)
			log.Printf(
				"proxy backend request failed: route=%s upstream=%s method=%s path=%s status=%d err=%v",
				h.routeID,
				upstreamIDOrEmpty(selected),
				r.Method,
				r.URL.Path,
				statusCode,
				err,
			)
			http.Error(w, http.StatusText(statusCode), statusCode)
		},
	}

	h.reverseProxy = rp

	return h, nil
}

func buildUpstreams(options Options) ([]*upstream, error) {
	upstreamOptions := options.Upstreams
	if len(upstreamOptions) == 0 && options.Target != "" {
		upstreamOptions = []UpstreamOptions{
			{
				ID:  "default",
				URL: options.Target,
			},
		}
	}

	if len(upstreamOptions) == 0 {
		return nil, fmt.Errorf("proxy route %q requires target or upstreams", options.RouteID)
	}

	upstreams := make([]*upstream, 0, len(upstreamOptions))
	for i, option := range upstreamOptions {
		id := strings.TrimSpace(option.ID)
		if id == "" {
			id = fmt.Sprintf("upstream-%d", i+1)
		}

		targetURL, err := url.Parse(option.URL)
		if err != nil {
			return nil, fmt.Errorf("parse proxy upstream %q url %q failed: %w", id, option.URL, err)
		}
		if targetURL.Scheme == "" || targetURL.Host == "" {
			return nil, fmt.Errorf("invalid proxy upstream %q url %q: schema and host are required", id, option.URL)
		}

		upstreams = append(upstreams, &upstream{
			id:             id,
			url:            targetURL,
			activeHealth:   activeHealth,
			passiveHealth:  newPassiveHealthState(options.RouteID, id, options.PassiveHealth),
			circuitBreaker: newCircuitBreakerState(options.RouteID, id, options.CircuitBreaker),
		})
	}

	return upstreams, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	selected := h.selectUpstream()
	if selected == nil {
		log.Printf("no available upstream: route=%s method=%s path=%s", h.routeID, r.Method, r.URL.Path)
		w.Header().Set("X-Gateway", gatewayName)
		w.Header().Set("X-Gateway-Route", h.routeID)
		http.Error(w, "no available upstream", http.StatusServiceUnavailable)
		return
	}

	ctx := context.WithValue(r.Context(), selectedUpstreamContextKey{}, selected)
	h.reverseProxy.ServeHTTP(w, r.WithContext(ctx))
}

func (h *Handler) rewriteRequest(pr *httputil.ProxyRequest) {
	selected := selectedUpstreamFromContext(pr.In.Context())
	if selected == nil {
		selected = h.selectUpstream()
	}
	if selected == nil {
		return
	}

	pr.SetURL(selected.url)

	path := pr.In.URL.Path

	if h.stripPrefix != "" {
		path = strings.TrimPrefix(path, h.stripPrefix)
	}

	if path == "" {
		path = "/"
	}

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	pr.Out.URL.Path = path
	pr.Out.URL.RawPath = ""

	pr.SetXForwarded()
	pr.Out.Header.Set("X-Gateway", gatewayName)
	pr.Out.Header.Set("X-Gateway-Route", h.routeID)
	pr.Out.Header.Set("X-Gateway-Upstream", selected.id)

	traceID := pr.In.Header.Get("X-Trace-ID")
	if traceID == "" {
		traceID = fmt.Sprintf("trace-%d", time.Now().UnixNano())
	}
	pr.Out.Header.Set("X-Trace-ID", traceID)
}

func (h *Handler) selectUpstream() *upstream {
	count := len(h.upstreams)
	if count == 0 {
		return nil
	}

	start := h.nextIndex.Add(1) - 1
	for i := 0; i < count; i++ {
		index := int((start + uint64(i)) % uint64(count))
		candidate := h.upstreams[index]
		if candidate.tryAcquire() {
			return candidate
		}
	}

	return nil
}

func selectedUpstreamFromContext(ctx context.Context) *upstream {
	if ctx == nil {
		return nil
	}

	selected, _ := ctx.Value(selectedUpstreamContextKey{}).(*upstream)
	return selected
}

func upstreamIDOrEmpty(selected *upstream) string {
	if selected == nil {
		return ""
	}
	return selected.id
}

func (u *upstream) tryAcquire() bool {
	if u == nil {
		return false
	}

	now := time.Now()
	if u.activeHealth != nil && !u.activeHealth.Available() {
		return false
	}
	if u.passiveHealth != nil && !u.passiveHealth.available(now) {
		return false
	}
	if u.circuitBreaker != nil && !u.circuitBreaker.allow(now) {
		return false
	}

	return true
}

func (u *upstream) recordPassiveSuccess() {
	if u == nil {
		return
	}
	now := time.Now()
	if u.passiveHealth != nil {
		u.passiveHealth.recordSuccess(now)
	}
	if u.circuitBreaker != nil {
		u.circuitBreaker.recordSuccess(now)
	}
}

func (u *upstream) recordPassiveFailure(reason string) {
	if u == nil {
		return
	}
	now := time.Now()
	if u.passiveHealth != nil {
		u.passiveHealth.recordFailure(now, reason)
	}
	if u.circuitBreaker != nil {
		u.circuitBreaker.recordFailure(now, reason)
	}
}

func (h *Handler) CloseIdleConnections() {
	if h == nil || h.transport == nil {
		return
	}
	h.transport.CloseIdleConnections()
}

func (h *Handler) UpstreamSnapshots() []UpstreamSnapshot {
	if h == nil {
		return nil
	}

	snapshots := make([]UpstreamSnapshot, 0, len(h.upstreams))
	for _, item := range h.upstreams {
		snapshot := UpstreamSnapshot{
			ID:  item.id,
			URL: item.url.String(),
		}
		if item.passiveHealth != nil {
			passiveSnapshot := item.passiveHealth.snapshot(time.Now())
			snapshot.PassiveHealth = &passiveSnapshot
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

type passiveHealthState struct {
	routeID           string
	upstreamID        string
	failureThreshold  int
	successThreshold  int
	unhealthyDuration time.Duration

	mu                   sync.RWMutex
	unhealthy            bool
	consecutiveFailures  int
	consecutiveSuccesses int
	retryAfter           time.Time
	lastFailureAt        time.Time
	lastSuccessAt        time.Time
	lastReason           string
}

func newPassiveHealthState(routeID string, upstreamID string, options PassiveHealthOptions) *passiveHealthState {
	if !options.Enabled {
		return nil
	}

	if options.FailureThreshold <= 0 {
		options.FailureThreshold = 3
	}
	if options.SuccessThreshold <= 0 {
		options.SuccessThreshold = 1
	}
	if options.UnhealthyDuration <= 0 {
		options.UnhealthyDuration = 10 * time.Second
	}

	return &passiveHealthState{
		routeID:           routeID,
		upstreamID:        upstreamID,
		failureThreshold:  options.FailureThreshold,
		successThreshold:  options.SuccessThreshold,
		unhealthyDuration: options.UnhealthyDuration,
	}
}

func (s *passiveHealthState) available(now time.Time) bool {
	if s == nil {
		return true
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.unhealthy {
		return true
	}

	return !now.Before(s.retryAfter)
}

func (s *passiveHealthState) recordSuccess(now time.Time) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastSuccessAt = now
	s.consecutiveFailures = 0

	if !s.unhealthy {
		s.consecutiveSuccesses = 0
		return
	}

	s.consecutiveSuccesses++
	if s.consecutiveSuccesses >= s.successThreshold {
		s.unhealthy = false
		s.retryAfter = time.Time{}
		s.lastReason = "recovered by passive health success"
		log.Printf(
			"passive health upstream recovered: route=%s upstream=%s successes=%d",
			s.routeID,
			s.upstreamID,
			s.consecutiveSuccesses,
		)
		s.consecutiveSuccesses = 0
	}
}

func (s *passiveHealthState) recordFailure(now time.Time, reason string) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastFailureAt = now
	s.lastReason = reason
	s.consecutiveFailures++
	s.consecutiveSuccesses = 0

	if s.consecutiveFailures >= s.failureThreshold {
		wasHealthy := !s.unhealthy
		s.unhealthy = true
		s.retryAfter = now.Add(s.unhealthyDuration)
		if wasHealthy {
			log.Printf(
				"passive health upstream marked unhealthy: route=%s upstream=%s failures=%d retryAfter=%s reason=%s",
				s.routeID,
				s.upstreamID,
				s.consecutiveFailures,
				s.retryAfter.Format(time.RFC3339Nano),
				reason,
			)
		}
	}
}

func (s *passiveHealthState) snapshot(now time.Time) PassiveHealthSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var retryAfter string
	if !s.retryAfter.IsZero() {
		retryAfter = s.retryAfter.Format(time.RFC3339Nano)
	}

	var lastFailureAt string
	if !s.lastFailureAt.IsZero() {
		lastFailureAt = s.lastFailureAt.Format(time.RFC3339Nano)
	}

	var lastSuccessAt string
	if !s.lastSuccessAt.IsZero() {
		lastSuccessAt = s.lastSuccessAt.Format(time.RFC3339Nano)
	}

	return PassiveHealthSnapshot{
		Enabled:              true,
		Healthy:              !s.unhealthy,
		Available:            !s.unhealthy || !now.Before(s.retryAfter),
		ConsecutiveFailures:  s.consecutiveFailures,
		ConsecutiveSuccesses: s.consecutiveSuccesses,
		FailureThreshold:     s.failureThreshold,
		SuccessThreshold:     s.successThreshold,
		UnhealthyDuration:    s.unhealthyDuration.String(),
		RetryAfter:           retryAfter,
		LastFailureAt:        lastFailureAt,
		LastSuccessAt:        lastSuccessAt,
		LastReason:           s.lastReason,
	}
}

func statusCodeFromProxyError(err error) int {
	if isTimeoutError(err) {
		return http.StatusGatewayTimeout
	}
	return http.StatusBadGateway
}

func isTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}
