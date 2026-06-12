package proxy

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"
)

type selectedUpstreamContextKey struct{}

func New(options Options) (*Handler, error) {
	upstreams, err := buildUpstreams(options)
	if err != nil {
		return nil, err
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 20
	transport.MaxConnsPerHost = 100
	transport.IdleConnTimeout = 90 * time.Second
	transport.ResponseHeaderTimeout = 10 * time.Second
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ExpectContinueTimeout = time.Second

	h := &Handler{
		routeID:     options.RouteID,
		stripPrefix: options.StripPrefix,
		upstreams:   upstreams,
		transport:   transport,
	}

	h.reverseProxy = &httputil.ReverseProxy{
		Transport:      h.transport,
		Rewrite:        h.rewriteRequest,
		ModifyResponse: h.modifyResponse,
		ErrorHandler:   h.handleProxyError,
	}

	return h, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	selected := h.selectUpstream()
	if selected == nil {
		log.Printf("no available upstream: route=%s method=%s path=%s", h.routeID, r.Method, r.URL.Path)
		w.Header().Set("X-Gateway", gatewayName)
		w.Header().Set("X-Gateway-Route", h.routeID)
		http.Error(w, "no available upstream", http.StatusServiceUnavailable)
		return
	}

	ctx := context.WithValue(r.Context(), selectedUpstreamContextKey{}, selected)
	h.reverseProxy.ServeHTTP(w, r.WithContext(ctx))
}

func (h *Handler) rewriteRequest(pr *httputil.ProxyRequest) {
	selected := selectedUpstreamFromContext(pr.In.Context())
	if selected == nil {
		selected = h.selectUpstream()
	}
	if selected == nil {
		return
	}

	pr.SetURL(selected.url)
	pr.Out.URL.Path = rewritePath(pr.In.URL.Path, h.stripPrefix)
	pr.Out.URL.RawPath = ""

	pr.SetXForwarded()
	pr.Out.Header.Set("X-Gateway", gatewayName)
	pr.Out.Header.Set("X-Gateway-Route", h.routeID)
	pr.Out.Header.Set("X-Gateway-Upstream", selected.id)
	pr.Out.Header.Set("X-Trace-ID", traceIDFromRequest(pr.In))
}

func (h *Handler) modifyResponse(resp *http.Response) error {
	selected := selectedUpstreamFromContext(resp.Request.Context())
	if selected != nil {
		if resp.StatusCode >= http.StatusInternalServerError {
			selected.recordFailure(fmt.Sprintf("backend status=%d", resp.StatusCode))
		} else {
			selected.recordSuccess()
		}
	}

	resp.Header.Set("X-Gateway", gatewayName)
	resp.Header.Set("X-Gateway-Route", h.routeID)
	if selected != nil {
		resp.Header.Set("X-Gateway-Upstream", selected.id)
	} else if resp.Request != nil {
		if upstreamID := resp.Request.Header.Get("X-Gateway-Upstream"); upstreamID != "" {
			resp.Header.Set("X-Gateway-Upstream", upstreamID)
		}
	}
	return nil
}

func (h *Handler) handleProxyError(w http.ResponseWriter, r *http.Request, err error) {
	selected := selectedUpstreamFromContext(r.Context())
	if selected != nil {
		selected.recordFailure(err.Error())
	}

	statusCode := statusCodeFromProxyError(err)
	log.Printf(
		"proxy backend request failed: route=%s upstream=%s method=%s path=%s status=%d err=%v",
		h.routeID,
		upstreamIDOrEmpty(selected),
		r.Method,
		r.URL.Path,
		statusCode,
		err,
	)
	http.Error(w, http.StatusText(statusCode), statusCode)
}

func rewritePath(path string, stripPrefix string) string {
	if stripPrefix != "" {
		path = strings.TrimPrefix(path, stripPrefix)
	}
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}

func traceIDFromRequest(r *http.Request) string {
	traceID := r.Header.Get("X-Trace-ID")
	if traceID == "" {
		traceID = fmt.Sprintf("trace-%d", time.Now().UnixNano())
	}
	return traceID
}
