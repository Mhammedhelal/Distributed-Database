package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const masterTokenHeader = "X-Master-Token"

// tokenValidator validates HMAC-SHA256 master tokens issued by the gateway.
type tokenValidator struct {
	secret []byte
	ttl    time.Duration
}

func newTokenValidator(secret string, ttl time.Duration) *tokenValidator {
	return &tokenValidator{secret: []byte(secret), ttl: ttl}
}

func (v *tokenValidator) validate(token string) error {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return fmt.Errorf("malformed token")
	}
	ts, encoded := parts[0], parts[1]
	unixSec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp")
	}
	if time.Since(time.Unix(unixSec, 0)) > v.ttl {
		return fmt.Errorf("token expired")
	}
	gotMAC, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("bad base64")
	}
	h := hmac.New(sha256.New, v.secret)
	h.Write([]byte(ts))
	expected := h.Sum(nil)
	if !hmac.Equal(gotMAC, expected) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

// RequireMasterToken is middleware that rejects requests lacking a valid
// X-Master-Token. Use on /replication/* and /internal/* endpoints.
func RequireMasterToken(secret string, ttl time.Duration, next http.HandlerFunc) http.HandlerFunc {
	v := newTokenValidator(secret, ttl)
	return func(w http.ResponseWriter, r *http.Request) {
		tok := r.Header.Get(masterTokenHeader)
		if tok == "" {
			http.Error(w, `{"error":"missing master token"}`, http.StatusForbidden)
			return
		}
		if err := v.validate(tok); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"invalid master token: %s"}`, err), http.StatusForbidden)
			return
		}
		next(w, r)
	}
}