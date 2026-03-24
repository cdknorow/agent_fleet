# Authentication — API Key + Optional PIN

## Overview

Two-layer auth for Coral: auto-generated API key for seamless access, optional PIN for extra security. Localhost connections bypass auth entirely.

## Design Principles

1. **Zero friction on localhost** — no login, no token, no config. Works like today.
2. **One-scan mobile setup** — QR code contains URL + API key. Scan → connected.
3. **No passwords to remember** — API key is auto-generated and persistent.
4. **Optional PIN for shared networks** — user enables if they want a human verification layer.

## Auth Flow

```
Request comes in
  │
  ├── From 127.0.0.1 / ::1 → ALLOW (no auth)
  │
  ├── Has valid API key (header or cookie) → ALLOW
  │
  ├── Has valid session cookie (from PIN login) → ALLOW
  │
  └── No auth → REDIRECT to /auth page
                  │
                  ├── PIN enabled → show PIN input
                  │     ├── Correct → set session cookie → redirect to /
                  │     └── Wrong → error, rate limit
                  │
                  └── PIN disabled → show API key input
                        ├── Correct → set session cookie → redirect to /
                        └── Wrong → error
```

## API Key

### Generation
- Auto-generated on first server start (32-char random hex)
- Stored in `~/.coral/api_key`
- Persists across restarts
- Can be regenerated via `coral --regenerate-key` or settings UI

### Usage
Clients send the key in one of three ways:
1. `Authorization: Bearer <key>` header
2. `?api_key=<key>` query parameter (for WebSocket connections)
3. `coral_session` cookie (set after successful auth via /auth page)

### Where the key appears
- Server startup log: `API Key: abcd1234...` (truncated)
- Settings modal: full key with copy button
- Mobile QR code: encodes `http://<ip>:<port>?api_key=<key>`
- `~/.coral/api_key` file

## Optional PIN

### Setup
- User sets a 4-6 digit PIN in Settings → Security
- Stored as bcrypt hash in `~/.coral/pin_hash`
- Can be disabled (delete the hash file or toggle in settings)

### Login
- When PIN is enabled and request has no valid session, show PIN input page
- 3 attempts before 30-second lockout (rate limiting)
- Correct PIN sets a session cookie (24-hour expiry, configurable)
- PIN is NOT an alternative to the API key — it's an additional layer for browser sessions

## Session Cookie

### Format
- Name: `coral_session`
- Value: random 32-char token
- HttpOnly, Secure (if HTTPS), SameSite=Lax
- Expiry: 24 hours (configurable)

### Storage
- Active sessions stored in memory (map[token]sessionInfo)
- sessionInfo: created_at, client_ip, user_agent
- Sessions cleared on server restart (re-auth required)
- Max 100 concurrent sessions (LRU eviction)

## Middleware

```go
func AuthMiddleware(cfg *config.Config, keyStore *auth.KeyStore) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // 1. Localhost bypass
            if isLocalhost(r) {
                next.ServeHTTP(w, r)
                return
            }

            // 2. API key (header or query param)
            if key := extractAPIKey(r); key != "" && keyStore.ValidateKey(key) {
                next.ServeHTTP(w, r)
                return
            }

            // 3. Session cookie
            if token := extractSessionCookie(r); token != "" && keyStore.ValidateSession(token) {
                next.ServeHTTP(w, r)
                return
            }

            // 4. Redirect to auth page
            if r.URL.Path == "/auth" || strings.HasPrefix(r.URL.Path, "/static/") {
                next.ServeHTTP(w, r) // allow auth page + static assets
                return
            }

            http.Redirect(w, r, "/auth", http.StatusTemporaryRedirect)
        })
    }
}
```

## WebSocket Auth

WebSockets can't send custom headers after upgrade. Options:
1. **Query parameter**: `ws://<host>/ws/terminal?api_key=<key>` — checked during upgrade
2. **Cookie**: if browser has a valid `coral_session` cookie, it's sent automatically with the WebSocket handshake
3. **First message**: client sends `{"type": "auth", "key": "<key>"}` as first WebSocket message

Recommendation: Cookie (automatic for browser) + query param (for programmatic access).

## QR Code Enhancement

Current QR encodes: `http://<ip>:<port>`

New QR encodes: `http://<ip>:<port>?api_key=<key>`

The mobile PWA:
1. Scans QR → opens URL with key in query param
2. Server validates key, sets session cookie
3. Cookie persists → subsequent visits don't need the key
4. PWA "Add to Home Screen" saves the authenticated session

## Endpoints

```
GET  /auth                     — auth page (PIN input or API key input)
POST /auth/pin                 — validate PIN, set session cookie
POST /auth/key                 — validate API key, set session cookie
GET  /api/system/auth-status   — returns { authenticated: bool, method: "localhost"|"key"|"session" }
GET  /api/system/api-key       — returns the API key (localhost only)
POST /api/system/api-key/regenerate — generate new key (localhost only)
GET  /api/settings             — includes pin_enabled: bool
PUT  /api/settings             — set/clear PIN via pin_code key
```

## File Layout

```
~/.coral/
  api_key          — plain text API key (32-char hex)
  pin_hash         — bcrypt hash of PIN (only if PIN enabled)
  sessions.db      — existing DB (no auth tables needed — sessions are in-memory)
```

## Implementation Plan

### Phase 1: API Key (core auth)
1. Generate/load API key on startup (`~/.coral/api_key`)
2. Auth middleware: localhost bypass + key validation
3. `/auth` page: simple key input form
4. Session cookie on successful auth
5. QR code includes key
6. WebSocket auth via cookie + query param

### Phase 2: Optional PIN
1. PIN setup in Settings UI
2. `/auth/pin` endpoint
3. Rate limiting (3 attempts / 30s lockout)
4. PIN login page

### Phase 3: Polish
1. Settings UI: show/regenerate API key, enable/disable PIN
2. Active sessions list in settings
3. Logout endpoint (clear session cookie)
4. Configurable session expiry

## Security Considerations

- API key is equivalent to full access — protect `~/.coral/api_key` with file permissions (0600)
- PIN is weak auth (4-6 digits) — rate limiting is essential
- Session cookies are HttpOnly (no JS access) and SameSite=Lax (CSRF protection)
- Localhost bypass means anyone with local access has full control (acceptable for a dev tool)
- No HTTPS by default — API key transmitted in plaintext on LAN. For untrusted networks, use a reverse proxy with TLS or Tailscale
- WebSocket auth via query param exposes key in server logs — use cookie when possible
