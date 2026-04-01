# Connected Apps

## Overview

Connected Apps is a generic OAuth2 credential store that lets Coral connect to external services (Google, GitHub, Slack, etc.). Credentials are stored securely and can be referenced by workflow steps and agents.

## Problem

Workflows that interact with external services (e.g., "fetch my recent emails", "post to Slack") need API credentials. Currently there's no way to store or manage OAuth tokens in Coral. Users must manage credentials manually.

## Design Principles

1. **Provider-agnostic** — Any OAuth2 service can be added as a provider.
2. **Secure storage** — Tokens encrypted at rest in SQLite.
3. **Auto-refresh** — Access tokens are refreshed transparently using stored refresh tokens.
4. **Workflow integration** — Steps can reference connections by name; the runner injects a fresh access token as an env var.
5. **Simple UI** — A "Connected Apps" settings page to connect, view, and revoke services.

## Provider Registry

Providers are defined in code with their OAuth2 configuration:

```go
type OAuthProvider struct {
    ID           string   // "google", "github", "slack"
    Name         string   // "Google", "GitHub", "Slack"
    AuthURL      string   // Authorization endpoint
    TokenURL     string   // Token endpoint
    Scopes       []string // Default scopes
    Icon         string   // Material icon name or URL
    Instructions string   // Help text for setup
}
```

### Built-in Providers

| Provider | Scopes | Use Cases |
|----------|--------|-----------|
| Google | `gmail.readonly`, `calendar.readonly`, `drive.readonly` | Email summaries, calendar checks, doc access |
| GitHub | `repo`, `read:org` | PR reviews, issue triage |
| Slack | `channels:read`, `chat:write` | Notifications, summaries |

Coral ships with **embedded OAuth client credentials** for built-in providers. Users just click "Connect" — no need to create their own OAuth apps. For custom/self-hosted providers, users can supply their own client ID + secret.

### Embedded vs Custom Credentials

| Mode | Use Case | UI |
|------|----------|-----|
| **Embedded** (default) | Built-in providers (Google, GitHub, Slack) | One-click "Connect" button → consent screen |
| **Custom** | Self-hosted or enterprise OAuth apps | "Advanced" toggle reveals client ID/secret fields |

Embedded credentials are compiled into the binary. The client secret for installed/desktop apps is not truly secret (per Google's documentation) — the security model relies on the redirect URI and user consent, not secret confidentiality.

## Data Model

### SQLite Schema

```sql
CREATE TABLE IF NOT EXISTS connected_apps (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    provider_id     TEXT NOT NULL,
    name            TEXT NOT NULL,
    client_id       TEXT NOT NULL,
    client_secret   TEXT NOT NULL,
    scopes          TEXT NOT NULL DEFAULT '',
    access_token    TEXT NOT NULL DEFAULT '',
    refresh_token   TEXT NOT NULL DEFAULT '',
    token_expiry    TEXT,
    account_email   TEXT DEFAULT '',
    account_name    TEXT DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'active',
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    UNIQUE(provider_id, name)
);
```

**Notes:**
- `client_id` and `client_secret` are stored per-connection (users bring their own OAuth app).
- `access_token` and `refresh_token` should be encrypted. For v1, we can use a simple AES-256-GCM key derived from a machine-specific identifier. Phase 2 can add a user-supplied encryption key.
- `name` is a user-friendly label (e.g., "Work Gmail", "Personal GitHub").
- `status`: `active`, `expired`, `revoked`.

## OAuth Flow

### Default Flow (Embedded Credentials)

1. User clicks "Connect" on a provider card (e.g., Google)
2. Optionally selects scopes (defaults pre-checked) and enters a connection name
3. Coral uses embedded client_id/secret to build the auth URL
4. Browser opens provider consent screen
5. User authorizes → provider redirects to `http://localhost:{port}/api/connected-apps/callback`
6. Coral exchanges code for tokens, fetches user profile, stores everything
7. Connection appears in the list — done

### Advanced Flow (Custom Credentials)

For enterprise/self-hosted OAuth apps, users toggle "Use custom credentials" to reveal client ID/secret fields. Same flow after that.

### Token Refresh

When a workflow step requests a connection's token:
1. Check if `token_expiry` is in the future (with 5-minute buffer)
2. If expired, use `refresh_token` to get a new `access_token`
3. Update DB with new token + expiry
4. Return the fresh access token

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/connected-apps` | List all connections (tokens redacted) |
| `GET` | `/api/connected-apps/providers` | List available providers with their config |
| `POST` | `/api/connected-apps/auth/start` | Start OAuth flow (returns auth URL to open in browser) |
| `GET` | `/api/connected-apps/callback` | OAuth callback handler (internal, browser redirects here) |
| `GET` | `/api/connected-apps/{id}` | Get connection details |
| `GET` | `/api/connected-apps/{id}/token` | Get a fresh access token (auto-refreshes if needed) |
| `DELETE` | `/api/connected-apps/{id}` | Revoke and delete a connection |
| `POST` | `/api/connected-apps/{id}/test` | Test the connection (e.g., fetch user profile) |

### Start Auth Flow

```json
POST /api/connected-apps/auth/start
{
    "provider_id": "google",
    "name": "Work Gmail",
    "scopes": ["gmail.readonly", "calendar.readonly"]
}
```

If `client_id` and `client_secret` are omitted, the server uses the embedded credentials for the provider. To use custom credentials:

```json
POST /api/connected-apps/auth/start
{
    "provider_id": "google",
    "name": "Work Gmail",
    "client_id": "xxxx.apps.googleusercontent.com",
    "client_secret": "GOCSPX-xxxx",
    "scopes": ["gmail.readonly", "calendar.readonly"]
}
```

Response:
```json
{
    "auth_url": "https://accounts.google.com/o/oauth2/v2/auth?client_id=...&scope=...&state=...",
    "state": "random-state-token"
}
```

The UI opens this URL in a new browser tab. After the user authorizes, the provider redirects to the callback URL, Coral exchanges the code, and the connection appears in the list.

### List Connections

```json
GET /api/connected-apps

{
    "connections": [
        {
            "id": 1,
            "provider_id": "google",
            "name": "Work Gmail",
            "scopes": ["gmail.readonly"],
            "account_email": "user@example.com",
            "account_name": "Chris K",
            "status": "active",
            "created_at": "2026-03-31T07:00:00Z"
        }
    ]
}
```

Tokens are never returned in list/get responses. Only the `/token` endpoint returns the access token.

## Workflow Integration

### Referencing connections in workflow steps

Workflows can reference connected apps by name. The runner resolves the connection, auto-refreshes the token, and injects it as an environment variable:

```json
{
    "name": "fetch-emails",
    "type": "shell",
    "command": "python3 scripts/fetch_gmail.py --token $CORAL_TOKEN_WORK_GMAIL",
    "connections": ["Work Gmail"]
}
```

The runner:
1. Looks up "Work Gmail" in connected_apps
2. Calls the token endpoint to get a fresh access token
3. Sets `CORAL_TOKEN_WORK_GMAIL` (name uppercased, spaces → underscores, prefixed with `CORAL_TOKEN_`)
4. Runs the command

For agent steps, the token is included in the prompt context or available via env var.

## UI: Connected Apps Page

A new page/tab in settings:

### Provider Cards
Show available providers as cards with:
- Provider icon + name
- "Connect" button
- List of existing connections for this provider

### Connection Form
When clicking "Connect":
1. Provider name + description
2. Client ID input
3. Client Secret input
4. Scope checkboxes (provider defaults pre-checked)
5. Connection name input (e.g., "Work Gmail")
6. "Authorize" button → opens browser for OAuth

### Connection List
Each connected account shows:
- Provider icon
- Connection name
- Account email/name (fetched during auth)
- Status badge (active/expired/revoked)
- Last used timestamp
- "Test" button
- "Disconnect" button (with confirmation)

## Implementation Plan

### Phase 1: Core

| Component | File | Description |
|-----------|------|-------------|
| Schema | `internal/store/connection.go` | Add `connected_apps` table |
| Store | `internal/store/connected_apps.go` | CRUD + token refresh |
| Providers | `internal/oauth/providers.go` | Provider registry (Google, GitHub, Slack) |
| OAuth | `internal/oauth/flow.go` | OAuth2 flow handler (auth URL, callback, token exchange) |
| API | `internal/server/routes/connected_apps.go` | HTTP handlers |
| Routes | `internal/server/server.go` | Register routes |

### Phase 2: UI + Workflow Integration

| Component | File | Description |
|-----------|------|-------------|
| UI | `templates/includes/views/connected_apps.html` | Settings page |
| JS | `static/connected_apps.js` | Frontend logic |
| CSS | `static/css/connected_apps.css` | Styling |
| Runner | `internal/background/workflow_runner.go` | Token injection for steps |

### Phase 3: Polish

- Token encryption at rest (AES-256-GCM)
- Connection health monitoring (periodic token validation)
- Usage audit log
- More providers (Jira, Linear, Notion, etc.)

## Example: Gmail Workflow

Once Google is connected as "Work Gmail":

```json
{
    "name": "email-summary",
    "description": "Fetch recent emails and summarize with Haiku",
    "repo_path": "/Users/dev/myproject",
    "steps": [
        {
            "name": "fetch-emails",
            "type": "shell",
            "command": "python3 .coral/scripts/fetch_gmail.py --count 5 --token $CORAL_TOKEN_WORK_GMAIL --output {{step_dir}}/artifacts/emails.json",
            "connections": ["Work Gmail"]
        },
        {
            "name": "summarize",
            "type": "agent",
            "prompt": "Read the emails at {{step_0_stdout}} and for each email: (1) write a 1-sentence summary, (2) suggest a brief response if action is needed. Write the output to {{step_dir}}/artifacts/summary.md",
            "agent": {
                "agent_type": "claude",
                "model": "claude-haiku-4-5-20251001"
            }
        }
    ]
}
```
