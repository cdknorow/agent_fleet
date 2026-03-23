# Agent Support Architecture

This document describes Coral's agent abstraction layer and provides a guide for adding support for new AI agents.

## Current Agent Interface

All agents implement the `Agent` interface defined in `internal/agent/agent.go`:

```go
type Agent interface {
    AgentType() string                              // e.g., "claude", "gemini", "codex"
    SupportsResume() bool                           // Can resume previous sessions
    BuildLaunchCommand(params LaunchParams) string  // Build CLI launch command
    PrepareResume(sessionID, workingDir string)      // Pre-launch session setup
    HistoryBasePath() string                        // Root directory for history files
    HistoryGlobPattern() string                     // Glob pattern for finding session files
}
```

Factory function in `agent.go`:
```go
func GetAgent(agentType string) Agent
```

## Supported Agents

| Agent | Type String | CLI Command | History Path | History Format | Resume |
|-------|------------|-------------|--------------|----------------|--------|
| Claude Code | `"claude"` | `claude` | `~/.claude/projects/{dir}/{id}.jsonl` | JSONL (messages, tool_use, tool_result) | Yes |
| Gemini CLI | `"gemini"` | `gemini` | `~/.gemini/tmp/{dir}/session-*.json` | JSON (role/parts array) | No |
| Codex CLI | `"codex"` | `codex` | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` | JSONL (messages, tool calls, results) | Yes |

## Architecture: What's Agent-Specific vs Generic

### Agent-Specific (requires per-agent implementation)

| Component | File | What to Implement |
|-----------|------|-------------------|
| Agent struct | `internal/agent/{type}.go` | All Agent interface methods |
| History path resolution | `internal/jsonl/reader.go` | `resolve{Type}Transcript()` function |
| Transcript parsing | `internal/jsonl/reader.go` | `parse{Type}Entry()` function |
| Factory registration | `internal/agent/agent.go` | Add case to `GetAgent()` switch |
| CLI name mapping | `internal/agent/agent.go` | Add entry to `CLINames` map |

### Agent-Generic (no changes needed)

| Component | File | Why It's Generic |
|-----------|------|-----------------|
| PULSE protocol | `internal/pulse/parser.go` | All agents emit `\|\|PULSE:STATUS\|\|`, `\|\|PULSE:SUMMARY\|\|` |
| PTY/tmux management | `internal/ptymanager/` | Session names embed agent type but behavior is identical |
| HTTP API routes | `internal/server/routes/` | `agent_type` passed as parameter, dispatched via factory |
| Background services | `internal/background/` | Enumerate all agents regardless of type |
| Message board | `internal/board/` | Uses `coral-board` CLI, agent-agnostic |
| Git polling | `internal/background/` | Groups by working directory, not agent type |
| Database schema | `internal/store/` | `agent_type` column stores any string |

### Optional Per-Agent Features

| Feature | Example | Notes |
|---------|---------|-------|
| Permission translation | `TranslateToClaudePermissions()` | Maps Coral capabilities to agent-specific tool names |
| Hook injection | Claude's settings.json hooks | Agent-specific configuration mechanism |
| System prompt injection | Gemini's `GEMINI_SYSTEM_MD` env var | How the agent receives Coral's instructions |

## Session Naming Convention

Sessions are named `{agent_type}-{uuid}`, e.g., `codex-550e8400-e29b-41d4-a716-446655440000`. This embeds the agent type in the session identifier, making it discoverable from tmux session names.

## Guide: Adding a New Agent

### Step 1: Create the Agent Implementation

Create `internal/agent/{type}.go`:

```go
type MyAgent struct{}

func (a *MyAgent) AgentType() string           { return "myagent" }
func (a *MyAgent) SupportsResume() bool        { return true }
func (a *MyAgent) HistoryBasePath() string     { return filepath.Join(homeDir, ".myagent", "history") }
func (a *MyAgent) HistoryGlobPattern() string  { return "session-*.jsonl" }
func (a *MyAgent) BuildLaunchCommand(params LaunchParams) string { /* ... */ }
func (a *MyAgent) PrepareResume(sessionID, workingDir string)    { /* ... */ }
```

### Step 2: Register in Factory

In `internal/agent/agent.go`, add to `GetAgent()`:
```go
case "myagent":
    return &MyAgent{}
```

Add to `CLINames` map if the agent uses a different board CLI name.

### Step 3: Add History/Transcript Support

In `internal/jsonl/reader.go`:

1. Add `resolveMyAgentTranscript(sessionID)` — returns the file path for a session's transcript
2. Add `parseMyAgentEntry(entry)` — parses one JSONL/JSON entry into the normalized message format
3. Add cases to `resolveTranscriptPath()` and `parseTranscriptEntry()` switch statements

The normalized message format all parsers must produce:
```go
map[string]any{
    "type":      "user" | "assistant",  // message role
    "timestamp": "2024-01-22T10:30:00", // ISO timestamp
    "content":   "text content",        // string or structured content
}
```

### Step 4: PULSE Protocol Support

If your agent runs in a terminal, it should emit PULSE markers for status tracking:
```
||PULSE:STATUS Working on authentication module||
||PULSE:SUMMARY Implementing OAuth2 login flow||
||PULSE:CONFIDENCE High Successfully connected to API||
```

This is typically injected via the agent's system prompt (in `BuildLaunchCommand` or equivalent).

### Step 5: Test

1. `go build ./...` — verify compilation
2. Launch a session: `POST /api/sessions/launch` with `agent_type: "myagent"`
3. Verify session appears in live sessions list
4. Verify history parsing after session ends
5. If resume supported, verify session continuation

## Modularity Gaps — What Needs Refactoring

The following areas are currently Claude-specific and need to be generalized for multi-agent support.

### 1. Board Prompt Logic (Duplicated)

**Problem:** `claude.go` has `buildBoardSystemPrompt()` (~40 lines) and board action prompt injection (~30 lines) that construct the message board instructions for the agent. `codex.go` duplicates a simpler version. `gemini.go` has none.

**Files:** `internal/agent/claude.go` lines 39-80, 136-165; `internal/agent/codex.go` lines 48-56

**Proposed fix:** Extract a shared `BuildBoardPrompt(params LaunchParams) string` helper in `agent.go` that all agents call. It should:
- Build the board CLI usage instructions (read/post/subscribers)
- Append orchestrator vs worker action prompts (using overrides if set)
- Replace `{board_name}` and CLI name placeholders
- Return the combined prompt string

Each agent's `BuildLaunchCommand` then calls this helper and injects the result via its own mechanism (Claude: `--settings` systemPrompt, Gemini: `GEMINI_SYSTEM_MD`, Codex: `--system-prompt`).

### 2. Settings/Hooks Injection (Claude-Only)

**Problem:** `buildMergedSettings()` in `claude.go` (lines 237-290) reads `~/.claude/settings.json`, merges project/local settings, deep-merges hooks, injects `coralHooks`, and writes a temp settings file passed via `--settings`. This is entirely Claude-specific.

**Files:** `internal/agent/claude.go` lines 212-350

**Impact:** Other agents need their own config injection:
- **Codex:** Has `~/.codex/` config, unclear if it supports settings file injection
- **Gemini:** Uses env vars (`GEMINI_SYSTEM_MD`) and has no equivalent settings mechanism

**Proposed fix:** Define a `ConfigInjector` interface or per-agent `InjectConfig(workdir string) map[string]any` method. The hooks (`coralHooks`) should be defined generically — each agent translates them to its native hook format. For agents without hook support, Coral can fall back to polling-based detection (which already works via the background services).

### 3. Permission Translation (Claude-Only)

**Problem:** `TranslateToClaudePermissions()` maps Coral `Capabilities` to Claude's tool name format (`"Bash(git push *)"`, `"Read"`, `"Write"`, etc.). No equivalent exists for Codex or Gemini.

**Files:** `internal/agent/permissions.go` lines 40-90

**Proposed fix:** Add a `TranslatePermissions(agentType string, caps *Capabilities)` dispatch function, or add a `TranslatePermissions(caps *Capabilities) any` method to the Agent interface. Each agent implements its own mapping:
- **Claude:** Current `TranslateToClaudePermissions()` (tool name patterns)
- **Codex:** Maps to `--full-auto` flag or codex-specific sandbox settings
- **Gemini:** Maps to whatever permission model Gemini CLI supports (if any)

### 4. Factory Not Complete

**Problem:** `GetAgent()` defaults unknown types to Claude. `GetAllAgents()` only returns Claude and Gemini — missing Codex.

**Files:** `internal/agent/agent.go` lines 50-62

**Fix:** Add `"codex"` case to `GetAgent()`. Add `&CodexAgent{}` to `GetAllAgents()`.

### 5. Prompt Constants Are Claude-Scoped

**Problem:** `DefaultOrchestratorSystemPrompt`, `DefaultWorkerSystemPrompt`, `DefaultOrchestratorActionPrompt`, `DefaultWorkerActionPrompt` are defined in `claude.go` but used by `codex.go` too. They're not Claude-specific — they're Coral-level prompts.

**Files:** `internal/agent/claude.go` lines 28-37

**Fix:** Move these constants to `agent.go` so all agents can reference them without cross-file dependencies.

## Modularity Notes

- The `"terminal"` agent type is special — it represents a plain shell session, not an AI agent. It's handled as a special case in the launcher.
- Unknown agent types default to Claude. When adding a new agent, always register it explicitly.
- The PULSE protocol is the primary mechanism for real-time status. If an agent doesn't support PULSE, it will still work but won't show live status in the dashboard.
