package proxy

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

const (
	apiPrefix   = "/api"
	gatewayName = "go-mini-gateway"
)

type Handler struct {
	reverseProxy *httputil.ReverseProxy
}

func New(target string) (*Handler, error) {
	targetURL, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("parse proxy target url %q failed: %w", target, err)
	}
	if targetURL.Scheme == "" || targetURL.Host == "" {
		return nil, fmt.Errorf("invalid proxy target %q: schema and host are required", target)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()

	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 20
	transport.MaxConnsPerHost = 100
	transport.IdleConnTimeout = 90 * time.Second
	transport.ResponseHeaderTimeout = 10 * time.Second
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ExpectContinueTimeout = 1 * time.Second

	rp := &httputil.ReverseProxy{
		Transport: transport,

		Rewrite: func(pr *httputil.ProxyRequest) {
			rewriteRequest(pr, targetURL)
		},

		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Set("X-Gateway", gatewayName)
			return nil
		},

		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("proxy backend request failed: method=%s path=%s err=%v", r.Method, r.URL.Path, err)
			http.Error(w, "backend unavailable", http.StatusBadGateway)
		},
	}

	return &Handler{
		reverseProxy: rp,
	}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.reverseProxy.ServeHTTP(w, r)
}

func rewriteRequest(pr *httputil.ProxyRequest, target *url.URL) {
	pr.SetURL(target)

	path := strings.TrimPrefix(pr.In.URL.Path, apiPrefix)
	if path == "" {
		path = "/"
	}

	pr.Out.URL.Path = path
	pr.Out.URL.RawPath = ""

	pr.SetXForwarded()
	pr.Out.Header.Set("X-Gateway", gatewayName)

	traceID := pr.In.Header.Get("X-Trace-ID")
	if traceID == "" {
		traceID = fmt.Sprintf("trace-%d", time.Now().UnixNano())
	}
	pr.Out.Header.Set("X-Trace-ID", traceID)
}
