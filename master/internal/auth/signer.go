// Package auth provides the HMAC-SHA256 token signer used by the master
// to authenticate replication requests sent to worker nodes.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"time"
)

// Signer generates X-Master-Token values.
type Signer struct {
	secret []byte
}

// New creates a Signer with the given shared secret.
func New(secret string) *Signer {
	return &Signer{secret: []byte(secret)}
}

// Generate returns a fresh signed token: "<unix-ts>.<HMAC-SHA256>".
func (s *Signer) Generate() string {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	h := hmac.New(sha256.New, s.secret)
	h.Write([]byte(ts))
	mac := base64.RawURLEncoding.EncodeToString(h.Sum(nil))
	return ts + "." + mac
}