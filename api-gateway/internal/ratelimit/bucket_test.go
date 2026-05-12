package ratelimit_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/distributed-db/api-gateway/internal/ratelimit"
)

// okHandler is a trivial upstream that always returns 200.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func makeRequest(handler http.Handler, ip string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/query", nil)
	req.RemoteAddr = ip + ":9999"
	handler.ServeHTTP(rec, req)
	return rec
}

// ── Allow / block behaviour ───────────────────────────────────────────────────

func TestRequestsWithinBurstAllPass(t *testing.T) {
	l := ratelimit.New(100, 5) // burst=5
	handler := l.Middleware(okHandler)

	for i := 0; i < 5; i++ {
		rec := makeRequest(handler, "1.2.3.4")
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d/%d: expected 200, got %d", i+1, 5, rec.Code)
		}
	}
}

func TestRequestOverBurstIsRejectedWith429(t *testing.T) {
	// rate≈0 so the bucket never meaningfully refills between calls.
	l := ratelimit.New(0.0001, 3) // burst=3
	handler := l.Middleware(okHandler)

	for i := 0; i < 3; i++ {
		makeRequest(handler, "10.0.0.1") // drain the bucket
	}

	rec := makeRequest(handler, "10.0.0.1")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after burst exhausted, got %d", rec.Code)
	}
}

func Test429ResponseBodyIsJSON(t *testing.T) {
	l := ratelimit.New(0.0001, 1)
	handler := l.Middleware(okHandler)

	makeRequest(handler, "2.2.2.2") // consume the one token
	rec := makeRequest(handler, "2.2.2.2")

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}
	if _, ok := body["error"]; !ok {
		t.Fatalf("JSON body missing 'error' field: %v", body)
	}
}

func Test429SetsRetryAfterHeader(t *testing.T) {
	l := ratelimit.New(0.0001, 1)
	handler := l.Middleware(okHandler)

	makeRequest(handler, "3.3.3.3")
	rec := makeRequest(handler, "3.3.3.3")

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header to be set on 429")
	}
}

// ── Per-IP isolation ──────────────────────────────────────────────────────────

func TestDifferentIPsHaveIndependentBuckets(t *testing.T) {
	l := ratelimit.New(0.0001, 1) // burst=1: each IP gets exactly one free request
	handler := l.Middleware(okHandler)

	ips := []string{"10.0.0.1", "10.0.0.2", "10.0.0.3", "10.0.0.4"}
	for _, ip := range ips {
		rec := makeRequest(handler, ip)
		if rec.Code != http.StatusOK {
			t.Fatalf("first request from %s should pass, got %d", ip, rec.Code)
		}
	}
}

func TestSecondRequestFromSameIPBlockedAfterBurstOne(t *testing.T) {
	l := ratelimit.New(0.0001, 1)
	handler := l.Middleware(okHandler)

	makeRequest(handler, "5.5.5.5")
	rec := makeRequest(handler, "5.5.5.5")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request should be blocked, got %d", rec.Code)
	}
}

func TestOneIPBlockedDoesNotAffectAnother(t *testing.T) {
	l := ratelimit.New(0.0001, 1)
	handler := l.Middleware(okHandler)

	// Exhaust IP A.
	makeRequest(handler, "6.6.6.6")
	makeRequest(handler, "6.6.6.6")

	// IP B should still pass.
	rec := makeRequest(handler, "7.7.7.7")
	if rec.Code != http.StatusOK {
		t.Fatalf("unrelated IP should not be blocked, got %d", rec.Code)
	}
}

// ── Token refill ──────────────────────────────────────────────────────────────

func TestBucketRefillsOverTime(t *testing.T) {
	// rate=10/s, burst=1: after 150 ms at least one token should have refilled.
	l := ratelimit.New(10, 1)
	handler := l.Middleware(okHandler)

	makeRequest(handler, "8.8.8.8") // consume the burst

	// Verify it's blocked immediately.
	rec := makeRequest(handler, "8.8.8.8")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 right after burst, got %d", rec.Code)
	}

	// Wait for refill (~1 token at 10/s takes 100 ms; 150 ms is safe).
	time.Sleep(150 * time.Millisecond)

	rec = makeRequest(handler, "8.8.8.8")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 after refill, got %d", rec.Code)
	}
}

// ── X-Forwarded-For ───────────────────────────────────────────────────────────

func TestXForwardedForUsedAsClientIP(t *testing.T) {
	l := ratelimit.New(0.0001, 1)
	handler := l.Middleware(okHandler)

	// Both requests come from the same RemoteAddr but different X-Forwarded-For IPs.
	send := func(xff string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "proxy.internal:8080"
		req.Header.Set("X-Forwarded-For", xff)
		handler.ServeHTTP(rec, req)
		return rec
	}

	r1 := send("192.168.1.10")
	r2 := send("192.168.1.11") // different real client

	if r1.Code != http.StatusOK {
		t.Fatalf("first unique IP should pass, got %d", r1.Code)
	}
	if r2.Code != http.StatusOK {
		t.Fatalf("second unique IP should also pass independently, got %d", r2.Code)
	}
}

func TestXForwardedForFirstIPUsedWhenMultiple(t *testing.T) {
	// burst=1: the "real" client IP (first in XFF) is rate-limited.
	l := ratelimit.New(0.0001, 1)
	handler := l.Middleware(okHandler)

	sendXFF := func(xff string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "proxy:80"
		req.Header.Set("X-Forwarded-For", xff)
		handler.ServeHTTP(rec, req)
		return rec
	}

	// First request: original client = 1.1.1.1.
	sendXFF("1.1.1.1, 10.0.0.1, 10.0.0.2") // consume token

	// Second request: same original client, different proxies.
	rec := sendXFF("1.1.1.1, 10.0.0.3")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("same originating IP should be blocked, got %d", rec.Code)
	}
}