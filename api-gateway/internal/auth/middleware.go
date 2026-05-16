package auth

import (
	"net/http"
	"strings"
)

// ClientAuth returns a middleware that enforces a bearer-token API key on
// all requests. If apiKey is empty the middleware is a no-op (auth disabled).
func ClientAuth(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if apiKey == "" {
			// Auth disabled — pass through.
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == "" || token == authHeader {
				writeJSONError(w, http.StatusUnauthorized, "missing Authorization header")
				return
			}
			if token != apiKey {
				writeJSONError(w, http.StatusUnauthorized, "invalid API key")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// InjectMasterToken attaches a freshly generated X-Master-Token to the
// outgoing request. Call this just before forwarding replication writes to
// slave nodes.
func InjectMasterToken(r *http.Request, signer *Signer) {
	r.Header.Set(MasterTokenHeader, signer.Generate())
}

// writeJSONError writes a minimal JSON error body and sets Content-Type.
func writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	//nolint:errcheck
	w.Write([]byte(`{"error":"` + msg + `"}`))
}