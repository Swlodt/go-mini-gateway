package proxy

import (
	"context"
	"fmt"
	"go-mini-gateway/internal/health"
	"net/url"
	"strings"
	"time"
)

func buildUpstreams(options Options) ([]*upstream, error) {
	upstreamOptions := options.Upstreams
	if len(upstreamOptions) == 0 && options.Target != "" {
		upstreamOptions = []UpstreamOptions{{ID: "default", URL: options.Target}}
	}
	if len(upstreamOptions) == 0 {
		return nil, fmt.Errorf("proxy route %q requires target or upstreams", options.RouteID)
	}

	upstreams := make([]*upstream, 0, len(upstreamOptions))
	for i, option := range upstreamOptions {
		item, err := buildUpstream(options, option, i)
		if err != nil {
			return nil, err
		}
		upstreams = append(upstreams, item)
	}

	return upstreams, nil
}

func buildUpstream(options Options, option UpstreamOptions, index int) (*upstream, error) {
	id := strings.TrimSpace(option.ID)
	if id == "" {
		id = fmt.Sprintf("upstream-%d", index+1)
	}

	targetURL, err := url.Parse(option.URL)
	if err != nil {
		return nil, fmt.Errorf("parse proxy upstream %q url %q failed: %w", id, option.URL, err)
	}
	if targetURL.Scheme == "" || targetURL.Host == "" {
		return nil, fmt.Errorf("invalid proxy upstream %q url %q: scheme and host are required", id, option.URL)
	}

	activeHealth, err := newActiveHealthChecker(options.RouteID, id, option.URL, options.ActiveHealth)
	if err != nil {
		return nil, err
	}

	return &upstream{
		id:             id,
		url:            targetURL,
		activeHealth:   activeHealth,
		passiveHealth:  newPassiveHealthState(options.RouteID, id, options.PassiveHealth),
		circuitBreaker: newCircuitBreakerState(options.RouteID, id, options.CircuitBreaker),
	}, nil
}

func newActiveHealthChecker(routeID string, upstreamID string, target string, options ActiveHealthOptions) (*health.Checker, error) {
	if !options.Enabled {
		return nil, nil
	}

	if options.Path == "" {
		options.Path = "/health"
	}
	if options.Interval <= 0 {
		options.Interval = 5 * time.Second
	}
	if options.Timeout <= 0 {
		options.Timeout = time.Second
	}

	checker, err := health.NewChecker(health.Options{
		Name:     fmt.Sprintf("%s/%s", routeID, upstreamID),
		Target:   target,
		Path:     options.Path,
		Interval: options.Interval,
		Timeout:  options.Timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("create active health checker for route %q upstream %q failed: %w", routeID, upstreamID, err)
	}

	checker.Start()
	return checker, nil
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

func (u *upstream) recordSuccess() {
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

func (u *upstream) recordFailure(reason string) {
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
	if h == nil {
		return
	}
	for _, item := range h.upstreams {
		if item.activeHealth != nil {
			item.activeHealth.Close()
		}
	}
	if h.transport != nil {
		h.transport.CloseIdleConnections()
	}
}

func (h *Handler) UpstreamSnapshots() []UpstreamSnapshot {
	if h == nil {
		return nil
	}

	now := time.Now()
	snapshots := make([]UpstreamSnapshot, 0, len(h.upstreams))
	for _, item := range h.upstreams {
		snapshot := UpstreamSnapshot{
			ID:  item.id,
			URL: item.url.String(),
		}
		if item.activeHealth != nil {
			activeSnapshot := item.activeHealth.Snapshot()
			snapshot.ActiveHealth = &activeSnapshot
		}
		if item.passiveHealth != nil {
			passiveSnapshot := item.passiveHealth.snapshot(now)
			snapshot.PassiveHealth = &passiveSnapshot
		}
		if item.circuitBreaker != nil {
			circuitBreakerSnapshot := item.circuitBreaker.snapshot(now)
			snapshot.CircuitBreaker = &circuitBreakerSnapshot
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}
