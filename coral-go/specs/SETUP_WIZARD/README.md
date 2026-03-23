# First-Run Setup Wizard

## Goal

Guide new users through detecting, installing, and configuring agent CLIs (Claude, Codex, Gemini) on first launch or when an agent CLI is missing.

## UX Flow

### First-Run (full-page onboarding)

Shown when no agents have been launched yet (no sessions in DB).

```
┌─────────────────────────────────────────────────────────┐
│  Welcome to Coral                                       │
│                                                         │
│  Coral orchestrates AI coding agents. Let's check       │
│  which agents are available on your system.              │
│                                                         │
│  ┌─────────────────────────────────────────────────┐    │
│  │  ✅ Claude Code         /usr/local/bin/claude    │    │
│  │  ❌ Codex CLI           Not found                │    │
│  │     npm install -g @openai/codex        [Copy]   │    │
│  │  ❌ Gemini CLI          Not found                │    │
│  │     pip install gemini-cli              [Copy]   │    │
│  └─────────────────────────────────────────────────┘    │
│                                                         │
│  Custom CLI Paths (optional)                            │
│  ┌──────────────────────────────────────────────────┐   │
│  │  Claude: [                                     ] │   │
│  │  Codex:  [                                     ] │   │
│  │  Gemini: [                                     ] │   │
│  └──────────────────────────────────────────────────┘   │
│                                                         │
│                              [Skip]  [Continue →]       │
└─────────────────────────────────────────────────────────┘
```

### Contextual (modal overlay)

Shown when a user tries to launch an agent that isn't found.

```
┌─────────────────────────────────────────────────────┐
│  Codex CLI not found                                │
│                                                     │
│  Install it:                                        │
│  npm install -g @openai/codex              [Copy]   │
│                                                     │
│  Or set a custom path:                              │
│  [ /home/user/.nvm/.../bin/codex         ]          │
│                                                     │
│  After installing, click Re-check.                  │
│                                                     │
│              [Cancel]  [Re-check]  [Save Path]      │
└─────────────────────────────────────────────────────┘
```

## Backend API

### GET /api/system/cli-status

Returns availability status for all agent CLIs.

```json
{
  "agents": {
    "claude": {
      "available": true,
      "path": "/usr/local/bin/claude",
      "version": "1.0.23",
      "install_command": "npm install -g @anthropic-ai/claude-code"
    },
    "codex": {
      "available": false,
      "path": "",
      "version": "",
      "install_command": "npm install -g @openai/codex"
    },
    "gemini": {
      "available": false,
      "path": "",
      "version": "",
      "install_command": "pip install gemini-cli"
    }
  }
}
```

Detection logic:
1. Check user setting `cli_path_{agent_type}` — if set, check that path
2. Check PATH via `exec.LookPath(binary)`
3. Check common install locations (platform-specific):
   - **macOS**: `/opt/homebrew/bin`, `/usr/local/bin`, `~/.nvm/versions/*/bin`
   - **Windows**: `%APPDATA%\npm`, `%LOCALAPPDATA%\Programs`, `%ProgramFiles%`
   - **Linux**: `~/.local/bin`, `~/.nvm/versions/*/bin`, `/snap/bin`
4. If found, get version via `{binary} --version`

### GET /api/system/first-run

Returns whether the wizard should be shown.

```json
{
  "show_wizard": true,
  "reason": "no_sessions"
}
```

Show wizard when:
- No sessions have ever been created (first run)
- User setting `setup_wizard_completed` is not set

Don't show when:
- `setup_wizard_completed` is set to "true"
- Sessions exist in the DB

### PUT /api/settings

Existing endpoint. Used to save:
- `cli_path_claude`, `cli_path_codex`, `cli_path_gemini`
- `setup_wizard_completed: "true"` (to dismiss the wizard)

## Platform-Specific Detection

### macOS
- npm global: `/usr/local/lib/node_modules/.bin/`, `/opt/homebrew/lib/node_modules/.bin/`
- nvm: `~/.nvm/versions/node/*/bin/`
- Homebrew: `/opt/homebrew/bin/`
- pip: `~/.local/bin/`, `/Library/Frameworks/Python.framework/Versions/*/bin/`

### Windows
- npm global: `%APPDATA%\npm\`
- nvm-windows: `%USERPROFILE%\.nvm\*`
- pip: `%APPDATA%\Python\*\Scripts\`
- Program Files: `%ProgramFiles%\`, `%LOCALAPPDATA%\Programs\`

### Linux
- npm global: `/usr/lib/node_modules/.bin/`, `~/.npm-global/bin/`
- nvm: `~/.nvm/versions/node/*/bin/`
- pip: `~/.local/bin/`
- snap: `/snap/bin/`

## Implementation Plan

1. **Backend**: Add `/api/system/cli-status` endpoint with platform-specific detection
2. **Backend**: Add `/api/system/first-run` endpoint
3. **Frontend**: Full-page onboarding component (shown on first run)
4. **Frontend**: Contextual modal (shown on agent-not-found error)
5. **Frontend**: Wire both to the API endpoints

## Future Enhancements

- API key detection (ANTHROPIC_API_KEY, OPENAI_API_KEY, GEMINI_API_KEY)
- One-click install via embedded terminal
- Agent version update notifications
