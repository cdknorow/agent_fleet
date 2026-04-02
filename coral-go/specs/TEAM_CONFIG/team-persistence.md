# Agent Team Persistence

Updated: 2026-04-02

## Problem

Agent teams are currently ephemeral — they exist only as a set of live sessions sharing a board name. The team config is either passed inline at launch and discarded, or stored as a JSON file on disk. This creates several problems:

1. **No team identity** — Once launched, there's no record of which sessions belong to a team, what config was used, or when it was created. The "team" is inferred from shared board membership.

2. **Worktree orphaning** — When a team uses a git worktree, the worktree path is stored per-session. If sessions are killed individually or the server restarts, the relationship to the worktree is lost and cleanup becomes unreliable.

3. **No team lifecycle** — You can't sleep/wake a team atomically, view team history, or relaunch a previously-used team config. Each launch is a fresh start.

4. **Config loss** — If a team is launched inline (not from a saved file), the config is gone after launch. You can't inspect what was launched or modify it for a relaunch.

5. **No team-level metadata** — There's nowhere to store team-level state like worktree path, launch time, status, or the relationship between a team and its agents.

## Design

### Core Concept

A **Team** is a first-class entity stored in the database. It holds the team configuration, tracks its lifecycle (created → running → sleeping → stopped), owns a worktree (optional), and maintains a relationship to its member sessions.

### Data Model

#### `teams` table

| Column | Type | Description |
|--------|------|-------------|
| `id` | INTEGER PK | Auto-increment |
| `name` | TEXT UNIQUE | Team name (also used as board_name) |
| `config_json` | TEXT | Full team config JSON (the launch payload) |
| `status` | TEXT | `running`, `sleeping`, `stopped` |
| `working_dir` | TEXT | The directory agents work in (may be a worktree, repo, or plain directory) |
| `is_worktree` | INTEGER | 1 if `working_dir` was created as a git worktree at launch (cleanup on stop) |
| `created_at` | TEXT | ISO 8601 timestamp |
| `updated_at` | TEXT | ISO 8601 timestamp |
| `stopped_at` | TEXT | When the team was stopped/killed (null if active) |

#### `live_sessions` updates

Add a `team_id` column (nullable INTEGER, FK to `teams.id`) so each session knows which team it belongs to. This replaces the current inference-by-board-name approach.

```sql
ALTER TABLE live_sessions ADD COLUMN team_id INTEGER REFERENCES teams(id) ON DELETE SET NULL;
```

### Schema

```sql
CREATE TABLE IF NOT EXISTS teams (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL UNIQUE,
    config_json     TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'running'
        CHECK (status IN ('running', 'sleeping', 'stopped')),
    working_dir     TEXT NOT NULL,
    is_worktree     INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    stopped_at      TEXT
);

CREATE INDEX IF NOT EXISTS idx_teams_status ON teams(status);
```

### Lifecycle

```
┌─────────┐   launch-team   ┌─────────┐
│         │ ──────────────→  │ running │
│ (none)  │                  │         │
└─────────┘                  └────┬────┘
                                  │
                    ┌─────────────┼─────────────┐
                    │ sleep       │ kill         │ kill-all
                    ▼             ▼              ▼
              ┌──────────┐  ┌─────────┐    ┌─────────┐
              │ sleeping │  │ stopped │    │ stopped │
              └────┬─────┘  └─────────┘    └─────────┘
                   │ wake        ▲
                   ▼             │ kill
              ┌─────────┐───────┘
              │ running │
              └─────────┘
```

**running** → All agents are live in tmux/PTY sessions. Board is active.

**sleeping** → Agents killed but sessions preserved in DB. Board paused. Team config and worktree retained. Can be woken to resume.

**stopped** → Team is finished. All sessions killed. Board unpaused. Worktree cleaned up (if `cleanup_worktree` is set). Config retained for history/relaunch.

### Team Operations

#### Launch Team
`POST /api/sessions/launch-team`

1. Create a `teams` row with the config, status=`running`
2. If `worktree: true`, create the worktree and store the path
3. Launch each agent session with `team_id` set
4. Return team ID, agent details, and worktree path

#### Kill Team
`POST /api/sessions/live/team/{boardName}/kill`

1. Kill all live sessions belonging to the team
2. Set team status to `stopped`, record `stopped_at`
3. If team has a worktree and `cleanup_worktree` is true, remove it
4. Unsubscribe agents from board

#### Sleep Team
`POST /api/sessions/live/team/{boardName}/sleep`

1. Kill tmux/PTY sessions (sessions stay in DB as sleeping)
2. Set team status to `sleeping`
3. Pause the board
4. Worktree is NOT cleaned up (preserved for wake)

#### Wake Team
`POST /api/sessions/live/team/{boardName}/wake`

1. Relaunch agents from the stored config
2. Set team status to `running`
3. Use the existing worktree (if any) as working directory
4. Unpause the board

#### Get Team
`GET /api/teams/{name}`

Returns the team record including config, status, worktree path, and member sessions.

#### List Teams
`GET /api/teams`

Returns all teams with status and summary info. Optionally filter by `?status=running`.

#### Relaunch Team
`POST /api/teams/{name}/relaunch`

Launches a new instance of a stopped team using its stored config. Creates a new `teams` row (or reuses the existing one if the name is unique). Optionally creates a fresh worktree.

### Worktree Handling

The team stores a `working_dir` and an `is_worktree` flag. The team doesn't care whether the directory is a worktree — it's just the path where agents work. The `is_worktree` flag is metadata that tells the cleanup logic whether `git worktree remove` should be called when the team stops.

**At launch:** If the user checks "Use Git Worktree," the launcher creates a worktree from `base_branch`, sets the team's `working_dir` to the worktree path, and sets `is_worktree = 1`. If no worktree is requested, `working_dir` is the user's chosen directory and `is_worktree = 0`.

**On sleep:** The `working_dir` is preserved regardless. Agents are killed but the directory stays intact for wake.

**On kill/stop:** If `is_worktree = 1`, run `git worktree remove --force` on `working_dir` with a 30-second timeout. If `is_worktree = 0`, do nothing — the directory belongs to the user.

**Orphan protection:** On server startup, scan for teams with status=`running` where no live sessions exist. Mark them as `stopped` and clean up worktrees where `is_worktree = 1`.

### Migration from Current Behavior

**Backward compatibility:** The `launch-team` API continues to work with the existing payload. Internally, it now creates a `teams` row. Old sessions without a `team_id` continue to work — they're treated as ad-hoc sessions not belonging to a team.

**Board inference:** The sidebar can still group sessions by board name for backward compatibility, but should prefer `team_id` grouping when available.

**File-based configs:** `~/.coral/teams/*.json` files remain supported for load/save. They're the serialization format for team configs, while the `teams` table is the runtime state.

### API Summary

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/sessions/launch-team` | Launch team (creates teams row) |
| POST | `/api/sessions/live/team/{name}/kill` | Kill team + cleanup worktree |
| POST | `/api/sessions/live/team/{name}/sleep` | Sleep team (preserve worktree) |
| POST | `/api/sessions/live/team/{name}/wake` | Wake team (reuse worktree) |
| GET | `/api/teams` | List all teams |
| GET | `/api/teams/{name}` | Get team details + member sessions |
| POST | `/api/teams/{name}/relaunch` | Relaunch a stopped team |
| DELETE | `/api/teams/{name}` | Delete a stopped team record |

### UI Changes

1. **Sidebar:** Show team name as a group header with status badge (running/sleeping/stopped). Clicking the header shows team-level actions (sleep, wake, kill, view config).

2. **Teams tab or section:** List all teams with status, agent count, worktree path, and last activity. Allow relaunch from here.

3. **Launch modal:** No changes needed — it already has the worktree checkbox. The backend now creates the team record automatically.

### Implementation Phases

**Phase 1: Core persistence**
- `teams` table + store CRUD
- `team_id` on `live_sessions`
- LaunchTeam creates team row
- Kill/Sleep/Wake update team status
- Worktree owned by team, cleaned on kill

**Phase 2: API + UI**
- GET /api/teams, GET /api/teams/{name}
- Relaunch endpoint
- Sidebar team grouping with status
- Team detail view

**Phase 3: Lifecycle management**
- Orphan detection on startup
- Team history (stopped teams retained for reference)
- Delete old stopped teams
