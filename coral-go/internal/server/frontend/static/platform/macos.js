/* macOS WKWebView workarounds.
   WKWebView has layout timing quirks that don't affect Chrome/Safari. */

import { platform } from './detect.js';

export function initMacOS() {
    if (!platform.isMacOS || !platform.isNative) return;
    // WKWebView-specific init applied here as needed.
}

// Force WKWebView to recalculate layout after display transitions.
// WKWebView computes 0 height for overflow:hidden containers when
// parent transitions from display:none to display:flex.
export function forceReflow(el) {
    if (!platform.capabilities.layoutOnDisplay) {
        requestAnimationFrame(() => { void el.offsetHeight; });
    }
}
