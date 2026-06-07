package health

import (
	"log"
	"net/http"
)

func Middleware(checker *Checker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if checker == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !checker.Available() {
				log.Printf(
					"backend unhealthy: route=%s method=%s path=%s remote=%s",
					checker.Name(),
					r.Method,
					r.URL.Path,
					r.RemoteAddr,
				)

				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.Header().Set("X-Gateway-Backend-Health", "unhealthy")
				http.Error(w, "backend unhealthy", http.StatusServiceUnavailable)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
