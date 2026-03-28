# Top Navigation Redesign

## Problem

The sidebar is cluttered — it contains:
- Team board name + board controls
- Live agent list (grouped by team)
- Chat History section with search
- Group chat / board messages
- Session summaries

Everything stacks vertically in ~250px of width. With a full team (7+ agents)
plus chat history, users have to scroll extensively. The agent list — the most
important element — gets squeezed.

## Reference Design

The reference UI uses a top navigation bar to separate major sections:

```
┌─────────────────────────────────────────────────────┐
│  Coral    Dashboard │ Agents │ History │ Logs   🔍  │
├──────┬──────────────────────────────────┬────────────┤
│      │                                  │            │
│ List │         Main Content             │   Panel    │
│      │                                  │            │
└──────┴──────────────────────────────────┴────────────┘
```

The sidebar only shows items relevant to the selected top-level section.

## Proposed Navigation Structure

### Top Nav Tabs

| Tab | What it shows | Sidebar content |
|-----|--------------|-----------------|
| **Agents** (default) | Terminal / live session view | Live agents grouped by team |
| **History** | Session history detail view | Session list with search + filters |
| **Board** | Message board view | Board project list |
| **Settings** | Settings modal (existing) | N/A (modal, not a tab) |

### What moves out of the sidebar

| Element | Current location | New location |
|---------|-----------------|--------------|
| Chat History section | Bottom of sidebar | **History** tab (becomes the sidebar) |
| Search chats input | In Chat History section | History tab sidebar |
| Board/group info | Top of sidebar | **Board** tab |
| Session summaries | Bottom of sidebar | History tab detail view |

### What stays in the sidebar (Agents tab)

- `+ New` button
- Team name header (collapsible)
- Agent list (name + status dot)
- That's it — clean and focused

## Layout Per Tab

### Agents Tab (default, current behavior)
```
┌──────────────────────────────────────────────────┐
│  Coral   [Agents] History  Board        🔍  ⚙️  │
├────────┬─────────────────────────┬───────────────┤
│ +New   │                         │ Files  Chat   │
│        │     Terminal Output     │ Tasks  Notes  │
│ Team A │                         │ Activity      │
│  Agent │                         │               │
│  Agent │                         │               │
│ Team B │                         │               │
│  Agent │    $ command input      │               │
└────────┴─────────────────────────┴───────────────┘
```

### History Tab
```
┌──────────────────────────────────────────────────┐
│  Coral   Agents  [History]  Board        🔍  ⚙️ │
├────────┬─────────────────────────────────────────┤
│ Search │                                         │
│ Filter │     Session Detail / Summary            │
│        │     Git Changes                         │
│ Mar 28 │     Agent Notes                         │
│  sess1 │     Events Timeline                     │
│  sess2 │                                         │
│ Mar 27 │                                         │
│  sess3 │                                         │
└────────┴─────────────────────────────────────────┘
```

### Board Tab
```
┌──────────────────────────────────────────────────┐
│  Coral   Agents  History  [Board]        🔍  ⚙️ │
├────────┬─────────────────────────────────────────┤
│ Boards │                                         │
│        │     Message Thread                      │
│ proj-1 │     [Author] message...                 │
│ proj-2 │     [Author] message...                 │
│ proj-3 │                                         │
│        │     [ Type a message...        Send ]   │
└────────┴─────────────────────────────────────────┘
```

## Implementation

### Phase 1: Add top nav tabs (HTML + CSS)
- Add tab buttons to the top bar in `index.html`
- Style as horizontal tabs (active tab has bottom border accent)
- CSS in `layout.css` for `.top-nav-tab` styling

### Phase 2: Restructure sidebar content (JS + HTML)
- `Agents` tab: show only `#live-sessions-section`
- `History` tab: show only `#history-section` (move from bottom of sidebar)
- `Board` tab: show board project list (currently in message board view)
- Tab switching via JS — show/hide sidebar sections

### Phase 3: Wire up view switching (JS)
- Clicking a top nav tab switches both sidebar content AND main view
- `Agents` → show live session view (terminal)
- `History` → show history session view
- `Board` → show message board view
- Preserve existing `showView()` logic — just trigger it from top nav

### What doesn't change
- All existing JS functionality (sessions, history, board, settings)
- Right panel (agentic state tabs)
- Terminal rendering
- API endpoints
- Mobile layout (already has bottom tab bar)

## Trade-offs

| Approach | Pros | Cons |
|----------|------|------|
| Top nav tabs | Clean sidebar, clear sections, matches reference | Template change, JS routing change |
| Keep current | No work | Sidebar stays cluttered |
| Collapsible sections | Less work than tabs | Still cluttered, just hidden |

## Risk

Medium — this is a structural template + JS change, not just CSS. The sidebar
HTML needs to be split into sections that can be shown/hidden per tab. Existing
JS that references sidebar elements needs to work regardless of which tab is
active.

## Decision Log

| Decision | Rationale |
|----------|-----------|
| Three tabs (Agents/History/Board) | Maps to the three main use cases |
| Settings stays as modal | Already works, no reason to change |
| Mobile keeps bottom tab bar | Already implemented, different navigation pattern |
| Top nav, not sidebar tabs | Matches reference, frees vertical sidebar space |
