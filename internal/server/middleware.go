package server

import (
	"context"
	"go-mini-gateway/internal/metrics"
	"log"
	"net/http"
	"time"
)

type statusRecorder struct {
	http.ResponseWriter

	statusCode   int
	bytesWritten int64
	wroteHeader  bool

	routeID string
}

func newStatusRecorder(w http.ResponseWriter) *statusRecorder {
	return &statusRecorder{
		ResponseWriter: w,
	}
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	if r.wroteHeader {
		return
	}
	r.statusCode = statusCode
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}

	n, err := r.ResponseWriter.Write(data)
	r.bytesWritten += int64(n)
	return n, err
}

func (r *statusRecorder) StatusCode() int {
	if r.statusCode == 0 {
		return http.StatusOK
	}
	return r.statusCode
}

func (r *statusRecorder) BytesWritten() int64 {
	return r.bytesWritten
}

func (r *statusRecorder) Flush() {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}

	flusher, ok := r.ResponseWriter.(http.Flusher)
	if ok {
		flusher.Flush()
	}
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (r *statusRecorder) SetRouteID(routeID string) {
	if routeID == "" {
		return
	}
	r.routeID = routeID
}

func (r *statusRecorder) RouteID() string {
	return r.routeID
}

func accessLogMiddleware(next http.Handler, register *metrics.Registry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		recorder := newStatusRecorder(w)
		next.ServeHTTP(recorder, r)

		cost := time.Since(start)
		routeID := recorder.RouteID()
		if routeID == "" {
			routeID = routeIDFromRequest(r)
		}

		statusCode := recorder.StatusCode()
		bytesWritten := recorder.BytesWritten()

		if register != nil {
			register.Record(metrics.Record{
				RouteID:     routeID,
				StatusCode:  statusCode,
				ByteWritten: bytesWritten,
				Latency:     cost,
			})
		}

		log.Printf(
			"access route=%s method=%s path=%s query=%q status=%d bytes=%d cost=%s remote=%s user_agent=%q",
			routeID,
			r.Method,
			r.URL.Path,
			r.URL.RawQuery,
			statusCode,
			bytesWritten,
			cost,
			r.RemoteAddr,
			r.UserAgent(),
		)
	})
}

func timeoutMiddleware(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
