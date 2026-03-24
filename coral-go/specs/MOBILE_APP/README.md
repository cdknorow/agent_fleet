# Mobile App Improvements

## Current State

### Breakpoints
- **Tablet** (max-width 1023px): sidebar becomes overlay, agentic panel hidden, compact command pane
- **Mobile** (max-width 767px): bottom tab bar, no sidebar, mobile agent list, sticky command input
- **Touch** (hover:none + pointer:coarse): larger touch targets, visible scrollbars

### What Works
- Bottom tab bar (Agents, Chat History, Jobs, Panel, Settings)
- Mobile agent list (card view of live sessions)
- Sticky command input at bottom
- Tablet sidebar as overlay with backdrop

### What's Broken
1. Agentic panel (files, tasks, notes, activity) hidden on mobile
2. No mobile-friendly diff/file preview
3. Team launch modal two-column layout breaks on narrow screens
4. Welcome team preset cards don't fit on phone
5. Terminal area has no pinch-to-zoom or text size adjustment
6. No swipe gestures for navigation

## Plan

### Phase 1: Quick Wins (CSS-only)
1. Stack welcome team cards vertically on mobile
2. Stack team modal columns on mobile
3. Make agentic panel show as full-screen overlay on mobile (Panel tab)
4. Larger touch targets for file list items and tab buttons

### Phase 2: Mobile-First Redesign
1. Mobile-first agentic panel (bottom sheet or full-screen tabs)
2. Swipe navigation between sessions
3. Mobile-optimized file preview (full-screen modal instead of sidebar pane)

### Phase 3: Mobile UX Polish
- TBD based on UI Engineer input

## Key Files
- `static/css/mobile.css` — mobile-specific styles
- `static/mobile.js` — mobile interactions (swipe, tab bar, agent list)
- `static/css/layout.css` — responsive layout breakpoints
