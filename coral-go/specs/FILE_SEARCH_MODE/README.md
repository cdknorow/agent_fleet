# File Search Mode

## Problem

The @ mention autocomplete and file browser panel use fuzzy matching to search
for files. This works well when users know part of a filename, but is frustrating
when browsing unfamiliar codebases — users want to explore the directory
structure progressively, like tab-completion in a terminal.

The existing fuzzy matching also has scoring issues: the position penalty
(`score += ti`) is too aggressive, making deep paths score poorly even with
perfect basename matches.

## Setting

User setting: `file_search_mode`
- `"directory"` (default) — Tab-completion style directory navigation
- `"fuzzy"` — Fuzzy substring matching with improved scoring weights

Stored in `user_settings` table via `PUT /api/settings`. Read on page load by
the frontend to determine which mode to activate.

## Mode 1: Directory Browsing (default)

Progressive directory navigation, similar to terminal tab-completion:

1. User starts typing `@` or opens file browser -> show root directory entries
2. Directories shown with trailing `/`, sorted before files
3. When user types `/` or selects a directory -> expand into that directory
4. Backspace past a `/` goes up one level
5. Within a directory, type to filter entries by name

### API

`GET /api/sessions/live/{name}/search-files?dir={path}&q={filter}`

**Parameters:**
- `dir` — Directory path to list (relative to git root). Use `.` or empty for
  root. Required for directory browsing mode.
- `q` — Optional filter to narrow entries within the directory
- `session_id` — Optional session identifier

**Response:**
```json
{
  "entries": [
    {"name": "src/", "path": "src", "type": "dir"},
    {"name": "internal/", "path": "internal", "type": "dir"},
    {"name": "go.mod", "path": "go.mod", "type": "file"},
    {"name": "main.go", "path": "main.go", "type": "file"}
  ],
  "dir": "."
}
```

**Entry fields:**
- `name` — Display name. Directories have trailing `/`.
- `path` — Full relative path from git root (no trailing `/` for dirs).
- `type` — `"dir"` or `"file"`.

**Behavior:**
- Only git-tracked files are shown (`git ls-files --cached --others --exclude-standard`)
- Directories are derived from file paths (a directory exists if any tracked file
  is inside it)
- Directories sort before files; both sorted alphabetically
- Path traversal is blocked (`..` rejected)
- Capped at 100 entries per request

### UX Flow

1. User types `@` or opens file browser
2. Frontend fetches `?dir=.` -> shows root entries
3. User sees: `cmd/`, `internal/`, `go.mod`, `main.go`
4. User clicks `cmd/` or types `cmd/`
5. Frontend fetches `?dir=cmd` -> shows cmd's contents
6. User types `co` -> frontend fetches `?dir=cmd&q=co` -> filters to `coral/`
7. User selects `coral/` -> fetches `?dir=cmd/coral` -> shows its contents
8. User presses backspace past `/` -> goes back to `cmd/` listing
9. User selects a file -> @ mention is completed with the file path

## Mode 2: Fuzzy Matching (existing, improved scoring)

The existing fuzzy substring matching with corrected scoring weights.

### Scoring Improvements

**Current issues:**
- Position penalty (`score += ti`) adds the character INDEX to the score, making
  deep paths score terribly even with perfect basename matches
- No bonus for exact basename match vs partial basename match

**Fixed scoring hierarchy:**
1. Exact basename match (score 0) — highest priority
2. Basename contains query (score 1)
3. Path contains query (score 2)

### API

Same as existing: `GET /api/sessions/live/{name}/search-files?q={query}`

Returns: `{"files": ["path/to/file1", ...]}`

## Files Involved

- `coral-go/internal/server/routes/sessions.go` — `SearchFiles()`, `searchFilesDir()`
- `coral-go/internal/server/routes/system.go` — `GetSettings()`, `PutSettings()`
- Frontend: `file_mention.js` — @ mention autocomplete
- Frontend: `changed_files.js` — File browser panel
- Frontend: settings UI — Toggle control

## Edge Cases

- **Empty directories**: Return empty entries array, not an error
- **Untracked files**: Included via `--others --exclude-standard`
- **Submodules**: Shown as directories (tracked files inside appear)
- **Hidden files (dotfiles)**: Shown if git-tracked
- **Unicode filenames**: Properly sorted and displayed
- **Large directories (>100 entries)**: Truncated at 100
- **Paths with spaces**: URL-encoded in API calls
