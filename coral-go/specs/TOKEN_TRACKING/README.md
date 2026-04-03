# Token Usage Tracking

## Problem

No visibility into token consumption across agents and teams. Users can't tell how much each agent or team costs, which agents are expensive, or what the total spend is for a session.

## Data Source

Claude Code's hook events include token usage data:

- **Stop hook**: `total_cost_usd`, `total_input_tokens`, `total_output_tokens`, `num_turns` — cumulative totals for the session at stop time
- **PostToolUse hook**: May include per-turn token deltas (needs verification)

Gemini CLI and Codex don't currently expose token data via hooks. Support for those can be added later.

## Design

### Capture

Update `coral-hook-agentic-state` to extract token fields from the Stop hook payload and forward them to the Coral API:

```go
// In the Stop hook handler:
if hookType == "Stop" {
    event["total_cost_usd"] = d["total_cost_usd"]
    event["total_input_tokens"] = d["total_input_tokens"]
    event["total_output_tokens"] = d["total_output_tokens"]
    event["num_turns"] = d["num_turns"]
}
```

### Store

New `token_usage` table:

```sql
CREATE TABLE IF NOT EXISTS token_usage (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id      TEXT NOT NULL,
    agent_name      TEXT NOT NULL,
    agent_type      TEXT NOT NULL DEFAULT 'claude',
    team_id         INTEGER,
    board_name      TEXT,
    input_tokens    INTEGER NOT NULL DEFAULT 0,
    output_tokens   INTEGER NOT NULL DEFAULT 0,
    total_tokens    INTEGER NOT NULL DEFAULT 0,
    cost_usd        REAL NOT NULL DEFAULT 0,
    num_turns       INTEGER NOT NULL DEFAULT 0,
    recorded_at     TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_token_usage_session ON token_usage(session_id);
CREATE INDEX IF NOT EXISTS idx_token_usage_team ON token_usage(team_id);
CREATE INDEX IF NOT EXISTS idx_token_usage_time ON token_usage(recorded_at);
```

Each row is a snapshot — recorded when the agent stops (or periodically via PostToolUse if we add incremental tracking later). The latest row per session_id gives the current cumulative total.

### API

**POST /api/sessions/live/{name}/token-usage** — Called by the Stop hook to report token usage.

**GET /api/token-usage** — Query token usage with filters:
- `?session_id=X` — single session
- `?team_id=N` — all sessions in a team  
- `?board_name=X` — all sessions on a board
- `?since=2026-04-01` — time range filter
- Returns per-session breakdown + aggregated totals

**GET /api/token-usage/summary** — High-level summary:
- Total tokens (input + output) and cost across all sessions
- Per-team totals
- Per-agent-type totals (Claude vs Gemini vs Codex)

### API Flow

```
Agent stops → Stop hook fires → coral-hook-agentic-state extracts tokens
  → POST /api/sessions/live/{name}/token-usage
    → Insert into token_usage table
```

### UI Display

**Per-agent**: Show token count and cost in the session header or sidebar entry. Format: "12.4K tokens · $0.03"

**Per-team**: Show aggregated totals in the team section header. Format: "Team total: 145K tokens · $0.42"

**Token usage panel**: Optional tab/panel showing breakdown by agent with bar chart or table.

## Per-Agent Capture Methods

### Claude

**Source**: Stop hook payload (real-time, native).

Claude Code's Stop event includes cumulative session totals:

```json
{
  "hook_event_name": "Stop",
  "session_id": "abc-123",
  "reason": "end_turn",
  "total_cost_usd": 0.0342,
  "total_input_tokens": 12450,
  "total_output_tokens": 3280,
  "num_turns": 7
}
```

**Capture**: `coral-hook-agentic-state` extracts these fields from the Stop event and POSTs to `/api/sessions/live/{name}/token-usage`.

**Frequency**: Every time the agent stops (end of each turn). Cumulative — each report supersedes the previous one for that session.

### Gemini

**Source**: Conversation transcript JSON at `~/.gemini/chats/{id}/chats/{sessionID}.json`.

Gemini CLI stores transcripts as JSON arrays. Each response object includes `usageMetadata`:

```json
{
  "role": "model",
  "parts": [...],
  "usageMetadata": {
    "promptTokenCount": 1234,
    "candidatesTokenCount": 567,
    "totalTokenCount": 1801
  }
}
```

**Capture**: A background poller (or the existing git poller cycle) periodically reads the Gemini transcript file, finds the last `usageMetadata` entry, and sums cumulative token counts. The JSONL reader (`internal/jsonl/reader.go`) already resolves Gemini transcript paths — extend it to extract usage metadata.

**Frequency**: Polled every N seconds (configurable, default 30s). Less real-time than Claude but adequate.

**Cost estimation**: Gemini doesn't report cost directly. Estimate using published per-token pricing based on the model name (from the team config).

### Codex

**Source**: Session JSONL transcript at `~/.codex/sessions/{date}/rollout-{id}.jsonl`.

Codex CLI stores transcripts as JSONL with each response including `usage`:

```json
{
  "role": "assistant",
  "content": [...],
  "usage": {
    "input_tokens": 2500,
    "output_tokens": 800,
    "total_tokens": 3300
  }
}
```

**Capture**: Same polling approach as Gemini. The JSONL reader already resolves Codex transcript paths (`resolveCodexTranscript`). Parse the last entry with `usage` data to get cumulative totals.

**Frequency**: Polled every N seconds alongside Gemini.

**Cost estimation**: Estimate from published per-token pricing based on the model name.

### Summary of capture methods

| Agent | Source | Method | Latency | Cost data |
|-------|--------|--------|---------|-----------|
| Claude | Stop hook | Real-time push via hook | Immediate | Native (USD) |
| Gemini | Transcript JSON | Background poll | ~30s | Estimated from model pricing |
| Codex | Transcript JSONL | Background poll | ~30s | Estimated from model pricing |

## Token Usage Poller

New background service: `TokenPoller` (alongside GitPoller, BoardNotifier, etc.)

- Runs on a configurable interval (default 30s)
- For each live agent session:
  - Claude: skip (handled by hooks in real-time)
  - Gemini: read transcript, extract last `usageMetadata`, POST to token-usage endpoint
  - Codex: read transcript, extract last `usage`, POST to token-usage endpoint
- Only processes sessions that have changed since last poll (check file mtime)

## Model Pricing

For Gemini and Codex cost estimation, maintain a simple pricing table:

```go
var modelPricing = map[string]struct{ InputPer1M, OutputPer1M float64 }{
    // Gemini
    "gemini-2.5-pro":   {1.25, 10.00},
    "gemini-2.5-flash": {0.15, 0.60},
    // Codex / OpenAI
    "o3":               {10.00, 40.00},
    "o4-mini":          {1.10, 4.40},
    "gpt-4.1":          {2.00, 8.00},
}
```

Updated periodically as pricing changes. Falls back to $0 if model is unknown.

## Implementation Phases

**Phase 1: Claude capture + Store**
- Extract token fields in Stop hook
- token_usage table + store methods
- POST endpoint to receive usage data
- Display in session header

**Phase 2: Gemini + Codex capture**
- TokenPoller background service
- Transcript parsing for Gemini/Codex usage metadata
- Model pricing table for cost estimation

**Phase 3: API + Aggregation + UI**
- GET /api/token-usage with filters (session, team, time range)
- GET /api/token-usage/summary (totals by team and agent type)
- Per-team totals in sidebar
- Token usage breakdown panel
