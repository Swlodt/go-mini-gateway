package proxy

import (
	"log"
	"sync"
	"time"
)

const (
	circuitBreakerStateClosed   = "closed"
	circuitBreakerStateOpen     = "open"
	circuitBreakerStateHalfOpen = "half_open"
)

type circuitBreakerState struct {
	routeID             string
	upstreamID          string
	failureThreshold    int
	openDuration        time.Duration
	halfOpenMaxRequests int

	mu                  sync.Mutex
	state               string
	consecutiveFailures int
	halfOpenInFlight    int
	halfOpenSuccesses   int
	nextAttemptAt       time.Time
	lastFailureAt       time.Time
	lastSuccessAt       time.Time
	lastReason          string
}

func newCircuitBreakerState(routeID string, upstreamID string, options CircuitBreakerOptions) *circuitBreakerState {
	if !options.Enabled {
		return nil
	}

	if options.FailureThreshold <= 0 {
		options.FailureThreshold = 5
	}
	if options.OpenDuration <= 0 {
		options.OpenDuration = 10 * time.Second
	}
	if options.HalfOpenMaxRequests <= 0 {
		options.HalfOpenMaxRequests = 1
	}

	return &circuitBreakerState{
		routeID:             routeID,
		upstreamID:          upstreamID,
		failureThreshold:    options.FailureThreshold,
		openDuration:        options.OpenDuration,
		halfOpenMaxRequests: options.HalfOpenMaxRequests,
		state:               circuitBreakerStateClosed,
	}
}

func (c *circuitBreakerState) allow(now time.Time) bool {
	if c == nil {
		return true
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.state {
	case circuitBreakerStateClosed:
		return true

	case circuitBreakerStateOpen:
		if now.Before(c.nextAttemptAt) {
			return false
		}

		c.state = circuitBreakerStateHalfOpen
		c.halfOpenInFlight = 0
		c.halfOpenSuccesses = 0
		c.lastReason = "enter half-open after open duration"
		log.Printf(
			"circuit breaker half-open: route=%s upstream=%s",
			c.routeID,
			c.upstreamID,
		)
		fallthrough

	case circuitBreakerStateHalfOpen:
		if c.halfOpenInFlight >= c.halfOpenMaxRequests {
			return false
		}
		c.halfOpenInFlight++
		return true

	default:
		c.state = circuitBreakerStateClosed
		return true
	}
}

func (c *circuitBreakerState) recordSuccess(now time.Time) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.lastSuccessAt = now

	switch c.state {
	case circuitBreakerStateClosed:
		c.consecutiveFailures = 0

	case circuitBreakerStateHalfOpen:
		if c.halfOpenInFlight > 0 {
			c.halfOpenInFlight--
		}
		c.halfOpenSuccesses++
		if c.halfOpenSuccesses >= c.halfOpenMaxRequests {
			c.closeLocked("recovered by half-open successes")
		}
	}
}

func (c *circuitBreakerState) recordFailure(now time.Time, reason string) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.lastFailureAt = now
	c.lastReason = reason

	switch c.state {
	case circuitBreakerStateClosed:
		c.consecutiveFailures++
		if c.consecutiveFailures >= c.failureThreshold {
			c.openLocked(now, reason)
		}

	case circuitBreakerStateHalfOpen:
		if c.halfOpenInFlight > 0 {
			c.halfOpenInFlight--
		}
		c.openLocked(now, reason)

	case circuitBreakerStateOpen:
		c.nextAttemptAt = now.Add(c.openDuration)
	}
}

func (c *circuitBreakerState) openLocked(now time.Time, reason string) {
	wasOpen := c.state == circuitBreakerStateOpen

	c.state = circuitBreakerStateOpen
	c.nextAttemptAt = now.Add(c.openDuration)
	c.halfOpenInFlight = 0
	c.halfOpenSuccesses = 0
	c.lastReason = reason

	if !wasOpen {
		log.Printf(
			"circuit breaker opened: route=%s upstream=%s failures=%d nextAttemptAt=%s reason=%s",
			c.routeID,
			c.upstreamID,
			c.consecutiveFailures,
			c.nextAttemptAt.Format(time.RFC3339Nano),
			reason,
		)
	}
}

func (c *circuitBreakerState) closeLocked(reason string) {
	c.state = circuitBreakerStateClosed
	c.consecutiveFailures = 0
	c.halfOpenInFlight = 0
	c.halfOpenSuccesses = 0
	c.nextAttemptAt = time.Time{}
	c.lastReason = reason

	log.Printf(
		"circuit breaker closed: route=%s upstream=%s reason=%s",
		c.routeID,
		c.upstreamID,
		reason,
	)
}

func (c *circuitBreakerState) snapshot(now time.Time) CircuitBreakerSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	state := c.state
	available := true
	if state == circuitBreakerStateOpen {
		available = !now.Before(c.nextAttemptAt)
	} else if state == circuitBreakerStateHalfOpen {
		available = c.halfOpenInFlight < c.halfOpenMaxRequests
	}

	var nextAttemptAt string
	if !c.nextAttemptAt.IsZero() {
		nextAttemptAt = c.nextAttemptAt.Format(time.RFC3339Nano)
	}

	var lastFailureAt string
	if !c.lastFailureAt.IsZero() {
		lastFailureAt = c.lastFailureAt.Format(time.RFC3339Nano)
	}

	var lastSuccessAt string
	if !c.lastSuccessAt.IsZero() {
		lastSuccessAt = c.lastSuccessAt.Format(time.RFC3339Nano)
	}

	return CircuitBreakerSnapshot{
		Enabled:             true,
		State:               state,
		Available:           available,
		ConsecutiveFailures: c.consecutiveFailures,
		FailureThreshold:    c.failureThreshold,
		OpenDuration:        c.openDuration.String(),
		HalfOpenMaxRequests: c.halfOpenMaxRequests,
		HalfOpenInFlight:    c.halfOpenInFlight,
		HalfOpenSuccesses:   c.halfOpenSuccesses,
		NextAttemptAt:       nextAttemptAt,
		LastFailureAt:       lastFailureAt,
		LastSuccessAt:       lastSuccessAt,
		LastReason:          c.lastReason,
	}
}
