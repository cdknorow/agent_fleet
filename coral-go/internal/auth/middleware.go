package auth

import (
	"log"
	"net/http"
	"strings"
)

// Middleware returns an HTTP middleware that enforces API key authentication.
// Localhost requests bypass auth entirely. Static assets and the /auth page
// are accessible without authentication.
func Middleware(ks *KeyStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Always allow localhost (zero friction for local development)
			if IsLocalhost(r) {
				next.ServeHTTP(w, r)
				return
			}

			// Allow auth page, static assets, and manifest
			path := r.URL.Path
			if path == "/auth" || strings.HasPrefix(path, "/static/") ||
				path == "/manifest.json" || path == "/favicon.ico" {
				next.ServeHTTP(w, r)
				return
			}

			// Check API key (header or query param)
			if key := ExtractAPIKey(r); key != "" {
				if !ks.CheckRateLimit(clientIP(r)) {
					http.Error(w, "Too many authentication attempts", http.StatusTooManyRequests)
					return
				}
				if ks.ValidateKey(key) {
					// Auto-create session for API key auth via query param
					if r.URL.Query().Get("api_key") != "" {
						token := ks.CreateSession(clientIP(r), r.UserAgent())
						SetSessionCookie(w, token)
					}
					next.ServeHTTP(w, r)
					return
				}
				log.Printf("[auth] invalid API key from %s", clientIP(r))
			}

			// Check session cookie
			if token := ExtractSessionCookie(r); token != "" {
				if ks.ValidateSession(token) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// No valid auth — redirect browser requests, reject API requests
			if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/ws/") {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/auth", http.StatusTemporaryRedirect)
		})
	}
}

func clientIP(r *http.Request) string {
	host, _, _ := strings.Cut(r.RemoteAddr, ":")
	return host
}
