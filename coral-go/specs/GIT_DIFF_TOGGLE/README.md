# Git Diff Mode Toggle

## Problem

The file diff view currently diffs against the `main` branch. This confuses
users when their local `main` is ahead of the remote — the diff shows files they
haven't touched in their feature branch because `main` has advanced.

Users need to choose what they're comparing against:

1. **What changed in my last commit?** (diff against `HEAD~1`)
2. **What changed since I branched from main?** (diff against merge-base)

## Setting

User setting: `git_diff_mode`
- `"previous"` — Diff against previous commit on current branch (`HEAD~1`)
- `"merge-base"` (default) — Diff against the branching point from main
  (`git merge-base main HEAD`)

Stored in `user_settings` table via `PUT /api/settings`.

## Mode 1: Previous Commit (`HEAD~1`)

Shows what changed in the most recent commit only.

**Git command:** `git diff HEAD~1`

**Use case:** Reviewing the last commit before pushing, verifying a single
change in isolation.

**Limitation:** Only shows the latest commit's changes. If the user has
multiple uncommitted changes or multiple commits since branching, this won't
show the full picture.

## Mode 2: Merge-Base (default)

Shows all changes since the current branch diverged from main.

**Git commands:**
```bash
# Find the branching point
base=$(git merge-base main HEAD)
# Diff from that point
git diff $base
```

**Use case:** Reviewing all feature branch changes before creating a PR.
Shows the complete diff that would appear in a pull request.

**Handling edge cases:**
- If on `main` branch: falls back to `HEAD~1`
- If `main` doesn't exist (e.g., `master` is the default): try `master`,
  then fall back to `HEAD~1`
- If no commits yet: show empty diff

## API Changes

The existing diff endpoint needs to accept a mode parameter:

`GET /api/sessions/live/{name}/diff?mode={previous|merge-base}`

Or the frontend can read the setting and construct the appropriate git
command parameters.

## UI

A toggle in the diff panel header or settings:
- "Last commit" / "Since branch" (or similar labels)
- Persisted via the `git_diff_mode` setting
- Changing the mode refreshes the diff view immediately

## Files Involved

- Backend: `sessions.go` — Diff handler (needs mode parameter)
- Backend: `tmux/client.go` or `gitutil/` — Git merge-base command
- Frontend: diff panel component — Toggle control, mode-aware fetching
- Frontend: settings UI — Optional setting entry

## Edge Cases

- **Detached HEAD**: `merge-base` may fail. Fall back to `HEAD~1`.
- **No previous commit**: `HEAD~1` fails on initial commit. Show empty diff.
- **Renamed default branch**: Try both `main` and `master` for merge-base.
- **Uncommitted changes**: Both modes should include staged + unstaged changes.
  Consider `git diff HEAD~1` vs `git diff HEAD~1 --staged` + `git diff`.
- **Merge commits**: `HEAD~1` on a merge commit shows the diff against the
  first parent, which is typically the branch being merged into.
