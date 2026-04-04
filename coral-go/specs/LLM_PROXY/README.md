# LLM Proxy

## Problem

Coral launches AI agents but has no visibility or control over their LLM API calls. We can't:
- Track real-time cost per request (only cumulative snapshots from Claude hooks)
- Enforce spend limits before money is spent
- Route requests intelligently across providers/models
- Detect runaway agents or cost spikes
- Cache duplicate requests
- Rate limit to stay within provider quotas

## Solution

Build a native HTTP proxy into Coral that intercepts all LLM API calls. Agents are automatically configured to route through the proxy via `ANTHROPIC_BASE_URL` / `OPENAI_BASE_URL` environment variables injected at launch time.

Inspired by [RelayPlane/proxy](https://github.com/RelayPlane/proxy) but implemented natively in Go as part of Coral's server.

## Architecture

```
Agent (Claude/Gemini/Codex)
  │
  │ ANTHROPIC_BASE_URL=http://127.0.0.1:{coral-port}/proxy
  │
  ▼
Coral Proxy Layer (/proxy/v1/messages, /proxy/v1/chat/completions)
  │
  ├─ Extract metadata (session_id, agent_name, model)
  ├─ Check budget → block/downgrade if over limit
  ├─ Check rate limits → queue if throttled
  ├─ Check cache → return cached response if hit
  ├─ Route to provider (or auto-route by complexity)
  │
  ▼
LLM Provider (Anthropic, OpenAI, Google, etc.)
  │
  ▼
Coral Proxy Layer (response path)
  │
  ├─ Count tokens (input + output)
  ├─ Calculate cost (provider-aware pricing)
  ├─ Record to proxy_requests table
  ├─ Update session/agent/team aggregates
  ├─ Cache response (if deterministic)
  ├─ Check anomaly detectors
  │
  ▼
Agent receives response
```

## Phases

### Phase 1: Proxy + Cost Tracking (ship first)
- Proxy endpoints (Anthropic + OpenAI format)
- Streaming SSE passthrough
- Per-request cost recording
- Agent wiring (auto-inject base URL at launch)
- Dashboard API for cost data

### Phase 2: Budget + Rate Limiting
- Budget enforcement (daily/hourly/per-request/per-session)
- Budget actions (block, warn, downgrade)
- Token bucket rate limiter per provider/model
- Rate limit queuing

### Phase 3: Smart Routing + Caching
- Auto-routing by request complexity
- Cross-provider cascade on failures
- Response caching (exact + aggressive modes)
- Anomaly detection (velocity spikes, cost acceleration, loops)

---

## Phase 1 Spec

### 1. Proxy Endpoints

Mount under `/proxy` to avoid collision with existing `/api` routes.

#### POST /proxy/v1/messages (Anthropic format)

Forwards to `https://api.anthropic.com/v1/messages`.

**Request passthrough:** The proxy forwards the request body as-is to the upstream provider. It does NOT parse or validate the LLM request schema — it is a transparent passthrough. The only things the proxy inspects are:
- `model` field (for routing and cost calculation)
- `stream` field (to determine SSE vs JSON response handling)
- `max_tokens` field (for cost estimation)

**Headers forwarded upstream:**
- `x-api-key` — from agent's env or Coral's configured API key
- `anthropic-version` — passthrough from agent
- `content-type: application/json`

**Headers injected by proxy (response to agent):**
- `X-Coral-Proxy: true`
- `X-Coral-Request-Id: {uuid}`
- `X-Coral-Session-Id: {session_id}`
- `X-Coral-Cost-Usd: {cost}` (after response completes)

**Streaming:** If `stream: true`, proxy streams SSE events back to the agent as they arrive from upstream. The proxy reads each SSE chunk to extract token usage from the final `message_stop` event.

#### POST /proxy/v1/chat/completions (OpenAI format)

Forwards to `https://api.openai.com/v1/chat/completions` (or configured provider).

Same passthrough behavior as above. Extracts `usage.prompt_tokens` and `usage.completion_tokens` from the response.

**Headers forwarded upstream:**
- `Authorization: Bearer {api_key}`
- `content-type: application/json`

#### GET /proxy/health

Returns `{"status": "ok", "uptime_seconds": N}`.

### 2. Agent Identification

The proxy must know which agent made each request. Two mechanisms:

**A. Header injection (preferred):**
Agents set `X-Coral-Session-Id` header. Coral's agent launch already injects `CORAL_SESSION_NAME` as an env var — we configure agents to include this as a header.

**B. Port-based identification (fallback):**
If per-agent headers aren't feasible for all agent types, assign each agent a unique proxy port. Map port → session_id in the proxy.

**Recommendation:** Start with a single proxy port + header-based identification. The `ANTHROPIC_BASE_URL` can include a session token in the path as fallback: `http://127.0.0.1:{port}/proxy/{session_id}/v1/messages`.

### 3. Token Counting & Cost Calculation

#### Token Extraction

**Anthropic (non-streaming):** Response JSON includes:
```json
{
  "usage": {
    "input_tokens": 1234,
    "output_tokens": 567,
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 100
  }
}
```

**Anthropic (streaming):** Final SSE event `message_stop` includes usage in the accumulated message. Parse the `message_delta` event which contains:
```json
{
  "type": "message_delta",
  "usage": {"output_tokens": 567}
}
```
Input tokens come from the `message_start` event.

**OpenAI (non-streaming):** Response JSON includes:
```json
{
  "usage": {
    "prompt_tokens": 1234,
    "completion_tokens": 567
  }
}
```

**OpenAI (streaming):** Final chunk with `usage` field (if `stream_options.include_usage: true`), otherwise estimate from chunk count.

#### Pricing Table

Stored in Go as a map, updatable via settings. Initial values:

```go
var Pricing = map[string]ModelPricing{
    // Anthropic
    "claude-opus-4-20250514":        {InputPerMTok: 15.00, OutputPerMTok: 75.00, CacheReadPerMTok: 1.50, CacheWritePerMTok: 18.75},
    "claude-sonnet-4-20250514":      {InputPerMTok: 3.00,  OutputPerMTok: 15.00, CacheReadPerMTok: 0.30, CacheWritePerMTok: 3.75},
    "claude-haiku-4-20250514":       {InputPerMTok: 0.80,  OutputPerMTok: 4.00,  CacheReadPerMTok: 0.08, CacheWritePerMTok: 1.00},

    // OpenAI
    "gpt-4o":                        {InputPerMTok: 2.50,  OutputPerMTok: 10.00},
    "gpt-4o-mini":                   {InputPerMTok: 0.15,  OutputPerMTok: 0.60},
    "o3":                            {InputPerMTok: 2.00,  OutputPerMTok: 8.00},

    // Google
    "gemini-2.5-pro":                {InputPerMTok: 1.25,  OutputPerMTok: 10.00},
    "gemini-2.5-flash":              {InputPerMTok: 0.15,  OutputPerMTok: 0.60},
}
```

```go
type ModelPricing struct {
    InputPerMTok      float64 // $ per 1M input tokens
    OutputPerMTok     float64 // $ per 1M output tokens
    CacheReadPerMTok  float64 // $ per 1M cache-read tokens (Anthropic)
    CacheWritePerMTok float64 // $ per 1M cache-write tokens (Anthropic)
}
```

**Cost formula:**
```
cost = (input_tokens * InputPerMTok / 1_000_000)
     + (output_tokens * OutputPerMTok / 1_000_000)
     + (cache_read_tokens * CacheReadPerMTok / 1_000_000)
     + (cache_write_tokens * CacheWritePerMTok / 1_000_000)
```

### 4. Storage Schema

#### New table: proxy_requests

```sql
CREATE TABLE IF NOT EXISTS proxy_requests (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id      TEXT NOT NULL UNIQUE,       -- UUID for this request
    session_id      TEXT NOT NULL,              -- FK to live_sessions
    agent_name      TEXT,
    agent_type      TEXT,
    team_id         INTEGER,
    board_name      TEXT,

    -- Request metadata
    provider        TEXT NOT NULL,              -- "anthropic", "openai", "google"
    model_requested TEXT NOT NULL,              -- model the agent asked for
    model_used      TEXT NOT NULL,              -- model actually used (may differ if downgraded)
    is_streaming     INTEGER NOT NULL DEFAULT 0,

    -- Token usage
    input_tokens    INTEGER NOT NULL DEFAULT 0,
    output_tokens   INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens  INTEGER NOT NULL DEFAULT 0,
    cache_write_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens    INTEGER NOT NULL DEFAULT 0,

    -- Cost
    cost_usd        REAL NOT NULL DEFAULT 0,

    -- Timing
    started_at      TEXT NOT NULL,
    completed_at    TEXT,
    latency_ms      INTEGER,

    -- Status
    status          TEXT NOT NULL DEFAULT 'pending', -- pending, success, error, blocked
    error_message   TEXT,
    http_status     INTEGER,

    -- Cache
    cache_hit       INTEGER NOT NULL DEFAULT 0,

    FOREIGN KEY (team_id) REFERENCES teams(id)
);

CREATE INDEX idx_proxy_requests_session ON proxy_requests(session_id);
CREATE INDEX idx_proxy_requests_team ON proxy_requests(team_id);
CREATE INDEX idx_proxy_requests_started ON proxy_requests(started_at);
CREATE INDEX idx_proxy_requests_model ON proxy_requests(model_used);
```

#### New table: proxy_budgets (Phase 2, schema defined now)

```sql
CREATE TABLE IF NOT EXISTS proxy_budgets (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    scope           TEXT NOT NULL,              -- "global", "team", "agent", "session"
    scope_id        TEXT,                       -- team_id, session_id, or NULL for global
    period          TEXT NOT NULL,              -- "daily", "hourly", "per_request"
    limit_usd       REAL NOT NULL,
    action          TEXT NOT NULL DEFAULT 'block', -- "block", "warn", "downgrade"
    enabled         INTEGER NOT NULL DEFAULT 1,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);
```

### 5. Agent Wiring

Inject proxy URL into agent environment at launch time.

#### Changes to LaunchParams (agent.go)

```go
type LaunchParams struct {
    // ... existing fields ...
    ProxyBaseURL string // e.g., "http://127.0.0.1:8420/proxy/{session_id}"
}
```

#### Claude (claude.go — buildMergedSettings)

Add to the env map:
```go
if params.ProxyBaseURL != "" {
    envMap["ANTHROPIC_BASE_URL"] = params.ProxyBaseURL
}
```

#### Gemini (gemini.go)

Add export before gemini command:
```go
if params.ProxyBaseURL != "" {
    parts = append(parts, fmt.Sprintf(`export GEMINI_API_BASE="%s" &&`, params.ProxyBaseURL))
}
```

Note: Verify Gemini CLI respects this env var. If not, may need `--api-base` flag or similar.

#### Codex (codex.go)

Add export before codex command:
```go
if params.ProxyBaseURL != "" {
    parts = append(parts, fmt.Sprintf(`export OPENAI_BASE_URL="%s" &&`, params.ProxyBaseURL))
}
```

#### Session URL format

Each agent gets a unique proxy URL that embeds its session ID:
```
http://127.0.0.1:{port}/proxy/{session_id}/v1/messages
```

The proxy extracts `session_id` from the URL path, so no custom headers are needed from the agent.

### 6. Dashboard API

#### GET /api/proxy/stats

Aggregate cost statistics.

**Query params:**
- `period` — `hour`, `day`, `week`, `month` (default: `day`)
- `team_id` — filter by team
- `session_id` — filter by session

**Response:**
```json
{
    "period": "day",
    "total_requests": 1542,
    "total_cost_usd": 23.47,
    "total_input_tokens": 4521000,
    "total_output_tokens": 892000,
    "by_model": [
        {"model": "claude-sonnet-4-20250514", "requests": 1200, "cost_usd": 8.40},
        {"model": "claude-opus-4-20250514", "requests": 342, "cost_usd": 15.07}
    ],
    "by_agent": [
        {"session_id": "abc-123", "agent_name": "Lead Developer", "requests": 450, "cost_usd": 12.30},
        {"session_id": "def-456", "agent_name": "QA Engineer", "requests": 200, "cost_usd": 2.10}
    ]
}
```

#### GET /api/proxy/requests

Recent request log.

**Query params:**
- `session_id` — filter by session
- `team_id` — filter by team
- `limit` — max results (default: 50)
- `offset` — pagination offset

**Response:**
```json
{
    "requests": [
        {
            "request_id": "uuid",
            "session_id": "abc-123",
            "agent_name": "Lead Developer",
            "provider": "anthropic",
            "model_used": "claude-sonnet-4-20250514",
            "input_tokens": 3200,
            "output_tokens": 850,
            "cost_usd": 0.022,
            "latency_ms": 2340,
            "status": "success",
            "started_at": "2026-04-03T10:15:32Z"
        }
    ],
    "total": 1542
}
```

#### GET /api/proxy/requests/{request_id}

Single request detail.

#### GET /api/proxy/session/{session_id}/cost

Cost summary for a single session.

**Response:**
```json
{
    "session_id": "abc-123",
    "agent_name": "Lead Developer",
    "total_requests": 450,
    "total_cost_usd": 12.30,
    "total_input_tokens": 1250000,
    "total_output_tokens": 340000,
    "by_model": [
        {"model": "claude-sonnet-4-20250514", "requests": 400, "cost_usd": 4.80},
        {"model": "claude-opus-4-20250514", "requests": 50, "cost_usd": 7.50}
    ],
    "first_request_at": "2026-04-03T09:00:00Z",
    "last_request_at": "2026-04-03T10:15:32Z"
}
```

### 7. Configuration

Add to Coral settings (stored in settings table):

```json
{
    "proxy": {
        "enabled": true,
        "providers": {
            "anthropic": {
                "api_key": "sk-ant-...",
                "base_url": "https://api.anthropic.com"
            },
            "openai": {
                "api_key": "sk-...",
                "base_url": "https://api.openai.com"
            },
            "google": {
                "api_key": "...",
                "base_url": "https://generativelanguage.googleapis.com"
            }
        }
    }
}
```

API keys can also come from environment variables (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, etc.) which the proxy reads at startup. Settings override env vars.

### 8. Proxy Handler Implementation

New package: `coral-go/internal/proxy/`

```
internal/proxy/
    proxy.go          -- Core proxy handler (request forwarding, SSE streaming)
    cost.go           -- Pricing table, cost calculation
    middleware.go      -- Session extraction, request logging
    store.go          -- proxy_requests DB operations
    providers.go      -- Provider-specific URL resolution and auth
```

#### Core flow (proxy.go)

```go
func (p *Proxy) HandleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
    sessionID := chi.URLParam(r, "sessionID")

    // 1. Read request body (tee for forwarding)
    body, _ := io.ReadAll(r.Body)

    // 2. Extract model and stream flag
    var req struct {
        Model  string `json:"model"`
        Stream bool   `json:"stream"`
    }
    json.Unmarshal(body, &req)

    // 3. Create proxy_request record (status: pending)
    reqID := uuid.New().String()
    p.store.CreateRequest(reqID, sessionID, "anthropic", req.Model)

    // 4. Build upstream request
    upstreamURL := p.anthropicBaseURL + "/v1/messages"
    upstreamReq, _ := http.NewRequest("POST", upstreamURL, bytes.NewReader(body))
    upstreamReq.Header.Set("x-api-key", p.anthropicKey)
    upstreamReq.Header.Set("anthropic-version", r.Header.Get("anthropic-version"))
    upstreamReq.Header.Set("content-type", "application/json")

    // 5. Forward and handle response
    if req.Stream {
        p.handleSSEStream(w, upstreamReq, reqID, sessionID, req.Model)
    } else {
        p.handleJSONResponse(w, upstreamReq, reqID, sessionID, req.Model)
    }
}
```

#### SSE Streaming (proxy.go)

```go
func (p *Proxy) handleSSEStream(w http.ResponseWriter, req *http.Request, reqID, sessionID, model string) {
    resp, _ := p.httpClient.Do(req)
    defer resp.Body.Close()

    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("X-Coral-Proxy", "true")
    w.Header().Set("X-Coral-Request-Id", reqID)
    w.WriteHeader(resp.StatusCode)

    flusher := w.(http.Flusher)
    scanner := bufio.NewScanner(resp.Body)

    var inputTokens, outputTokens int

    for scanner.Scan() {
        line := scanner.Text()

        // Parse SSE data lines for token usage
        if strings.HasPrefix(line, "data: ") {
            data := line[6:]
            // Extract tokens from message_start (input) and message_delta (output)
            inputTokens, outputTokens = extractTokensFromSSE(data, inputTokens, outputTokens)
        }

        // Forward line to agent as-is
        fmt.Fprintf(w, "%s\n", line)
        flusher.Flush()
    }

    // Record final usage
    cost := calculateCost(model, inputTokens, outputTokens, 0, 0)
    p.store.CompleteRequest(reqID, inputTokens, outputTokens, cost, "success")
}
```

### 9. Route Registration (server.go)

```go
// In server.go route setup:
if proxyEnabled {
    proxy := proxy.New(store, settings)

    // Session-scoped proxy routes
    r.Route("/proxy/{sessionID}", func(r chi.Router) {
        r.Post("/v1/messages", proxy.HandleAnthropicMessages)
        r.Post("/v1/chat/completions", proxy.HandleOpenAIChatCompletions)
    })

    // Proxy health
    r.Get("/proxy/health", proxy.Health)
}

// Dashboard API (always available, reads proxy_requests table)
r.Route("/api/proxy", func(r chi.Router) {
    r.Get("/stats", proxyHandler.Stats)
    r.Get("/requests", proxyHandler.ListRequests)
    r.Get("/requests/{requestID}", proxyHandler.GetRequest)
    r.Get("/session/{sessionID}/cost", proxyHandler.SessionCost)
})
```

---

## Phase 2 Spec (Budget + Rate Limiting)

### Budget Enforcement

Checked pre-request in the proxy middleware.

**Scopes:** global, per-team, per-agent, per-session
**Periods:** daily, hourly, per-request
**Actions:**
- `block` — reject request with 429 and error message
- `warn` — allow but log warning, send WebSocket event to dashboard
- `downgrade` — swap model to cheaper tier (opus→sonnet, sonnet→haiku)

**Downgrade mapping:**
```go
var DowngradeMap = map[string]string{
    "claude-opus-4-20250514":   "claude-sonnet-4-20250514",
    "claude-sonnet-4-20250514": "claude-haiku-4-20250514",
    "gpt-4o":                   "gpt-4o-mini",
    "gemini-2.5-pro":           "gemini-2.5-flash",
}
```

### Rate Limiter

Token bucket algorithm per provider and per model.

```go
type RateLimiter struct {
    buckets map[string]*Bucket // key: "provider:model" or "provider"
}

type Bucket struct {
    Rate       float64       // requests per second
    Capacity   int           // max burst
    Tokens     float64
    LastRefill time.Time
    Queue      chan struct{} // waiting requests
}
```

**Default limits (requests per minute):**
- claude-opus: 30 RPM
- claude-sonnet: 60 RPM
- claude-haiku: 100 RPM
- gpt-4o: 60 RPM

When a request would exceed the limit, it's queued (up to 50 depth, 30s timeout) rather than immediately rejected.

### Budget API

```
GET    /api/proxy/budgets                — list all budgets
POST   /api/proxy/budgets                — create budget rule
PUT    /api/proxy/budgets/{id}           — update budget
DELETE /api/proxy/budgets/{id}           — delete budget
GET    /api/proxy/budgets/{id}/usage     — current spend vs limit
```

---

## Phase 3 Spec (Smart Routing + Caching)

### Auto-Routing

Classify the final user message by complexity and route to appropriate model.

**Classification heuristic (simple, no ML):**
- **Simple** (→ haiku): < 500 chars, single question, no code blocks
- **Moderate** (→ sonnet): 500-5000 chars, or contains code blocks
- **Complex** (→ opus): > 5000 chars, multi-step instructions, or system prompt mentions "architect"/"design"/"review"

Enabled via setting: `proxy.auto_routing: true`. Default: off.

Agent can opt out by setting model explicitly (auto-routing only applies when model is `auto` or omitted).

### Cross-Provider Cascade

On HTTP 429, 529, or 503 from upstream, retry with the next configured provider.

**Cascade order** (configurable):
```json
{
    "cascade": [
        {"provider": "anthropic", "model": "claude-sonnet-4-20250514"},
        {"provider": "openai", "model": "gpt-4o"},
        {"provider": "google", "model": "gemini-2.5-pro"}
    ]
}
```

Max retries: 2. Backoff: none (immediate retry to different provider).

### Response Caching

**Cache key (exact mode):** SHA-256 of `model + system_prompt + messages + max_tokens + temperature`

**Rules:**
- Only cache if `temperature == 0`
- TTL: 1 hour (configurable)
- Max cache size: 100 MB in-memory LRU
- Cache hits skip upstream entirely, return immediately
- Response includes `X-Coral-Cache-Hit: true` header

### Anomaly Detection

Four detectors, checked after each request:

1. **Velocity spike:** > 10x baseline request rate in 5-min window
2. **Cost acceleration:** Spending rate doubles between first and second half of rolling window
3. **Loop detection:** Same model + similar token count (within 10%) repeats 20+ times
4. **Token explosion:** Single request costs > $5

**Action on anomaly:** Log warning + send WebSocket event to dashboard. Phase 3 can add auto-pause.

---

## File Structure

```
coral-go/internal/proxy/
    proxy.go           -- Proxy struct, HTTP handlers, SSE streaming
    cost.go            -- ModelPricing map, calculateCost()
    store.go           -- proxy_requests CRUD, aggregation queries
    providers.go       -- Provider URL resolution, API key management
    middleware.go      -- Session extraction from URL path

    -- Phase 2
    budget.go          -- Budget enforcement, downgrade logic
    ratelimit.go       -- Token bucket rate limiter

    -- Phase 3
    router.go          -- Auto-routing classifier
    cascade.go         -- Cross-provider failover
    cache.go           -- Response cache (LRU + disk)
    anomaly.go         -- Anomaly detection
```

## Migration Path

The existing `token_usage` table captures cumulative snapshots from Claude hooks. The new `proxy_requests` table captures per-request granular data. Both can coexist:
- `token_usage` remains the source of truth for agents that don't route through the proxy
- `proxy_requests` provides granular data for proxied requests
- Dashboard queries can UNION both sources, preferring proxy data when available

## Non-Goals

- **Multi-node proxy:** This is a single-instance proxy running on the same host as Coral. No distributed routing.
- **Custom model hosting:** No support for local models or custom endpoints (can be added later via provider config).
- **Request/response modification:** The proxy is transparent — it does not modify request or response bodies (except model swaps for downgrade/routing).
