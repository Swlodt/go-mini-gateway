package ratelimit

import (
	"log"
	"net/http"
)

func Middleware(name string, limiter *TokenBucket) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if limiter == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow() {
				log.Printf(
					"rate limit exceeded: limiter=%s method=%s path=%s remote=%s",
					name,
					r.Method,
					r.URL.Path,
					r.RemoteAddr,
				)

				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
