// Package auth provides HMAC-SHA256 token generation and validation
// for inter-node communication between the gateway and slave nodes.
//
// Token format (URL-safe base64 of the raw bytes):
//
//	<unix-timestamp-seconds>.<HMAC-SHA256(secret, timestamp)>
//
// The timestamp is included so tokens expire after TokenTTL and cannot
// be replayed by an attacker who captures one.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Signer generates and validates X-Master-Token values.
type Signer struct {
	secret []byte
	ttl    time.Duration
}

// NewSigner creates a Signer with the given shared secret and token TTL.
func NewSigner(secret string, ttl time.Duration) *Signer {
	return &Signer{
		secret: []byte(secret),
		ttl:    ttl,
	}
}

// Generate creates a new signed token valid for Signer.ttl.
func (s *Signer) Generate() string {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	mac := s.sign(ts)
	return ts + "." + base64.RawURLEncoding.EncodeToString(mac)
}

// Validate checks that token is well-formed, not expired, and has a valid
// HMAC. Returns nil on success, a descriptive error otherwise.
func (s *Signer) Validate(token string) error {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return fmt.Errorf("malformed token: missing separator")
	}

	ts, encodedMAC := parts[0], parts[1]

	unixSec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return fmt.Errorf("malformed token: invalid timestamp")
	}

	issued := time.Unix(unixSec, 0)
	if time.Since(issued) > s.ttl {
		return fmt.Errorf("token expired (issued %s, ttl %s)", issued, s.ttl)
	}
	// Also reject tokens from the future (clock skew > 5 s).
	if time.Until(issued) > 5*time.Second {
		return fmt.Errorf("token issued in the future")
	}

	gotMAC, err := base64.RawURLEncoding.DecodeString(encodedMAC)
	if err != nil {
		return fmt.Errorf("malformed token: bad base64")
	}

	expected := s.sign(ts)
	if !hmac.Equal(gotMAC, expected) {
		return fmt.Errorf("invalid token signature")
	}

	return nil
}

// sign returns HMAC-SHA256(secret, message).
func (s *Signer) sign(message string) []byte {
	h := hmac.New(sha256.New, s.secret)
	h.Write([]byte(message))
	return h.Sum(nil)
}