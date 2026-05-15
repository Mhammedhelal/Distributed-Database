package auth_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"api-gateway/internal/auth"
)

// ── Signer.Generate ───────────────────────────────────────────────────────────

func TestGenerateProducesTokenWithDotSeparator(t *testing.T) {
	s := auth.NewSigner("secret", 30*time.Second)
	tok := s.Generate()
	if !strings.Contains(tok, ".") {
		t.Fatalf("expected <ts>.<mac> format, got %q", tok)
	}
}

func TestGenerateTimestampIsCurrentUnixSecond(t *testing.T) {
	before := time.Now().Unix()
	s := auth.NewSigner("secret", 30*time.Second)
	tok := s.Generate()
	after := time.Now().Unix()

	ts, _, _ := strings.Cut(tok, ".")
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		t.Fatalf("timestamp part is not an integer: %v", err)
	}
	if sec < before || sec > after {
		t.Fatalf("timestamp %d outside expected window [%d, %d]", sec, before, after)
	}
}

func TestGenerateTwoTokensAreDifferentOverTime(t *testing.T) {
	s := auth.NewSigner("secret", 30*time.Second)
	t1 := s.Generate()
	time.Sleep(1100 * time.Millisecond) // ensure different unix second
	t2 := s.Generate()
	if t1 == t2 {
		t.Fatal("tokens generated in different seconds should differ")
	}
}

// ── Signer.Validate ───────────────────────────────────────────────────────────

func TestValidateFreshTokenReturnsNil(t *testing.T) {
	s := auth.NewSigner("my-secret", 30*time.Second)
	if err := s.Validate(s.Generate()); err != nil {
		t.Fatalf("fresh token should be valid, got: %v", err)
	}
}

func TestValidateExpiredTokenReturnsError(t *testing.T) {
	s := auth.NewSigner("my-secret", time.Millisecond)
	tok := s.Generate()
	time.Sleep(5 * time.Millisecond)
	err := s.Validate(tok)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected 'expired' in error message, got: %v", err)
	}
}

func TestValidateWrongSecretReturnsError(t *testing.T) {
	signer := auth.NewSigner("secret-A", 30*time.Second)
	verifier := auth.NewSigner("secret-B", 30*time.Second)
	err := verifier.Validate(signer.Generate())
	if err == nil {
		t.Fatal("expected signature mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid token signature") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestValidateTamperedMACReturnsError(t *testing.T) {
	s := auth.NewSigner("secret", 30*time.Second)
	tok := s.Generate()
	ts, _, _ := strings.Cut(tok, ".")
	// Valid base64 but wrong MAC bytes.
	tampered := ts + ".dGhpcyBpcyBub3QgdGhlIHJlYWwgbWFj"
	if err := s.Validate(tampered); err == nil {
		t.Fatal("expected error for tampered MAC, got nil")
	}
}

func TestValidateTamperedTimestampReturnsError(t *testing.T) {
	s := auth.NewSigner("secret", 30*time.Second)
	tok := s.Generate()
	_, mac, _ := strings.Cut(tok, ".")
	// Old timestamp: valid format but MAC was computed against a different ts.
	tampered := "1000000000." + mac
	if err := s.Validate(tampered); err == nil {
		t.Fatal("expected error for tampered timestamp, got nil")
	}
}

func TestValidateFutureTokenReturnsError(t *testing.T) {
	s := auth.NewSigner("secret", 30*time.Second)
	futureTS := strconv.FormatInt(time.Now().Add(60*time.Second).Unix(), 10)
	// Doesn't matter that the MAC is wrong — timestamp check fires first.
	tok := futureTS + ".AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	err := s.Validate(tok)
	if err == nil {
		t.Fatal("expected error for future-dated token, got nil")
	}
	if !strings.Contains(err.Error(), "future") {
		t.Fatalf("expected 'future' in error message, got: %v", err)
	}
}

var malformedCases = []struct {
	name  string
	token string
}{
	{"empty string", ""},
	{"no dot separator", "justaplainstring"},
	{"invalid base64 mac", "1700000000.not!!valid%%base64"},
	{"empty mac after dot", "1700000000."},
	{"non-numeric timestamp", "notanumber.abc123"},
	{"only a dot", "."},
}

func TestValidateMalformedTokensReturnErrors(t *testing.T) {
	s := auth.NewSigner("secret", 30*time.Second)
	for _, tc := range malformedCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := s.Validate(tc.token); err == nil {
				t.Fatalf("expected error for %q, got nil", tc.token)
			}
		})
	}
}

// ── StripExternalMasterToken ──────────────────────────────────────────────────

func TestStripRemovesTokenBeforeHandlerRuns(t *testing.T) {
	var received string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.Header.Get(auth.MasterTokenHeader)
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/query", nil)
	req.Header.Set(auth.MasterTokenHeader, "forged-token")

	auth.StripExternalMasterToken(inner).ServeHTTP(httptest.NewRecorder(), req)

	if received != "" {
		t.Fatalf("X-Master-Token should be stripped, inner handler got %q", received)
	}
}

func TestStripPreservesUnrelatedHeaders(t *testing.T) {
	var received string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/query", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.MasterTokenHeader, "forged")

	auth.StripExternalMasterToken(inner).ServeHTTP(httptest.NewRecorder(), req)

	if received != "application/json" {
		t.Fatalf("Content-Type should be preserved, got %q", received)
	}
}

func TestStripCallsNextEvenWithoutToken(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	// No X-Master-Token at all.
	rec := httptest.NewRecorder()
	auth.StripExternalMasterToken(inner).ServeHTTP(rec, req)

	if !called {
		t.Fatal("inner handler was not called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

// ── ClientAuth ────────────────────────────────────────────────────────────────

func TestClientAuthDisabledWhenKeyIsEmpty(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := auth.ClientAuth("")(inner)

	req := httptest.NewRequest(http.MethodGet, "/query", nil)
	// No Authorization header — should still pass because auth is disabled.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with auth disabled, got %d", rec.Code)
	}
}

func TestClientAuthAcceptsValidBearerToken(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := auth.ClientAuth("valid-api-key")(inner)

	req := httptest.NewRequest(http.MethodGet, "/query", nil)
	req.Header.Set("Authorization", "Bearer valid-api-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestClientAuthRejectsWrongKey(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := auth.ClientAuth("correct-key")(inner)

	req := httptest.NewRequest(http.MethodGet, "/query", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong key, got %d", rec.Code)
	}
}

func TestClientAuthRejectsMissingAuthorizationHeader(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := auth.ClientAuth("any-key")(inner)

	req := httptest.NewRequest(http.MethodGet, "/query", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestClientAuthRejectsNonBearerScheme(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := auth.ClientAuth("my-key")(inner)

	req := httptest.NewRequest(http.MethodGet, "/query", nil)
	req.Header.Set("Authorization", "Basic my-key") // Basic, not Bearer
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for non-Bearer scheme, got %d", rec.Code)
	}
}

// ── InjectMasterToken ─────────────────────────────────────────────────────────

func TestInjectSetsValidToken(t *testing.T) {
	s := auth.NewSigner("secret", 30*time.Second)
	req := httptest.NewRequest(http.MethodPost, "/replication/apply", nil)
	auth.InjectMasterToken(req, s)

	tok := req.Header.Get(auth.MasterTokenHeader)
	if tok == "" {
		t.Fatal("expected X-Master-Token to be set")
	}
	if err := s.Validate(tok); err != nil {
		t.Fatalf("injected token failed validation: %v", err)
	}
}

func TestInjectOverwritesPreviousValue(t *testing.T) {
	s := auth.NewSigner("secret", 30*time.Second)
	req := httptest.NewRequest(http.MethodPost, "/replication/apply", nil)
	req.Header.Set(auth.MasterTokenHeader, "stale-token")
	auth.InjectMasterToken(req, s)

	tok := req.Header.Get(auth.MasterTokenHeader)
	if tok == "stale-token" {
		t.Fatal("InjectMasterToken must overwrite the previous value")
	}
	if err := s.Validate(tok); err != nil {
		t.Fatalf("overwritten token failed validation: %v", err)
	}
}