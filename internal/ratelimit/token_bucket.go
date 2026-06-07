package ratelimit

import (
	"sync"
	"time"
)

type TokenBucket struct {
	name   string
	rate   int
	burst  int
	tokens chan struct{}

	ticker *time.Ticker
	done   chan struct{}
	once   sync.Once
}

type SnapShot struct {
	Name            string `json:"name"`
	Rate            int    `json:"rate"`
	Burst           int    `json:"burst"`
	AvailableTokens int    `json:"availableTokens"`
}

func NewTokenBucket(name string, rate int, burst int) *TokenBucket {
	if rate <= 0 {
		return nil
	}
	if burst <= 0 {
		return nil
	}
	b := &TokenBucket{
		name:   name,
		rate:   rate,
		burst:  burst,
		tokens: make(chan struct{}, burst),
		ticker: time.NewTicker(time.Second / time.Duration(rate)),
		done:   make(chan struct{}),
	}
	for i := 0; i < burst; i++ {
		b.tokens <- struct{}{}
	}
	go b.refill()
	return b
}

func (b *TokenBucket) Allow() bool {
	if b == nil {
		return true
	}
	select {
	case <-b.tokens:
		return true
	default:
		return false
	}
}

func (b *TokenBucket) Close() {
	if b == nil {
		return
	}
	b.once.Do(func() {
		b.ticker.Stop()
		close(b.done)
	})
}

func (b *TokenBucket) refill() {
	for {
		select {
		case <-b.ticker.C:
			select {
			case b.tokens <- struct{}{}:
			default:
			}
		case <-b.done:
			return
		}
	}
}

func (b *TokenBucket) Snapshot() SnapShot {
	if b == nil {
		return SnapShot{}
	}
	return SnapShot{
		Name:            b.name,
		Rate:            b.rate,
		Burst:           b.burst,
		AvailableTokens: len(b.tokens),
	}
}
