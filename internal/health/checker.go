package health

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Checker struct {
	name     string
	target   *url.URL
	path     string
	interval time.Duration
	timeout  time.Duration

	client *http.Client

	checked atomic.Bool
	healthy atomic.Bool

	mu          sync.RWMutex
	lastCheckAt time.Time
	lastReason  string

	done chan struct{}
	once sync.Once
}

type Snapshot struct {
	Name          string `json:"name"`
	Target        string `json:"target"`
	Path          string `json:"path"`
	Interval      string `json:"interval"`
	Timeout       string `json:"timeout"`
	Checked       bool   `json:"checked"`
	Healthy       bool   `json:"healthy"`
	LastCheckedAt string `json:"lastCheckedAt,omitempty"`
	LastReason    string `json:"lastReason,omitempty"`
}

type Options struct {
	Name     string
	Target   string
	Path     string
	Interval time.Duration
	Timeout  time.Duration
}

func NewChecker(options Options) (*Checker, error) {
	if options.Name == "" {
		return nil, fmt.Errorf("health checker name is required")
	}

	targetURL, err := url.Parse(options.Target)
	if err != nil {
		return nil, fmt.Errorf("parse health check target %q failed: %w", options.Target, err)
	}

	if targetURL.Scheme == "" || targetURL.Host == "" {
		return nil, fmt.Errorf("invalid health check target %q: scheme and host are required", options.Target)
	}

	if options.Path == "" {
		options.Path = "/health"
	}

	if !strings.HasPrefix(options.Path, "/") {
		options.Path = "/" + options.Path
	}

	if options.Interval <= 0 {
		return nil, fmt.Errorf("invalid health check interval must be positive")
	}

	if options.Timeout <= 0 {
		return nil, fmt.Errorf("invalid health check timeout must be positive")
	}

	c := &Checker{
		name:     options.Name,
		target:   targetURL,
		path:     options.Path,
		interval: options.Interval,
		timeout:  options.Timeout,
		client: &http.Client{
			Timeout: options.Timeout,
		},
		done: make(chan struct{}),
	}
	return c, nil
}

func (c *Checker) Start() {
	if c == nil {
		return
	}
	go c.loop()
}

func (c *Checker) Close() {
	if c == nil {
		return
	}
	c.once.Do(func() {
		close(c.done)
	})
}

func (c *Checker) Available() bool {
	if c == nil {
		return true
	}
	if !c.checked.Load() {
		return true
	}
	return c.healthy.Load()
}

func (c *Checker) IsHealthy() bool {
	if c == nil {
		return true
	}
	return c.healthy.Load()
}

func (c *Checker) Name() string {
	if c == nil {
		return ""
	}
	return c.name
}

func (c *Checker) loop() {
	c.checkOnce()
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.checkOnce()
		case <-c.done:
			return
		}
	}
}

func (c *Checker) checkOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	healthURL := c.healthURL()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		c.update(false, fmt.Sprintf("create health check request failed: %v", err))
		return
	}

	resp, err := c.client.Do(req)
	if err != nil {
		c.update(false, fmt.Sprintf("health check request failed: %v", err))
		return
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
		}
	}(resp.Body)

	_, _ = io.Copy(io.Discard, resp.Body)

	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !ok {
		c.update(ok, fmt.Sprintf("health check status=%d", resp.StatusCode))
		return
	}

	c.update(ok, fmt.Sprintf("health check status=%d", resp.StatusCode))
}

func (c *Checker) healthURL() string {
	u := *c.target
	u.Path = c.path
	u.RawPath = ""
	u.RawQuery = ""
	return u.String()
}

func (c *Checker) update(healthy bool, reason string) {
	oldChecked := c.checked.Load()
	oldHealthy := c.healthy.Load()

	c.mu.Lock()
	c.lastCheckAt = time.Now()
	c.lastReason = reason
	c.mu.Unlock()

	c.healthy.Store(healthy)
	c.checked.Store(true)

	if !oldChecked || oldHealthy != healthy {
		log.Printf(
			"backend health changed: route=%s healthy=%v reason=%s",
			c.name,
			healthy,
			reason,
		)
	}
}

func (c *Checker) Snapshot() Snapshot {
	if c == nil {
		return Snapshot{
			Checked: true,
			Healthy: true,
		}
	}

	c.mu.RLock()
	lastCheckAt := c.lastCheckAt
	lastReason := c.lastReason
	c.mu.RUnlock()

	var lastCheckedAsText string
	if !lastCheckAt.IsZero() {
		lastCheckedAsText = lastCheckAt.Format(time.RFC3339Nano)
	}

	return Snapshot{
		Name:          c.name,
		Target:        c.target.String(),
		Path:          c.path,
		Interval:      c.interval.String(),
		Timeout:       c.timeout.String(),
		Checked:       c.checked.Load(),
		Healthy:       c.healthy.Load(),
		LastCheckedAt: lastCheckedAsText,
		LastReason:    lastReason,
	}
}
