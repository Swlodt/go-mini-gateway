package concurrency

import "testing"

func TestLimiterTryAcquireAndRelease(t *testing.T) {
	limiter := NewLimiter("test", 2)

	if limiter == nil {
		t.Fatalf("limiter is nil")
	}

	if !limiter.TryAcquire() {
		t.Fatalf("first acquire should succeed")
	}

	if !limiter.TryAcquire() {
		t.Fatalf("second acquire should succeed")
	}

	if limiter.TryAcquire() {
		t.Fatalf("third acquire should fail")
	}

	if limiter.InUse() != 2 {
		t.Fatalf("inUse got %d, want 2", limiter.InUse())
	}

	limiter.Release()

	if limiter.InUse() != 1 {
		t.Fatalf("inUse got %d, want 1", limiter.InUse())
	}

	if !limiter.TryAcquire() {
		t.Fatalf("acquire after release should succeed")
	}
}

func TestLimiterReleaseWithoutAcquire(t *testing.T) {
	limiter := NewLimiter("test", 1)

	limiter.Release()

	if limiter.InUse() != 0 {
		t.Fatalf("inUse got %d, want 0", limiter.InUse())
	}
}

func TestLimiterSnapshot(t *testing.T) {
	limiter := NewLimiter("test", 3)

	limiter.TryAcquire()
	limiter.TryAcquire()

	snapshot := limiter.Snapshot()

	if snapshot.Name != "test" {
		t.Fatalf("name got %q, want test", snapshot.Name)
	}

	if snapshot.Capacity != 3 {
		t.Fatalf("capacity got %d, want 3", snapshot.Capacity)
	}

	if snapshot.InUse != 2 {
		t.Fatalf("inUse got %d, want 2", snapshot.InUse)
	}

	if snapshot.Available != 1 {
		t.Fatalf("available got %d, want 1", snapshot.Available)
	}
}

func TestNewLimiterDisabled(t *testing.T) {
	if limiter := NewLimiter("disabled", 0); limiter != nil {
		t.Fatalf("limiter should be nil when maxConcurrency <= 0")
	}

	if limiter := NewLimiter("disabled", -1); limiter != nil {
		t.Fatalf("limiter should be nil when maxConcurrency <= 0")
	}
}

func TestNilLimiterBehavior(t *testing.T) {
	var limiter *Limiter

	if !limiter.TryAcquire() {
		t.Fatalf("nil limiter should allow")
	}

	limiter.Release()

	if limiter.InUse() != 0 {
		t.Fatalf("nil limiter inUse got %d, want 0", limiter.InUse())
	}

	if limiter.Capacity() != 0 {
		t.Fatalf("nil limiter capacity got %d, want 0", limiter.Capacity())
	}

	if limiter.Name() != "" {
		t.Fatalf("nil limiter name got %q, want empty", limiter.Name())
	}
}
