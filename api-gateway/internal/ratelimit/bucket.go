// Package ratelimit provides a per-IP token-bucket rate limiter middleware.
// Each unique client IP gets its own bucket that refills at RequestsPerSecond
// tokens/s and allows up to Burst tokens in a single surge.
package ratelimit

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// bucket is a single client's token bucket state.
type bucket struct {
	mu       sync.Mutex
	tokens   float64
	lastSeen time.Time
}

// Limiter holds all per-IP buckets and the shared rate parameters.
type Limiter struct {
	rate  float64 // tokens per second
	burst float64 // maximum tokens

	mu      sync.Mutex
	buckets map[string]*bucket

	// cleanupInterval controls how often idle buckets are evicted.
	cleanupInterval time.Duration
	// idleTimeout is how long a bucket must be unused before eviction.
	idleTimeout time.Duration
}

// New creates a Limiter with the given steady-state rate and burst capacity.
func New(requestsPerSecond float64, burst int) *Limiter {
	l := &Limiter{
		rate:            requestsPerSecond,
		burst:           float64(burst),
		buckets:         make(map[string]*bucket),
		cleanupInterval: 5 * time.Minute,
		idleTimeout:     10 * time.Minute,
	}
	go l.cleanupLoop()
	return l
}

// Middleware returns an http.Handler middleware that enforces the rate limit.
// Excess requests receive 429 Too Many Requests.
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !l.allow(ip) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			//nolint:errcheck
			w.Write([]byte(`{"error":"rate limit exceeded"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// allow consumes one token from the bucket for ip and returns true if
// the request is within the rate limit.
func (l *Limiter) allow(ip string) bool {
	l.mu.Lock()
	b, ok := l.buckets[ip]
	if !ok {
		b = &bucket{tokens: l.burst, lastSeen: time.Now()}
		l.buckets[ip] = b
	}
	l.mu.Unlock()

	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastSeen).Seconds()
	b.lastSeen = now

	// Refill tokens based on elapsed time.
	b.tokens += elapsed * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// cleanupLoop periodically removes buckets that haven't been used recently.
func (l *Limiter) cleanupLoop() {
	ticker := time.NewTicker(l.cleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		l.mu.Lock()
		for ip, b := range l.buckets {
			b.mu.Lock()
			idle := time.Since(b.lastSeen)
			b.mu.Unlock()
			if idle > l.idleTimeout {
				delete(l.buckets, ip)
			}
		}
		l.mu.Unlock()
	}
}

// clientIP extracts the real client IP, honouring X-Forwarded-For when
// the gateway is behind another proxy.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Use the leftmost (original client) IP.
		if idx := len(xff); idx > 0 {
			parts := splitFirst(xff, ',')
			if ip := net.ParseIP(parts); ip != nil {
				return ip.String()
			}
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if ip := net.ParseIP(xri); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func splitFirst(s, sep byte) string {
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			return trimSpace(s[:i])
		}
	}
	return trimSpace(s)
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}