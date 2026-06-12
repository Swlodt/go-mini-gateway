package server

import (
	"net/http"
	"sync/atomic"
)

type reloadableHandler struct {
	value atomic.Value
}

func newReloadableHandler(handler http.Handler) *reloadableHandler {
	h := &reloadableHandler{}
	h.Store(handler)
	return h
}

func (h *reloadableHandler) Store(handler http.Handler) {
	if handler == nil {
		handler = http.NotFoundHandler()
	}
	h.value.Store(handler)
}

func (h *reloadableHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler, _ := h.value.Load().(http.Handler)
	if handler == nil {
		http.NotFound(w, r)
		return
	}

	handler.ServeHTTP(w, r)
}
