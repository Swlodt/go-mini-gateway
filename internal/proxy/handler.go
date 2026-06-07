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
	"time"
)

const (
	gatewayName = "go-mini-gateway"
)

type Options struct {
	RouteID     string
	Target      string
	StripPrefix string
}

type Handler struct {
	routeID      string
	stripPrefix  string
	target       *url.URL
	reverseProxy *httputil.ReverseProxy
}

func New(options Options) (*Handler, error) {
	targetURL, err := url.Parse(options.Target)
	if err != nil {
		return nil, fmt.Errorf("parse proxy target url %q failed: %w", options.Target, err)
	}
	if targetURL.Scheme == "" || targetURL.Host == "" {
		return nil, fmt.Errorf("invalid proxy target %q: schema and host are required", options.Target)
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
		target:      targetURL,
	}

	rp := &httputil.ReverseProxy{
		Transport: transport,

		Rewrite: func(pr *httputil.ProxyRequest) {
			h.rewriteRequest(pr)
		},

		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Set("X-Gateway", gatewayName)
			resp.Header.Set("X-Gateway-Route", h.routeID)
			return nil
		},

		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			statusCode := statusCodeFromProxyError(err)
			log.Printf("proxy backend request failed: route=%s method=%s path=%s status==%d err=%v", h.routeID, r.Method, r.URL.Path, statusCode, err)
			http.Error(w, http.StatusText(statusCode), statusCode)
		},
	}

	h.reverseProxy = rp

	return h, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.reverseProxy.ServeHTTP(w, r)
}

func (h *Handler) rewriteRequest(pr *httputil.ProxyRequest) {
	pr.SetURL(h.target)

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

	traceID := pr.In.Header.Get("X-Trace-ID")
	if traceID == "" {
		traceID = fmt.Sprintf("trace-%d", time.Now().UnixNano())
	}
	pr.Out.Header.Set("X-Trace-ID", traceID)
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
	if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() {
		return true
	}
	return false
}
