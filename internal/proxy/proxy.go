// Package proxy provides a reverse proxy that forwards requests to an upstream origin.
package proxy

import (
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// ReverseProxy wraps httputil.ReverseProxy with structured logging and Open-WR headers.
type ReverseProxy struct {
	origin *url.URL
	proxy  *httputil.ReverseProxy
	logger *slog.Logger
}

// NewReverseProxy creates a ReverseProxy that forwards all traffic to originStr.
// Returns an error if originStr is not a valid URL.
func NewReverseProxy(originStr string, logger *slog.Logger) (*ReverseProxy, error) {
	u, err := url.Parse(originStr)
	if err != nil {
		return nil, fmt.Errorf("proxy: invalid origin URL %q: %w", originStr, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("proxy: origin URL %q must have a scheme and host", originStr)
	}

	p := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(u)
			r.Out.Header.Set("X-Forwarded-Host", r.In.Host)
			if ip := clientIP(r.In); ip != "" {
				r.Out.Header.Set("X-Real-IP", ip)
			}
			r.Out.Header.Set("X-Open-WR", "1")
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			logger.Error("origin proxy error", "path", r.URL.Path, "err", err)
			http.Error(w, "Bad Gateway", http.StatusBadGateway)
		},
	}

	return &ReverseProxy{origin: u, proxy: p, logger: logger}, nil
}

// ServeHTTP proxies the request to the upstream origin.
func (rp *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rp.logger.Debug("proxying to origin", "host", rp.origin.Host, "path", r.URL.Path)
	rp.proxy.ServeHTTP(w, r)
}

//go:embed templates/demo.html
var demoFS embed.FS

var demoTmpl = template.Must(template.ParseFS(demoFS, "templates/demo.html"))

type demoData struct {
	Path string
}

// DemoOriginHandler returns a simple HTML handler used when no real origin is configured.
func DemoOriginHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = demoTmpl.Execute(w, demoData{Path: r.URL.Path})
	})
}

// clientIP extracts only the IP address (no port) from r.RemoteAddr.
// It prefers X-Forwarded-For if set (for reverse-proxy-aware setups).
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first (client) IP in the chain.
		parts := strings.SplitN(xff, ",", 2)
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
