# Frontend App Separation Spec

## Problem

Coral's frontend serves three distinct targets from a single codebase:

1. **Browser** — Chrome, Safari, Firefox accessing `http://localhost:8420`
2. **Native macOS** — WKWebView embedded in `coral-app`
3. **Native Windows** — WebView2 embedded in `coral-app` (planned)

Each target has different webview engines, layout quirks, and platform conventions,
but currently all share identical JS/CSS with differences handled by:

- Ad-hoc JS injection in `coral-app/main.go` (lines 139-218)
- A `window.__CORAL_APP__` flag checked sporadically in JS
- CSS classes (`.native-app`, `.native-macos`) for layout tweaks
- Scattered workarounds for WebKit quirks (comments in xterm_renderer.js, capture.js)

This has caused multiple bugs where features work in the browser but break in the
native app:

| Bug | Root cause |
|-----|-----------|
| "Server Disconnected" overlay | `/api/health` polled by injected JS, endpoint missing |
| Blank agentic-state panel | WKWebView layout timing collapses overflow:hidden panels |
| Terminal fit issues | ResizeObserver doesn't fire on display transitions |
| Input events missing | WebKit doesn't fire `input` events reliably |

Each fix is currently a one-off workaround discovered in production.

## Goals

1. **Isolate platform-specific behavior** so browser, macOS, and Windows bugs don't cross-contaminate
2. **Make webview engine quirks explicit** instead of hidden in scattered comments
3. **Share core logic** — don't duplicate the 90% that's identical across all targets
4. **Maintain single build** — keep the embedded FS approach (`//go:embed`)
5. **No frameworks** — stay vanilla JS, keep the ES module architecture
6. **Support multiple native platforms** — architecture must scale to macOS + Windows (and future platforms) without per-platform forks

## Non-Goals

- Rewriting the frontend in a framework (React, Vue, etc.)
- Separate build pipelines per target
- Separate HTML templates per target

---

## Current Architecture

```
frontend/
  static/
    app.js              ← single entry point, 38 modules imported
    state.js            ← shared global state
    css/
      style.css         ← imports 14 CSS files in cascade order
      layout.css        ← contains .native-app overrides (lines 760-782)
      ...
    vendor/             ← xterm, codemirror, marked, etc.
  templates/
    index.html          ← Go template, single SPA shell
    includes/           ← sidebar, modals, views

coral-app/main.go       ← injects ~80 lines of JS into webview at startup
```

**How native detection works today:**
1. `coral-app` injects `window.__CORAL_APP__ = true` + body classes
2. `app.js` line 523 checks `navigator.userAgent.includes('CoralApp')` as fallback
3. CSS uses `.native-app` and `.native-macos` selectors in `layout.css`
4. Individual modules check `document.hidden` or `window.__CORAL_APP__` ad-hoc

---

## Proposed Architecture

```
frontend/
  static/
    app.js              ← shared entry point (unchanged)
    state.js            ← shared state (unchanged)
    platform/
      detect.js         ← platform detection + capability flags
      browser.js        ← browser-specific init
      native.js         ← shared native init (health check, link intercept)
      macos.js          ← macOS-specific (WKWebView layout workarounds)
      windows.js        ← Windows-specific (WebView2 quirks, future)
    css/
      style.css         ← shared base styles (unchanged)
      layout.css        ← remove platform rules from here
      native.css        ← shared native CSS (health overlay, window chrome)
      native-macos.css  ← macOS-specific CSS (traffic lights, WKWebView fixes)
      native-windows.css← Windows-specific CSS (titlebar, WebView2 fixes)
      browser.css       ← browser-only CSS (if needed, likely minimal)
    ... (all other modules unchanged)
```

### Layer 1: Platform Detection (`platform/detect.js`)

Single source of truth for runtime environment. Replaces scattered `__CORAL_APP__`
checks and `navigator.userAgent` sniffing.

```js
// platform/detect.js
export const platform = {
    isNative:  false,
    isBrowser: true,
    isMacOS:   false,
    isWindows: false,
    isLinux:   false,

    // Webview engine (set during init)
    // 'browser' | 'wkwebview' | 'webview2'
    engine: 'browser',

    // Capability flags — what the runtime supports reliably.
    // Modules check capabilities, not platform names. This way a fix for
    // WKWebView automatically applies if WebView2 has the same issue.
    capabilities: {
        resizeObserver:   true,   // false in WKWebView (some versions)
        inputEvents:      true,   // false in WKWebView (use keyup fallback)
        layoutOnDisplay:  true,   // false in WKWebView (needs rAF reflow)
        externalLinks:    true,   // false in all native (intercept → system browser)
        nativeTitlebar:   false,  // true in all native (drag regions, traffic lights)
    },

    init() {
        this.isNative  = !!window.__CORAL_APP__;
        this.isBrowser = !this.isNative;
        this.isMacOS   = navigator.platform?.includes('Mac');
        this.isWindows = navigator.platform?.includes('Win');
        this.isLinux   = navigator.platform?.includes('Linux');

        if (this.isNative) {
            this.capabilities.externalLinks  = false;
            this.capabilities.nativeTitlebar = true;

            if (this.isMacOS) {
                this.engine = 'wkwebview';
                this.capabilities.inputEvents    = false;
                this.capabilities.layoutOnDisplay = false;
                this.capabilities.resizeObserver  = false;
            }
            if (this.isWindows) {
                this.engine = 'webview2';
                // Windows WebView2 capabilities set here as discovered.
                // WebView2 is Chromium-based, so most things work — but
                // test and add flags as issues are found.
            }
        }
    }
};
```

**Usage in modules:**
```js
// Before (scattered):
if (window.__CORAL_APP__) { ... }
if (navigator.userAgent.includes('CoralApp')) { ... }

// After (centralized):
import { platform } from './platform/detect.js';

// Check WHAT works, not WHERE we are:
if (!platform.capabilities.layoutOnDisplay) { forceReflow(el); }
if (!platform.capabilities.externalLinks)   { interceptLink(el); }
if (platform.capabilities.nativeTitlebar)   { setupDragRegion(el); }

// When you truly need to know the platform (rare):
if (platform.isMacOS && platform.isNative)  { /* macOS-only native logic */ }
```

### Layer 2: App-Specific Initialization

Move injected JS from `coral-app/main.go` into proper modules that are
conditionally loaded.

**`platform/native.js`** — shared native init (all platforms):
```js
import { platform } from './detect.js';

export function initNative() {
    if (!platform.isNative) return;

    document.body.classList.add('native-app');
    if (platform.isMacOS)   document.body.classList.add('native-macos');
    if (platform.isWindows) document.body.classList.add('native-windows');
    if (platform.isLinux)   document.body.classList.add('native-linux');

    initHealthCheck();
    initLinkInterceptor();
}

function initHealthCheck() { /* move from coral-app/main.go lines 157-218 */ }
function initLinkInterceptor() { /* move from coral-app/main.go lines 139-155 */ }
```

**`platform/macos.js`** — macOS WKWebView workarounds:
```js
import { platform } from './detect.js';

export function initMacOS() {
    if (!platform.isMacOS || !platform.isNative) return;
    // WKWebView-specific fixes applied here
}

// Force WKWebView to recalculate layout after display transitions.
// WKWebView computes 0 height for overflow:hidden containers when
// parent transitions from display:none → display:flex.
export function forceReflow(el) {
    if (!platform.capabilities.layoutOnDisplay) {
        requestAnimationFrame(() => { void el.offsetHeight; });
    }
}

// Ensure overflow:hidden containers don't collapse to 0 height
export function ensureMinHeight(el, fallback = '100px') {
    if (!platform.capabilities.layoutOnDisplay) {
        if (el.offsetHeight === 0) {
            el.style.minHeight = fallback;
        }
    }
}
```

**`platform/windows.js`** — Windows WebView2 workarounds:
```js
import { platform } from './detect.js';

export function initWindows() {
    if (!platform.isWindows || !platform.isNative) return;
    // WebView2-specific fixes applied here as discovered.
    // WebView2 is Chromium-based so fewer quirks expected, but
    // Windows has its own titlebar conventions, font rendering
    // differences, and scroll behavior.
}
```

**`platform/browser.js`** — browser-specific init:
```js
import { platform } from './detect.js';

export function initBrowser() {
    if (!platform.isBrowser) return;
    // PWA service worker registration, etc.
}
```

### Layer 3: App-Specific CSS

**`css/native.css`** — shared native styles (all platforms):
```css
/* Window chrome — common to all native apps */
.native-app .top-bar {
    -webkit-app-region: drag;
    padding: 6px 12px;
}
.native-app .top-bar-title { display: none; }
.native-app .top-bar-btn,
.native-app .top-bar-actions { -webkit-app-region: no-drag; }

/* Health check overlay */
.native-app #coral-app-disconnect-overlay { /* styles from injected JS */ }
```

**`css/native-macos.css`** — macOS-specific styles:
```css
/* Traffic light button spacing */
.native-macos .top-bar { padding-left: 78px; }

/* WKWebView layout fixes — prevent overflow:hidden collapse */
.native-macos .agentic-state { min-height: 100px; }
.native-macos .agentic-panel { min-height: 50px; }
```

**`css/native-windows.css`** — Windows-specific styles:
```css
/* Windows titlebar conventions */
.native-windows .top-bar { padding-right: 140px; } /* space for min/max/close */

/* WebView2 font rendering — Windows ClearType needs explicit smoothing */
.native-windows body { -webkit-font-smoothing: auto; }
```

**`css/browser.css`** — browser-only styles (if needed):
```css
/* Currently empty — browser is the baseline. Add here if browser needs
   styles that would break a native app. */
```

### Layer 4: Integration in app.js

Minimal changes to the entry point:

```js
// app.js — add near top
import { platform } from './platform/detect.js';
import { initNative } from './platform/native.js';
import { initMacOS } from './platform/macos.js';
import { initWindows } from './platform/windows.js';
import { initBrowser } from './platform/browser.js';

// In DOMContentLoaded:
platform.init();

if (platform.isNative) {
    initNative();                           // shared native setup
    if (platform.isMacOS)   initMacOS();    // WKWebView fixes
    if (platform.isWindows) initWindows();  // WebView2 fixes
} else {
    initBrowser();
}
```

CSS loading in `index.html`:
```html
<link rel="stylesheet" href="/static/style.css">
<!-- Platform-specific CSS — loaded for all targets, scoped by class selectors -->
<link rel="stylesheet" href="/static/css/native.css">
<link rel="stylesheet" href="/static/css/native-macos.css">
<link rel="stylesheet" href="/static/css/native-windows.css">
```

All platform CSS files are loaded for all targets. Rules are scoped under
`.native-app`, `.native-macos`, `.native-windows` class selectors so they have
no effect on other platforms. This avoids conditional loading complexity and
keeps the `//go:embed` approach simple.

### Layer 5: Reduce coral-app/main.go Injection

After moving logic to `platform/native.js`, the injected JS in `main.go` shrinks to:

```js
// Minimal injection — just set the flag before modules load
window.__CORAL_APP__ = true;
```

Everything else (health check, link intercept, body classes) moves to
`platform/native.js` which runs on DOMContentLoaded via `app.js`.

---

## Migration Plan

### Phase 1: Create platform layer
1. Create `platform/detect.js` with capability flags and engine detection
2. Create `platform/native.js` — move health check + link intercept from main.go
3. Create `platform/macos.js` — WKWebView reflow helpers
4. Create `platform/windows.js` — WebView2 stub (populated as issues found)
5. Create `platform/browser.js` — browser-specific init
6. Update `app.js` to import and init platform layer
7. Reduce `coral-app/main.go` injection to just `window.__CORAL_APP__ = true`

### Phase 2: Consolidate CSS
1. Create `css/native.css` — shared native rules (window chrome, health overlay)
2. Create `css/native-macos.css` — macOS rules (traffic lights, WKWebView fixes)
3. Create `css/native-windows.css` — Windows rules (titlebar, font rendering)
4. Remove `.native-app` / `.native-macos` rules from `layout.css`
5. Add all three to `index.html` (loaded for all targets, scoped by class)

### Phase 3: Fix existing WebKit bugs
1. Add `forceReflow()` call in `showView()` for agentic-state panel
2. Add `resp.ok` checks to `loadAgentEvents`, `loadChangedFiles`
3. Replace ad-hoc `__CORAL_APP__` checks with `platform.capabilities.*` imports

### Phase 4: Adopt in new features
1. New webview-sensitive code checks `platform.capabilities.*`
2. New native-only features go in `platform/native.js` (shared) or `platform/<os>.js`
3. New native-only styles go in `css/native.css` or `css/native-<os>.css`

---

## Decision Log

| Decision | Rationale |
|----------|-----------|
| Per-platform CSS files, scoped by class | Each platform's quirks isolated; easy to add new platforms |
| Capability flags over platform sniffing | Modules check *what works*, not *what platform*. If WKWebView and WebView2 share a quirk, one flag covers both |
| No separate HTML templates | 99% of HTML is shared; differences are CSS/JS only |
| Keep injected JS minimal (just the flag) | Move logic to proper modules for testability and debuggability |
| No build-time splitting | Keep single `//go:embed` FS; runtime detection is sufficient |
| `platform/` subdirectory | Groups related files; scales to N platforms without clutter |
| Per-platform JS files (`macos.js`, `windows.js`) | Each platform's workarounds in one file, easy to find and audit |
| `engine` field in detect.js | Distinguishes WKWebView vs WebView2 vs browser for edge cases where capability flags aren't granular enough |

## Files Changed

| File | Change |
|------|--------|
| `static/platform/detect.js` | NEW — platform detection, engine ID, capability flags |
| `static/platform/native.js` | NEW — shared native init (health check, link intercept, body classes) |
| `static/platform/macos.js` | NEW — macOS WKWebView layout workarounds |
| `static/platform/windows.js` | NEW — Windows WebView2 quirks (stub, populated as discovered) |
| `static/platform/browser.js` | NEW — browser-specific init |
| `static/css/native.css` | NEW — shared native CSS (window chrome, health overlay) |
| `static/css/native-macos.css` | NEW — macOS-specific CSS (traffic lights, WKWebView fixes) |
| `static/css/native-windows.css` | NEW — Windows-specific CSS (titlebar, font rendering) |
| `static/app.js` | MODIFY — import + init platform layer |
| `static/css/layout.css` | MODIFY — remove `.native-app` / `.native-macos` rules |
| `cmd/coral-app/main.go` | MODIFY — reduce injected JS to flag-only |
| `scripts/bundle-frontend.sh` | MODIFY — include `platform/` dir in esbuild bundle |
