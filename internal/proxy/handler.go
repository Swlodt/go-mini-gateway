package proxy

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

const (
	gatewayName = "go-mini-gateway"
)

type Options struct {
	RouteID     string
	Target      string
	Upstreams   []UpstreamOptions
	StripPrefix string
}

type UpstreamOptions struct {
	ID  string
	URL string
}

type UpstreamSnapshot struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type upstream struct {
	id  string
	url *url.URL
}

type Handler struct {
	routeID      string
	stripPrefix  string
	upstreams    []*upstream
	nextIndex    atomic.Uint64
	transport    *http.Transport
	reverseProxy *httputil.ReverseProxy
}

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
	transport.ExpectContinueTimeout = 1 * time.Second

	h := &Handler{
		routeID:     options.RouteID,
		stripPrefix: options.StripPrefix,
		upstreams:   upstreams,
		transport:   transport,
	}

	rp := &httputil.ReverseProxy{
		Transport: h.transport,

		Rewrite: func(pr *httputil.ProxyRequest) {
			h.rewriteRequest(pr)
		},

		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Set("X-Gateway", gatewayName)
			resp.Header.Set("X-Gateway-Route", h.routeID)
			if resp.Request != nil {
				if upstreamID := resp.Request.Header.Get("X-Gateway-Upstream"); upstreamID != "" {
					resp.Header.Set("X-Gateway-Upstream", upstreamID)
				}
			}
			return nil
		},

		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			statusCode := statusCodeFromProxyError(err)
			log.Printf(
				"proxy backend request failed: route=%s method=%s path=%s status=%d err=%v",
				h.routeID,
				r.Method,
				r.URL.Path,
				statusCode,
				err,
			)
			http.Error(w, http.StatusText(statusCode), statusCode)
		},
	}

	h.reverseProxy = rp

	return h, nil
}

func buildUpstreams(options Options) ([]*upstream, error) {
	upstreamOptions := options.Upstreams
	if len(upstreamOptions) == 0 && options.Target != "" {
		upstreamOptions = []UpstreamOptions{
			{
				ID:  "default",
				URL: options.Target,
			},
		}
	}

	if len(upstreamOptions) == 0 {
		return nil, fmt.Errorf("proxy route %q requires target or upstreams", options.RouteID)
	}

	upstreams := make([]*upstream, 0, len(upstreamOptions))
	for i, option := range upstreamOptions {
		id := strings.TrimSpace(option.ID)
		if id == "" {
			id = fmt.Sprintf("upstream-%d", i+1)
		}

		targetURL, err := url.Parse(option.URL)
		if err != nil {
			return nil, fmt.Errorf("parse proxy upstream %q url %q failed: %w", id, option.URL, err)
		}
		if targetURL.Scheme == "" || targetURL.Host == "" {
			return nil, fmt.Errorf("invalid proxy upstream %q url %q: schema and host are required", id, option.URL)
		}

		upstreams = append(upstreams, &upstream{
			id:  id,
			url: targetURL,
		})
	}

	return upstreams, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.reverseProxy.ServeHTTP(w, r)
}

func (h *Handler) rewriteRequest(pr *httputil.ProxyRequest) {
	selected := h.selectUpstream()
	pr.SetURL(selected.url)

	path := pr.In.URL.Path

	if h.stripPrefix != "" {
		path = strings.TrimPrefix(path, h.stripPrefix)
	}

	if path == "" {
		path = "/"
	}

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	pr.Out.URL.Path = path
	pr.Out.URL.RawPath = ""

	pr.SetXForwarded()
	pr.Out.Header.Set("X-Gateway", gatewayName)
	pr.Out.Header.Set("X-Gateway-Route", h.routeID)
	pr.Out.Header.Set("X-Gateway-Upstream", selected.id)

	traceID := pr.In.Header.Get("X-Trace-ID")
	if traceID == "" {
		traceID = fmt.Sprintf("trace-%d", time.Now().UnixNano())
	}
	pr.Out.Header.Set("X-Trace-ID", traceID)
}

func (h *Handler) selectUpstream() *upstream {
	if len(h.upstreams) == 1 {
		return h.upstreams[0]
	}

	index := h.nextIndex.Add(1) - 1
	return h.upstreams[int(index%uint64(len(h.upstreams)))]
}

func (h *Handler) CloseIdleConnections() {
	if h == nil || h.transport == nil {
		return
	}
	h.transport.CloseIdleConnections()
}

func (h *Handler) UpstreamSnapshots() []UpstreamSnapshot {
	if h == nil {
		return nil
	}

	snapshots := make([]UpstreamSnapshot, 0, len(h.upstreams))
	for _, item := range h.upstreams {
		snapshots = append(snapshots, UpstreamSnapshot{
			ID:  item.id,
			URL: item.url.String(),
		})
	}
	return snapshots
}

func statusCodeFromProxyError(err error) int {
	if isTimeoutError(err) {
		return http.StatusGatewayTimeout
	}
	return http.StatusBadGateway
}

func isTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}
