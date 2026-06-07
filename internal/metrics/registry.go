package metrics

import (
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"
)

type Registry struct {
	mu        sync.RWMutex
	startTime time.Time
	total     *bucket
	routes    map[string]*bucket
}

type bucket struct {
	requests       int64
	bytesWritten   int64
	totalLatencyNs int64
	maxLatencyNs   int64
	statusCodes    map[int]int64
}

type Record struct {
	RouteID     string
	StatusCode  int
	ByteWritten int64
	Latency     time.Duration
}

type Snapshot struct {
	Uptime string                    `json:"uptime"`
	Total  BucketSnapshot            `json:"total"`
	Routes map[string]BucketSnapshot `json:"routes"`
}

type BucketSnapshot struct {
	Requests     int64            `json:"requests"`
	BytesWritten int64            `json:"bytesWritten"`
	AvgLatencyMs float64          `json:"avgLatencyMs"`
	MaxLatencyMs float64          `json:"maxLatencyMs"`
	StatusCodes  map[string]int64 `json:"statusCodes"`
}

func NewRegistry() *Registry {
	return &Registry{
		startTime: time.Now(),
		total:     newBucket(),
		routes:    make(map[string]*bucket),
	}
}

func newBucket() *bucket {
	return &bucket{
		statusCodes: make(map[int]int64),
	}
}

func (r *Registry) Record(record Record) {
	if r == nil {
		return
	}

	if record.RouteID == "" {
		record.RouteID = "unknown"
	}

	if record.StatusCode <= 0 {
		record.StatusCode = http.StatusOK
	}

	latencyNs := record.Latency.Nanoseconds()

	r.mu.Lock()
	defer r.mu.Unlock()

	recordBucket(r.total, record.StatusCode, record.ByteWritten, latencyNs)

	routeBucket := r.routes[record.RouteID]
	if routeBucket == nil {
		routeBucket = newBucket()
		r.routes[record.RouteID] = routeBucket
	}

	recordBucket(routeBucket, record.StatusCode, record.ByteWritten, latencyNs)
}

func recordBucket(b *bucket, statusCode int, bytesWritten int64, latencyNs int64) {
	b.requests++
	b.bytesWritten += bytesWritten
	b.totalLatencyNs += latencyNs
	b.statusCodes[statusCode]++

	if latencyNs > b.maxLatencyNs {
		b.maxLatencyNs = latencyNs
	}
}

func (r *Registry) Snapshot() Snapshot {
	if r == nil {
		return Snapshot{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	routes := make(map[string]BucketSnapshot, len(r.routes))

	routeIDs := make([]string, 0, len(r.routes))
	for routeID := range r.routes {
		routeIDs = append(routeIDs, routeID)
	}

	sort.Strings(routeIDs)

	for _, routeID := range routeIDs {
		routes[routeID] = snapshotBucket(r.routes[routeID])
	}

	return Snapshot{
		Uptime: time.Since(r.startTime).String(),
		Total:  snapshotBucket(r.total),
		Routes: routes,
	}
}

func snapshotBucket(b *bucket) BucketSnapshot {
	if b == nil {
		return BucketSnapshot{
			StatusCodes: make(map[string]int64),
		}
	}

	statusCodes := make(map[string]int64, len(b.statusCodes))

	codes := make([]int, 0, len(b.statusCodes))
	for code := range b.statusCodes {
		codes = append(codes, code)
	}
	sort.Ints(codes)

	for _, code := range codes {
		statusCodes[strconv.Itoa(code)] = b.statusCodes[code]
	}

	var avgLatencyMs float64
	if b.requests > 0 {
		avgLatencyMs = float64(b.totalLatencyNs) / float64(b.requests) / float64(time.Millisecond)
	}

	maxLatencyMs := float64(b.maxLatencyNs) / float64(time.Millisecond)

	return BucketSnapshot{
		Requests:     b.requests,
		BytesWritten: b.bytesWritten,
		AvgLatencyMs: avgLatencyMs,
		MaxLatencyMs: maxLatencyMs,
		StatusCodes:  statusCodes,
	}
}
