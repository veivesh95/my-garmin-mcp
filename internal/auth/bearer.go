// Package auth protects the HTTP endpoint.
package auth

import (
	"crypto/subtle"
	"net/http"
)

// RequireBearer rejects requests whose Authorization header is not
// exactly "Bearer <token>". The comparison is constant-time.
func RequireBearer(token string, next http.Handler) http.Handler {
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare(got, want) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="garmin-mcp"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
