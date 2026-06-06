package proxy

import (
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Handler struct {
	target *url.URL
	client *http.Client
}

func New(target string) (*Handler, error) {
	targetUrl, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	return &Handler{
		target: targetUrl,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	targetUrl := *h.target
	targetUrl.Path = strings.TrimPrefix(r.URL.Path, "/api")
	if targetUrl.Path == "" {
		targetUrl.Path = "/"
	}

	targetUrl.RawQuery = r.URL.RawQuery

	outReq, err := http.NewRequestWithContext(
		r.Context(),
		r.Method,
		targetUrl.String(),
		r.Body,
	)
	if err != nil {
		http.Error(w, "create backend request failed", http.StatusInternalServerError)
		return
	}
	outReq.Header = r.Header.Clone()

	resp, err := h.client.Do(outReq)
	if err != nil {
		log.Printf("proxy request failed: method=%s path=%s err=%v", r.Method, r.URL.Path, err)
		http.Error(w, "backend unavailable", http.StatusBadGateway)
		return
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
		}
	}(resp.Body)

	copyHeader(w.Header(), resp.Header)

	w.WriteHeader(resp.StatusCode)

	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("copy backend response failed: method=%s path=%s err=%v", r.Method, r.URL.Path, err)
	}

}

func copyHeader(dst, src http.Header) {
	for k, v := range src {
		for _, iv := range v {
			dst.Add(k, iv)
		}
	}
}
