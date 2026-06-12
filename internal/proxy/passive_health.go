package proxy

import (
	"log"
	"sync"
	"time"
)

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
		log.Printf("passive health upstream recovered: route=%s upstream=%s successes=%d", s.routeID, s.upstreamID, s.consecutiveSuccesses)
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

	return PassiveHealthSnapshot{
		Enabled:              true,
		Healthy:              !s.unhealthy,
		Available:            !s.unhealthy || !now.Before(s.retryAfter),
		ConsecutiveFailures:  s.consecutiveFailures,
		ConsecutiveSuccesses: s.consecutiveSuccesses,
		FailureThreshold:     s.failureThreshold,
		SuccessThreshold:     s.successThreshold,
		UnhealthyDuration:    s.unhealthyDuration.String(),
		RetryAfter:           formatTime(s.retryAfter),
		LastFailureAt:        formatTime(s.lastFailureAt),
		LastSuccessAt:        formatTime(s.lastSuccessAt),
		LastReason:           s.lastReason,
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339Nano)
}
