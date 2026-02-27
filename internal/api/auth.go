package api

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
)

// BearerAuth returns middleware that validates a Bearer token on every request.
// When token is empty, all requests are allowed (auth disabled).
func BearerAuth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Auth disabled when no token configured.
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}

			auth := r.Header.Get("Authorization")
			if auth == "" {
				slog.Warn("auth: missing Authorization header", "path", r.URL.Path)
				writeError(w, http.StatusUnauthorized, "unauthorized", "Authorization header required")
				return
			}

			const prefix = "Bearer "
			if len(auth) < len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
				writeError(w, http.StatusUnauthorized, "unauthorized", "Bearer token required")
				return
			}

			provided := auth[len(prefix):]
			if provided == "" {
				writeError(w, http.StatusUnauthorized, "unauthorized", "Bearer token cannot be empty")
				return
			}

			if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
				slog.Warn("auth: invalid token", "path", r.URL.Path)
				writeError(w, http.StatusForbidden, "forbidden", "Invalid token")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
