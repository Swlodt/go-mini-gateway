package concurrency

import (
	"log"
	"net/http"
)

func Middleware(name string, limiter *Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if limiter == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.TryAcquire() {
				log.Printf(
					"concurrency limit exceeded: limiter=%s method=%s path=%s remote=%s inUse=%d capacity=%d",
					name,
					r.Method,
					r.URL.Path,
					r.RemoteAddr,
					limiter.InUse(),
					limiter.Capacity(),
				)
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				http.Error(w, "concurrency limit exceeded", http.StatusServiceUnavailable)
				return
			}
			defer limiter.Release()
			next.ServeHTTP(w, r)
		})
	}
}
