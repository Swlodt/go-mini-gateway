package server

import (
	"net/http"
	"net/http/pprof"
)

func (s *Server) registerPprofRoutes(mux *http.ServeMux) {
	mux.Handle("/debug/pprof/", s.adminAuthMiddleware(http.HandlerFunc(pprof.Index)))
	mux.Handle("/debug/pprof/cmdline", s.adminAuthMiddleware(http.HandlerFunc(pprof.Cmdline)))
	mux.Handle("/debug/pprof/profile", s.adminAuthMiddleware(http.HandlerFunc(pprof.Profile)))
	mux.Handle("/debug/pprof/symbol", s.adminAuthMiddleware(http.HandlerFunc(pprof.Symbol)))
	mux.Handle("/debug/pprof/trace", s.adminAuthMiddleware(http.HandlerFunc(pprof.Trace)))

	mux.Handle("/debug/pprof/allocs", s.adminAuthMiddleware(pprof.Handler("allocs")))
	mux.Handle("/debug/pprof/block", s.adminAuthMiddleware(pprof.Handler("block")))
	mux.Handle("/debug/pprof/goroutine", s.adminAuthMiddleware(pprof.Handler("goroutine")))
	mux.Handle("/debug/pprof/heap", s.adminAuthMiddleware(pprof.Handler("heap")))
	mux.Handle("/debug/pprof/mutex", s.adminAuthMiddleware(pprof.Handler("mutex")))
	mux.Handle("/debug/pprof/threadcreate", s.adminAuthMiddleware(pprof.Handler("threadcreate")))
}
