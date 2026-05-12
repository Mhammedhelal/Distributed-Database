// Package router implements the gateway's routing and reverse-proxy logic.
//
// Routing rules (in priority order):
//
//  1. GET  /cluster/status   → health proxy (handled by health package)
//  2. POST /admin/*           → master only (admin ops including DROP DB)
//  3. POST /query             → master (writes; replication token injected)
//  4. GET  /query             → any alive worker, round-robin
//  5. POST /replication/*     → blocked from external clients (403)
//  6. GET  /health            → gateway own health
package router

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/distributed-db/api-gateway/internal/auth"
)

// Config holds everything the router needs to know about the cluster topology.
type Config struct {
	MasterURL  string
	WorkerURLs []string
	Signer     *auth.Signer
	Logger     *slog.Logger
}

// Router is the core request dispatcher.
type Router struct {
	cfg        Config
	workerIdx  atomic.Uint64 // for round-robin worker selection
}

// New creates a Router from cfg.
func New(cfg Config) *Router {
	return &Router{cfg: cfg}
}

// ServeHTTP implements http.Handler.
func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	// Block any attempt to hit replication endpoints directly.
	case strings.HasPrefix(path, "/replication/"):
		writeJSONError(w, http.StatusForbidden,
			"replication endpoints are internal — not accessible externally")

	// Admin operations go exclusively to master.
	case strings.HasPrefix(path, "/admin/"):
		rt.proxyTo(w, r, rt.cfg.MasterURL, false)

	// Writes go to master; gateway injects the master token so master can
	// later fan the write out to workers with authority.
	case r.Method == http.MethodPost && path == "/query":
		rt.proxyTo(w, r, rt.cfg.MasterURL, false)

	// Reads load-balance across alive workers.
	case r.Method == http.MethodGet && path == "/query":
		target := rt.pickWorker()
		if target == "" {
			writeJSONError(w, http.StatusServiceUnavailable,
				"no healthy workers available")
			return
		}
		// Indicate eventual consistency on slave reads.
		w.Header().Set("X-Consistency-Level", "eventual")
		rt.proxyTo(w, r, target, false)

	// Analytics (bonus) — proxied to worker-1 with master token so
	// the C++ worker accepts the request.
	case path == "/analytics":
		if len(rt.cfg.WorkerURLs) == 0 {
			writeJSONError(w, http.StatusServiceUnavailable, "no workers configured")
			return
		}
		rt.proxyTo(w, r, rt.cfg.WorkerURLs[0], true)

	// Full-text search (bonus) — proxied to worker-2 (index 1).
	case path == "/search":
		if len(rt.cfg.WorkerURLs) < 2 {
			writeJSONError(w, http.StatusServiceUnavailable, "worker-2 not configured")
			return
		}
		rt.proxyTo(w, r, rt.cfg.WorkerURLs[1], false)

	default:
		writeJSONError(w, http.StatusNotFound, fmt.Sprintf("no route for %s %s", r.Method, path))
	}
}

// proxyTo reverse-proxies r to targetBase, optionally injecting the master token.
func (rt *Router) proxyTo(w http.ResponseWriter, r *http.Request, targetBase string, injectToken bool) {
	dest, err := url.Parse(targetBase)
	if err != nil {
		rt.cfg.Logger.Error("invalid target URL", "target", targetBase, "err", err)
		writeJSONError(w, http.StatusBadGateway, "gateway misconfiguration")
		return
	}

	// Build outbound request.
	outURL := *dest
	outURL.Path = r.URL.Path
	outURL.RawQuery = r.URL.RawQuery

	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, outURL.String(), r.Body)
	if err != nil {
		rt.cfg.Logger.Error("build outbound request", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to build upstream request")
		return
	}

	// Copy safe headers (skip hop-by-hop).
	copyHeaders(outReq.Header, r.Header)

	// Inject master token if required (replication writes to slaves).
	if injectToken {
		auth.InjectMasterToken(outReq, rt.cfg.Signer)
	}

	// Tag the forwarded request with the original client IP.
	outReq.Header.Set("X-Forwarded-For", clientIP(r))
	outReq.Header.Set("X-Forwarded-Proto", scheme(r))

	resp, err := http.DefaultTransport.RoundTrip(outReq)
	if err != nil {
		rt.cfg.Logger.Error("upstream request failed",
			"target", targetBase,
			"path", r.URL.Path,
			"err", err)
		writeJSONError(w, http.StatusBadGateway,
			fmt.Sprintf("upstream unreachable: %v", err))
		return
	}
	defer resp.Body.Close()

	// Forward response headers and status.
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		rt.cfg.Logger.Warn("copy response body", "err", err)
	}
}

// pickWorker returns a worker URL using round-robin. Returns "" if no workers.
func (rt *Router) pickWorker() string {
	if len(rt.cfg.WorkerURLs) == 0 {
		return ""
	}
	idx := rt.workerIdx.Add(1) - 1
	return rt.cfg.WorkerURLs[idx%uint64(len(rt.cfg.WorkerURLs))]
}

// hopByHop lists headers that must not be forwarded.
var hopByHop = map[string]bool{
	"Connection":          true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"TE":                  true,
	"Trailers":            true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
}

func copyHeaders(dst, src http.Header) {
	for k, vv := range src {
		if hopByHop[k] {
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	host, _, _ := strings.Cut(r.RemoteAddr, ":")
	return host
}

func scheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	//nolint:errcheck
	w.Write([]byte(`{"error":"` + strings.ReplaceAll(msg, `"`, `'`) + `"}`))
}