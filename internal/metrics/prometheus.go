package metrics

import (
	"fmt"
	"sort"
	"strings"
)

func (r *Registry) PrometheusText() string {
	if r == nil {
		return ""
	}

	snapshot := r.Snapshot()

	var b strings.Builder

	writeHelp(&b, "gateway_uptime_seconds", "Gateway process uptime in seconds.")
	writeType(&b, "gateway_uptime_seconds", "gauge")
	_, _ = fmt.Fprintf(&b, "gateway_uptime_seconds %.6f\n\n", snapshot.UptimeSeconds)

	writeHelp(&b, "gateway_http_requests_total", "Total number of HTTP requests.")
	writeType(&b, "gateway_http_requests_total", "counter")
	_, _ = fmt.Fprintf(&b, "gateway_http_requests_total %d\n\n", snapshot.Total.Requests)

	writeHelp(&b, "gateway_http_bytes_written_total", "Total number of response bytes written.")
	writeType(&b, "gateway_http_bytes_written_total", "counter")
	_, _ = fmt.Fprintf(&b, "gateway_http_bytes_written_total %d\n\n", snapshot.Total.BytesWritten)

	writeHelp(&b, "gateway_http_request_duration_avg_milliseconds", "Average HTTP request duration in milliseconds.")
	writeType(&b, "gateway_http_request_duration_avg_milliseconds", "gauge")
	_, _ = fmt.Fprintf(&b, "gateway_http_request_duration_avg_milliseconds %.6f\n\n", snapshot.Total.AvgLatencyMs)

	writeHelp(&b, "gateway_http_request_duration_max_milliseconds", "Maximum observed HTTP request duration in milliseconds.")
	writeType(&b, "gateway_http_request_duration_max_milliseconds", "gauge")
	_, _ = fmt.Fprintf(&b, "gateway_http_request_duration_max_milliseconds %.6f\n\n", snapshot.Total.MaxLatencyMs)

	writeHelp(&b, "gateway_http_status_requests_total", "Total number of HTTP requests by status code.")
	writeType(&b, "gateway_http_status_requests_total", "counter")
	for _, status := range sortedStatusCodes(snapshot.Total.StatusCodes) {
		value := snapshot.Total.StatusCodes[status]
		_, _ = fmt.Fprintf(
			&b,
			"gateway_http_status_requests_total{status=\"%s\"} %d\n",
			escapeLabelValue(status),
			value,
		)
	}
	b.WriteString("\n")

	writeHelp(&b, "gateway_http_request_duration_seconds", "HTTP request duration in seconds.")
	writeType(&b, "gateway_http_request_duration_seconds", "histogram")
	writeHistogram(&b, "gateway_http_request_duration_seconds", nil, snapshot.Total.DurationHistogram)
	b.WriteString("\n")

	writeHelp(&b, "gateway_route_http_requests_total", "Total number of HTTP requests by route.")
	writeType(&b, "gateway_route_http_requests_total", "counter")
	for _, routeID := range sortedRouteIDs(snapshot.Routes) {
		bucket := snapshot.Routes[routeID]
		_, _ = fmt.Fprintf(
			&b,
			"gateway_route_http_requests_total{route=\"%s\"} %d\n",
			escapeLabelValue(routeID),
			bucket.Requests,
		)
	}
	b.WriteString("\n")

	writeHelp(&b, "gateway_route_http_bytes_written_total", "Total number of response bytes written by route.")
	writeType(&b, "gateway_route_http_bytes_written_total", "counter")
	for _, routeID := range sortedRouteIDs(snapshot.Routes) {
		bucket := snapshot.Routes[routeID]
		_, _ = fmt.Fprintf(
			&b,
			"gateway_route_http_bytes_written_total{route=\"%s\"} %d\n",
			escapeLabelValue(routeID),
			bucket.BytesWritten,
		)
	}
	b.WriteString("\n")

	writeHelp(&b, "gateway_route_http_status_requests_total", "Total number of HTTP requests by route and status code.")
	writeType(&b, "gateway_route_http_status_requests_total", "counter")
	for _, routeID := range sortedRouteIDs(snapshot.Routes) {
		bucket := snapshot.Routes[routeID]
		for _, status := range sortedStatusCodes(bucket.StatusCodes) {
			value := bucket.StatusCodes[status]
			_, _ = fmt.Fprintf(
				&b,
				"gateway_route_http_status_requests_total{route=\"%s\",status=\"%s\"} %d\n",
				escapeLabelValue(routeID),
				escapeLabelValue(status),
				value,
			)
		}
	}
	b.WriteString("\n")

	writeHelp(&b, "gateway_route_http_request_duration_avg_milliseconds", "Average HTTP request duration by route in milliseconds.")
	writeType(&b, "gateway_route_http_request_duration_avg_milliseconds", "gauge")
	for _, routeID := range sortedRouteIDs(snapshot.Routes) {
		bucket := snapshot.Routes[routeID]
		_, _ = fmt.Fprintf(
			&b,
			"gateway_route_http_request_duration_avg_milliseconds{route=\"%s\"} %.6f\n",
			escapeLabelValue(routeID),
			bucket.AvgLatencyMs,
		)
	}
	b.WriteString("\n")

	writeHelp(&b, "gateway_route_http_request_duration_max_milliseconds", "Maximum observed HTTP request duration by route in milliseconds.")
	writeType(&b, "gateway_route_http_request_duration_max_milliseconds", "gauge")
	for _, routeID := range sortedRouteIDs(snapshot.Routes) {
		bucket := snapshot.Routes[routeID]
		_, _ = fmt.Fprintf(
			&b,
			"gateway_route_http_request_duration_max_milliseconds{route=\"%s\"} %.6f\n",
			escapeLabelValue(routeID),
			bucket.MaxLatencyMs,
		)
	}
	b.WriteString("\n")

	writeHelp(&b, "gateway_route_http_request_duration_seconds", "HTTP request duration by route in seconds.")
	writeType(&b, "gateway_route_http_request_duration_seconds", "histogram")
	for _, routeID := range sortedRouteIDs(snapshot.Routes) {
		bucket := snapshot.Routes[routeID]
		writeHistogram(&b, "gateway_route_http_request_duration_seconds", map[string]string{
			"route": routeID,
		}, bucket.DurationHistogram)
	}
	b.WriteString("\n")

	return b.String()
}

func writeHistogram(b *strings.Builder, name string, labels map[string]string, histogram HistogramSnapshot) {
	for _, upperBound := range sortedHistogramBucketLabels(histogram.Buckets) {
		bucketLabels := cloneLabels(labels)
		bucketLabels["le"] = upperBound

		_, _ = fmt.Fprintf(
			b,
			"%s_bucket%s %d\n",
			name,
			formatLabels(bucketLabels),
			histogram.Buckets[upperBound],
		)
	}

	_, _ = fmt.Fprintf(
		b,
		"%s_sum%s %.9f\n",
		name,
		formatLabels(labels),
		histogram.SumSeconds,
	)

	_, _ = fmt.Fprintf(
		b,
		"%s_count%s %d\n",
		name,
		formatLabels(labels),
		histogram.Count,
	)
}

func writeHelp(b *strings.Builder, name string, help string) {
	_, _ = fmt.Fprintf(b, "# HELP %s %s\n", name, help)
}

func writeType(b *strings.Builder, name string, metricType string) {
	_, _ = fmt.Fprintf(b, "# TYPE %s %s\n", name, metricType)
}

func sortedRouteIDs(routes map[string]BucketSnapshot) []string {
	routeIDs := make([]string, 0, len(routes))
	for routeID := range routes {
		routeIDs = append(routeIDs, routeID)
	}
	sort.Strings(routeIDs)
	return routeIDs
}

func sortedStatusCodes(statusCodes map[string]int64) []string {
	codes := make([]string, 0, len(statusCodes))
	for code := range statusCodes {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

func sortedHistogramBucketLabels(buckets map[string]int64) []string {
	labels := make([]string, 0, len(buckets))
	for label := range buckets {
		if label == "+Inf" {
			continue
		}
		labels = append(labels, label)
	}

	sort.Slice(labels, func(i, j int) bool {
		return bucketLabelValue(labels[i]) < bucketLabelValue(labels[j])
	})

	labels = append(labels, "+Inf")
	return labels
}

func bucketLabelValue(label string) float64 {
	for i, bucket := range defaultDurationBucketsSeconds {
		if label == formatBucketBoundary(bucket) {
			return float64(i)
		}
	}
	return float64(len(defaultDurationBucketsSeconds))
}

func cloneLabels(labels map[string]string) map[string]string {
	cloned := make(map[string]string, len(labels)+1)
	for key, value := range labels {
		cloned[key] = value
	}
	return cloned
}

func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}

	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=\"%s\"", key, escapeLabelValue(labels[key])))
	}

	return "{" + strings.Join(parts, ",") + "}"
}

func escapeLabelValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}
