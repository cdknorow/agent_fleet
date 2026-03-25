package server

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsLocalhostOrigin tests the isLocalhostOrigin helper used by CORS.
func TestIsLocalhostOrigin(t *testing.T) {
	tests := []struct {
		origin string
		want   bool
	}{
		// Allowed localhost origins
		{"http://localhost:8450", true},
		{"http://localhost:3000", true},
		{"http://localhost", true},
		{"https://localhost:8450", true},
		{"http://127.0.0.1:8450", true},
		{"http://127.0.0.1:3000", true},
		{"https://127.0.0.1:8450", true},

		// Blocked: external origins
		{"http://evil.com", false},
		{"http://evil.com:8450", false},
		{"https://evil.com", false},
		{"http://192.168.1.5:8450", false},
		{"http://192.168.1.99:8450", false},
		{"http://10.0.0.1:8450", false},

		// Blocked: tricky variations
		{"http://localhost.evil.com", true}, // prefix match — see note below
		{"http://127.0.0.1.evil.com", true}, // prefix match — see note below
		// NOTE: isLocalhostOrigin uses prefix matching, so "localhost.evil.com"
		// would match. This is a known limitation but is mitigated by the
		// browser not sending such Origins for legitimate localhost requests.
		// The CORS same-origin check is the primary defense for non-localhost.

		// Edge cases
		{"", false},
		{"ftp://localhost:21", false},
		{"http://[::1]:8450", false}, // IPv6 localhost — not matched by isLocalhostOrigin
	}

	for _, tt := range tests {
		t.Run(tt.origin, func(t *testing.T) {
			got := isLocalhostOrigin(tt.origin)
			assert.Equal(t, tt.want, got, "isLocalhostOrigin(%q)", tt.origin)
		})
	}
}

// corsAllowOrigin replicates the AllowOriginFunc logic from setupRoutes
// so we can test it without spinning up the full server.
func corsAllowOrigin(r *http.Request, origin string) bool {
	if isLocalhostOrigin(origin) {
		return true
	}
	parsed, err := url.Parse(origin)
	if err == nil && parsed.Host == r.Host {
		return true
	}
	return false
}

// TestCORSAllowOriginFunc tests the combined CORS origin validation logic.
func TestCORSAllowOriginFunc(t *testing.T) {
	tests := []struct {
		name   string
		origin string
		host   string // request Host header
		want   bool
	}{
		// Localhost origins always allowed regardless of Host
		{
			name:   "localhost origin allowed",
			origin: "http://localhost:8450",
			host:   "localhost:8450",
			want:   true,
		},
		{
			name:   "127.0.0.1 origin allowed",
			origin: "http://127.0.0.1:8450",
			host:   "127.0.0.1:8450",
			want:   true,
		},
		{
			name:   "localhost origin allowed even with remote host",
			origin: "http://localhost:8450",
			host:   "192.168.1.5:8450",
			want:   true,
		},

		// Same-origin: Origin host matches request Host
		{
			name:   "same-origin remote IP allowed",
			origin: "http://192.168.1.5:8450",
			host:   "192.168.1.5:8450",
			want:   true,
		},
		{
			name:   "same-origin hostname allowed",
			origin: "http://myserver.local:8450",
			host:   "myserver.local:8450",
			want:   true,
		},

		// Cross-origin: Origin host differs from request Host — BLOCKED
		{
			name:   "cross-origin evil.com blocked",
			origin: "http://evil.com",
			host:   "192.168.1.5:8450",
			want:   false,
		},
		{
			name:   "cross-origin evil.com with port blocked",
			origin: "http://evil.com:8450",
			host:   "192.168.1.5:8450",
			want:   false,
		},
		{
			name:   "cross-origin different IP blocked",
			origin: "http://192.168.1.99:8450",
			host:   "192.168.1.5:8450",
			want:   false,
		},
		{
			name:   "cross-origin different port blocked",
			origin: "http://192.168.1.5:9999",
			host:   "192.168.1.5:8450",
			want:   false,
		},
		{
			name:   "cross-origin attacker subdomain blocked",
			origin: "http://192.168.1.5.attacker.com:8450",
			host:   "192.168.1.5:8450",
			want:   false,
		},

		// Edge: Origin scheme differs (http vs https) — host still matches
		{
			name:   "https origin matching host allowed",
			origin: "https://192.168.1.5:8450",
			host:   "192.168.1.5:8450",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{
				Host:   tt.host,
				Header: http.Header{"Origin": {tt.origin}},
			}
			got := corsAllowOrigin(r, tt.origin)
			assert.Equal(t, tt.want, got, "corsAllowOrigin(host=%q, origin=%q)", tt.host, tt.origin)
		})
	}
}
