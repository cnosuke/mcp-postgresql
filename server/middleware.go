package server

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

// withOriginValidation wraps an http.Handler with Origin header validation.
// Per MCP spec 2025-11-25, servers MUST validate the Origin header and return
// 403 Forbidden if it does not match the allowed origins.
// If allowedOrigins is empty, all origins are permitted (development mode).
// If the Origin header is absent, the request is allowed (non-browser clients).
func withOriginValidation(next http.Handler, allowedOrigins []string) http.Handler {
	if len(allowedOrigins) == 0 {
		return next
	}

	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[strings.TrimRight(o, "/")] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		normalized := strings.TrimRight(origin, "/")
		if _, ok := allowed[normalized]; !ok {
			zap.S().Warnw("rejected request with disallowed origin",
				"origin", origin,
			)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// withAuthMiddleware wraps an http.Handler with Bearer token authentication.
// If authToken is empty, authentication is disabled and the handler is returned as-is.
// Token comparison uses crypto/subtle.ConstantTimeCompare to prevent timing attacks.
func withAuthMiddleware(next http.Handler, authToken string) http.Handler {
	if authToken == "" {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if subtle.ConstantTimeCompare([]byte(token), []byte(authToken)) != 1 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// extractBearerToken extracts the Bearer token from the Authorization header.
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}

	const prefix = "Bearer "
	if len(auth) < len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
		return ""
	}

	return auth[len(prefix):]
}
