# Native Titlebar Drag

## Problem

The macOS native app (coral-app) uses a transparent titlebar with
`NSWindowStyleMaskFullSizeContentView` so web content extends edge-to-edge.
Window dragging was previously handled by CSS `-webkit-app-region: drag` on
the top bar element.

This approach was removed during a platform CSS refactor and proved unreliable
when re-added:

1. **First-click-only drag:** CSS drag regions work on the first click (when
   the window gains focus) but stop working once the WKWebView captures focus.
   Subsequent drag attempts are consumed by the webview's event handler.

2. **Timing sensitivity:** The CSS rule requires `.native-app` class to be
   present before the first layout pass. Applying it via DOMContentLoaded
   is often too late. Applying on `<html>` synchronously via `w.Init()` helps
   but doesn't fix the focus issue.

3. **webview_go limitation:** The `webview_go` library doesn't forward
   `-webkit-app-region` events to the native window system after the webview
   has focus. This is a known limitation of embedded WKWebView.

## Decision

Replace CSS-based drag with a **native NSEvent local monitor** that intercepts
mouse events at the application dispatch level before they reach any view,
including WKWebView's internal subviews.

### Why native over CSS

| Approach | Pros | Cons |
|----------|------|------|
| CSS `-webkit-app-region: drag` | No native code, works in browsers | Unreliable in WKWebView after focus, timing-dependent |
| Native `NSView` overlay | Works initially | WKWebView re-orders subviews after gaining focus, breaking the overlay |
| **NSEvent local monitor** | **Always works, bypasses view hierarchy entirely, native feel** | Requires Cocoa code, top N pixels are non-interactive for web content |

### Why not a subview overlay (DragOverlayView)

The original fix used a transparent `DragOverlayView` (NSView subclass) added
on top of the WKWebView's contentView. This worked on the first click but
failed after WKWebView gained focus — WKWebView dynamically creates internal
subviews (WKCompositingView, etc.) that end up above the overlay in the z-order,
preventing the overlay's `hitTest:` from being called.

### How Electron solves this

Electron uses a native drag handler, not CSS. Their implementation intercepts
mouse events at the native level and forwards them to the window system. Our
NSEvent local monitor approach is equivalent.

## Implementation

**File:** `cmd/coral-app/titlebar_darwin.go`

An `NSEvent` local monitor intercepts `NSLeftMouseDown` events at the
application level and uses a tracking loop to distinguish between clicks and
drags in the titlebar region:

```objc
[NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskLeftMouseDown
    handler:^NSEvent *(NSEvent *event) {
        // ... check if in drag region ...

        // Tracking loop: wait for drag or click release
        while (YES) {
            NSEvent *nextEvent = [window nextEventMatchingMask:trackMask ...];
            if (nextEvent.type == NSEventTypeLeftMouseUp) {
                // Click — pass through to WKWebView (buttons work)
                [window sendEvent:event];
                [window sendEvent:nextEvent];
                return nil;
            }
            if (nextEvent.type == NSEventTypeLeftMouseDragged) {
                // Drag beyond threshold — initiate window drag
                [window performWindowDragWithEvent:event];
                return nil;
            }
        }
    }];
```

**Key design points:**
- **Click vs drag disambiguation:** On mouseDown in the drag region, a tracking
  loop waits for either mouseDragged (drag) or mouseUp (click). Clicks pass
  through to WKWebView so all buttons in the top bar remain clickable. Only
  actual drag gestures initiate window movement. This is the standard macOS
  pattern used by NSTableView, Finder, and Electron.
- **Event monitor vs subview:** The monitor fires at the NSApplication dispatch
  level, before any view receives the event. This completely bypasses WKWebView's
  internal view hierarchy, which was the root cause of the subview overlay failure.
- **No retained references:** The block uses `event.window` (transient) rather
  than capturing a window variable, avoiding retain cycles.
- **Main thread dispatch:** The monitor is installed via `dispatch_async` to the
  main queue for thread safety.

**Configuration:**
- Drag region height: 37px (`kDragRegionHeight`, matches the top bar)
- Drag threshold: 3px (`kDragThreshold`, minimum mouse movement to start drag)
- `setMovableByWindowBackground:NO` prevents conflicts with native drag
- Window retains `titlebarAppearsTransparent` and `fullSizeContentView`
- Double-click in titlebar triggers `performZoom:` (standard macOS behavior)

**No exclusion zones needed:** Unlike the previous implementation which required
hardcoded exclusion zones for traffic light buttons and action buttons, the
click-vs-drag approach lets ALL clicks pass through to the webview. Only drag
gestures are intercepted. This eliminates the need for coordinate-based exclusions
and works correctly regardless of button positions or window width.

## History

1. **Original:** CSS drag in `layout.css`, classes on `<body>` via
   `w.Init()` DOMContentLoaded — worked but was removed in CSS refactor
2. **Refactor:** CSS moved to `native.css`, classes on `<html>` synchronous —
   broke (first-click-only issue discovered)
3. **Overlay:** Native Cocoa `DragOverlayView` (NSView subclass) — worked on
   first click but failed after WKWebView re-ordered its internal subviews
4. **Final:** NSEvent local monitor — reliable, bypasses view hierarchy entirely
