package metrics

import (
	"fmt"
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

	for status, value := range snapshot.Total.StatusCodes {
		_, _ = fmt.Fprintf(
			&b,
			"gateway_http_status_requests_total{status=%q} %d\n",
			escapeLabelValue(status),
			value,
		)
	}
	b.WriteString("\n")

	writeHelp(&b, "gateway_route_http_requests_total", "Total number of HTTP requests by route.")
	writeType(&b, "gateway_route_http_requests_total", "counter")
	for routeID, bucket := range snapshot.Routes {
		_, _ = fmt.Fprintf(
			&b,
			"gateway_route_http_requests_total{route=%q} %d\n",
			escapeLabelValue(routeID),
			bucket.Requests,
		)
	}
	b.WriteString("\n")

	writeHelp(&b, "gateway_route_http_bytes_written_total", "Total number of response bytes written by route.")
	writeType(&b, "gateway_route_http_bytes_written_total", "counter")
	for routeID, bucket := range snapshot.Routes {
		_, _ = fmt.Fprintf(
			&b,
			"gateway_route_http_bytes_written_total{route=%q} %d\n",
			escapeLabelValue(routeID),
			bucket.BytesWritten,
		)
	}
	b.WriteString("\n")

	writeHelp(&b, "gateway_route_http_status_requests_total", "Total number of HTTP requests by route and status code.")
	writeType(&b, "gateway_route_http_status_requests_total", "counter")
	for routeID, bucket := range snapshot.Routes {
		for status, value := range bucket.StatusCodes {
			_, _ = fmt.Fprintf(
				&b,
				"gateway_route_http_status_requests_total{route=%q,status=%q} %d\n",
				escapeLabelValue(routeID),
				escapeLabelValue(status),
				value,
			)
		}
	}
	b.WriteString("\n")

	writeHelp(&b, "gateway_route_http_request_duration_avg_milliseconds", "Average HTTP request duration by route in milliseconds.")
	writeType(&b, "gateway_route_http_request_duration_avg_milliseconds", "gauge")
	for routeID, bucket := range snapshot.Routes {
		_, _ = fmt.Fprintf(
			&b,
			"gateway_route_http_request_duration_avg_milliseconds{route=%q} %.6f\n",
			escapeLabelValue(routeID),
			bucket.AvgLatencyMs,
		)
	}
	b.WriteString("\n")

	writeHelp(&b, "gateway_route_http_request_duration_max_milliseconds", "Maximum observed HTTP request duration by route in milliseconds.")
	writeType(&b, "gateway_route_http_request_duration_max_milliseconds", "gauge")
	for routeID, bucket := range snapshot.Routes {
		_, _ = fmt.Fprintf(
			&b,
			"gateway_route_http_request_duration_max_milliseconds{route=%q} %.6f\n",
			escapeLabelValue(routeID),
			bucket.MaxLatencyMs,
		)
	}
	b.WriteString("\n")

	return b.String()
}

func writeHelp(b *strings.Builder, name string, help string) {
	_, _ = fmt.Fprintf(b, "# HELP %s %s\n", name, help)
}

func writeType(b *strings.Builder, name string, metricType string) {
	_, _ = fmt.Fprintf(b, "# TYPE %s %s\n", name, metricType)
}

func escapeLabelValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}
