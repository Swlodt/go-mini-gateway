package ratelimit

import (
	"testing"
	"time"
)

func TestTokenBucketAllowConsumesTokens(t *testing.T) {
	bucket := NewTokenBucket("test", 10, 2)
	defer bucket.Close()

	if bucket == nil {
		t.Fatalf("bucket is nil")
	}

	if !bucket.Allow() {
		t.Fatalf("first allow should succeed")
	}

	if !bucket.Allow() {
		t.Fatalf("second allow should succeed")
	}

	if bucket.Allow() {
		t.Fatalf("third allow should fail because burst is exhausted")
	}
}

func TestTokenBucketRefill(t *testing.T) {
	bucket := NewTokenBucket("test", 20, 1)
	defer bucket.Close()

	if !bucket.Allow() {
		t.Fatalf("first allow should succeed")
	}

	if bucket.Allow() {
		t.Fatalf("second allow should fail before refill")
	}

	time.Sleep(80 * time.Millisecond)

	if !bucket.Allow() {
		t.Fatalf("allow should succeed after refill")
	}
}

func TestTokenBucketSnapshot(t *testing.T) {
	bucket := NewTokenBucket("test", 10, 3)
	defer bucket.Close()

	bucket.Allow()

	snapshot := bucket.Snapshot()

	if snapshot.Name != "test" {
		t.Fatalf("name got %q, want test", snapshot.Name)
	}

	if snapshot.Rate != 10 {
		t.Fatalf("rate got %d, want 10", snapshot.Rate)
	}

	if snapshot.Burst != 3 {
		t.Fatalf("burst got %d, want 3", snapshot.Burst)
	}

	if snapshot.AvailableTokens != 2 {
		t.Fatalf("available tokens got %d, want 2", snapshot.AvailableTokens)
	}
}

func TestTokenBucketDefaultBurst(t *testing.T) {
	bucket := NewTokenBucket("test", 5, 5)
	defer bucket.Close()

	snapshot := bucket.Snapshot()

	if snapshot.Burst != 5 {
		t.Fatalf("burst got %d, want 5", snapshot.Burst)
	}
}

func TestNewTokenBucketDisabled(t *testing.T) {
	if bucket := NewTokenBucket("disabled", 0, 10); bucket != nil {
		t.Fatalf("bucket should be nil when rate <= 0")
	}

	if bucket := NewTokenBucket("disabled", -1, 10); bucket != nil {
		t.Fatalf("bucket should be nil when rate <= 0")
	}
}

func TestTokenBucketCloseIdempotent(t *testing.T) {
	bucket := NewTokenBucket("test", 10, 1)

	bucket.Close()
	bucket.Close()
}

func TestNilTokenBucketBehavior(t *testing.T) {
	var bucket *TokenBucket

	if !bucket.Allow() {
		t.Fatalf("nil bucket should allow")
	}

	bucket.Close()

	snapshot := bucket.Snapshot()
	if snapshot.Name != "" {
		t.Fatalf("nil snapshot name got %q, want empty", snapshot.Name)
	}
}
