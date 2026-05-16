// Package auth — strip.go
//
// StripExternalMasterToken is intentionally isolated in its own file because
// it is the gateway's primary security invariant: every inbound request must
// pass through it before reaching any routing or auth logic, ensuring no
// external caller can ever present a master token to a slave node.
//
// Security model
//
//	External client           Gateway                       Slave node
//	─────────────             ───────────────────────       ──────────
//	X-Master-Token: forged ──▶ StripExternalMasterToken ──▶ (no token)
//	                           InjectMasterToken         ──▶ X-Master-Token: <valid HMAC>
//
// The token is added ONLY by InjectMasterToken (middleware.go) when the
// gateway itself forwards a write to a slave. It is never passed through
// from the outside.
package auth

import "net/http"

// MasterTokenHeader is the HTTP header used to authenticate inter-node
// requests from the gateway to slave nodes.
const MasterTokenHeader = "X-Master-Token"

// StripExternalMasterToken removes MasterTokenHeader from every inbound
// request before any other middleware or routing logic runs.
//
// This must be registered as the first middleware in the chain (before
// RateLimit, ClientAuth, and the Router). Placement is enforced in
// cmd/gateway/main.go via the chi middleware stack ordering.
//
// Why stripping matters
//
// Without this middleware, a client that learns the HMAC secret could include
// a valid X-Master-Token and send write requests directly to a slave,
// bypassing the master entirely. Stripping unconditionally — regardless of
// whether the token is valid or forged — eliminates that attack surface
// at the boundary.
func StripExternalMasterToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Del removes all values for the key, including multiple values
		// set by a client attempting header injection.
		r.Header.Del(MasterTokenHeader)
		next.ServeHTTP(w, r)
	})
}