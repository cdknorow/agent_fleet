//go:build webview

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

// Drag region height in points — matches the CSS top-bar height (~37px).
static const CGFloat kDragRegionHeight = 37.0;

// Minimum mouse movement (in points) to distinguish a drag from a click.
static const CGFloat kDragThreshold = 3.0;

static NSWindow* findAppWindow() {
    NSWindow *window = [[NSApplication sharedApplication] keyWindow];
    if (!window) window = [[NSApplication sharedApplication] mainWindow];
    if (!window) {
        for (NSWindow *w in [[NSApplication sharedApplication] windows]) {
            if ([w isVisible]) { window = w; break; }
        }
    }
    return window;
}

// Returns YES if the point (in window coordinates) is within the titlebar
// drag region.
static BOOL isInDragRegion(NSWindow *window, NSPoint windowPoint) {
    NSView *contentView = [window contentView];
    if (!contentView) return NO;

    CGFloat contentHeight = contentView.bounds.size.height;
    // NSWindow uses bottom-left origin, so the drag region is y > contentHeight - kDragRegionHeight.
    CGFloat minY = contentHeight - kDragRegionHeight;

    return windowPoint.y >= minY;
}

// installTitlebarDragMonitor sets up three local event monitors that
// distinguish between clicks and drags in the titlebar region:
//
// 1. mouseDown: If in the drag region, save the event and let it pass through
//    to WKWebView (so buttons remain clickable). Double-clicks trigger zoom.
// 2. mouseDragged: If a drag-region mouseDown was saved and the mouse has
//    moved beyond the threshold, initiate window drag.
// 3. mouseUp: Clear the saved mouseDown (click completed without dragging).
//
// This avoids reentrancy issues with nested event loops and follows the
// standard macOS pattern for click-vs-drag disambiguation.
void installTitlebarDragMonitor() {
    dispatch_async(dispatch_get_main_queue(), ^{
        // Shared state between the three monitors.
        __block NSEvent *savedMouseDown = nil;
        __block NSPoint savedStartPoint = NSZeroPoint;

        // Monitor 1: mouseDown — save if in drag region, pass through
        [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskLeftMouseDown
            handler:^NSEvent *(NSEvent *event) {
                savedMouseDown = nil; // clear any stale state

                NSWindow *window = event.window;
                if (!window) return event;

                if (!isInDragRegion(window, event.locationInWindow)) return event;

                if (event.clickCount == 2) {
                    [window performZoom:nil];
                    return nil; // consume double-click
                }

                // Save the mouseDown for potential drag initiation.
                // Return the event so WKWebView receives it (buttons work).
                savedMouseDown = event;
                savedStartPoint = event.locationInWindow;
                return event;
            }];

        // Monitor 2: mouseDragged — if saved mouseDown was in drag region,
        // check threshold and initiate window drag
        [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskLeftMouseDragged
            handler:^NSEvent *(NSEvent *event) {
                if (!savedMouseDown) return event;

                NSPoint dragPoint = event.locationInWindow;
                CGFloat dx = dragPoint.x - savedStartPoint.x;
                CGFloat dy = dragPoint.y - savedStartPoint.y;

                if (dx * dx + dy * dy > kDragThreshold * kDragThreshold) {
                    // User dragged beyond threshold — initiate window drag.
                    // performWindowDragWithEvent: takes over the run loop until
                    // the mouse is released. Pass the original mouseDown event
                    // as the drag anchor.
                    NSEvent *mouseDown = savedMouseDown;
                    savedMouseDown = nil;
                    [mouseDown.window performWindowDragWithEvent:mouseDown];
                    return nil; // consume the drag event
                }

                return event;
            }];

        // Monitor 3: mouseUp — clear saved mouseDown (click completed)
        [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskLeftMouseUp
            handler:^NSEvent *(NSEvent *event) {
                savedMouseDown = nil;
                return event;
            }];

        NSLog(@"[TITLEBAR] drag monitors installed (height=%.0f, threshold=%.0f)",
              kDragRegionHeight, kDragThreshold);
    });
}

// configureTitlebar sets up a transparent titlebar with full-size content view.
void configureTitlebar() {
    NSLog(@"[TITLEBAR] configureTitlebar called");
    dispatch_async(dispatch_get_main_queue(), ^{
        NSWindow *window = findAppWindow();

        if (!window) {
            NSLog(@"[TITLEBAR] window not found, retrying in 500ms");
            dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 500 * NSEC_PER_MSEC),
                           dispatch_get_main_queue(), ^{
                NSWindow *w = findAppWindow();
                if (w) {
                    NSLog(@"[TITLEBAR] window found on retry, configuring");
                    w.titlebarAppearsTransparent = YES;
                    w.titleVisibility = NSWindowTitleHidden;
                    w.styleMask |= NSWindowStyleMaskFullSizeContentView;
                    [w setMovableByWindowBackground:NO];
                    NSLog(@"[TITLEBAR] titlebar configured on retry");
                } else {
                    NSLog(@"[TITLEBAR] ERROR: window still nil after retry");
                }
            });
            return;
        }

        NSLog(@"[TITLEBAR] window found, configuring");
        window.titlebarAppearsTransparent = YES;
        window.titleVisibility = NSWindowTitleHidden;
        window.styleMask |= NSWindowStyleMaskFullSizeContentView;
        [window setMovableByWindowBackground:NO];

        NSLog(@"[TITLEBAR] titlebar configured");
    });
}
*/
import "C"

import "time"

// setupNativeTitlebar configures the macOS window for a transparent title bar
// with full-size content view, and installs a local event monitor that
// intercepts mouse events in the titlebar region for native window dragging.
//
// The event monitor approach is used instead of a subview overlay because
// WKWebView rearranges its internal view hierarchy after gaining focus,
// which causes overlay subviews to stop receiving events. The local event
// monitor operates at the NSApplication event-dispatch level, before events
// reach any view, so it works reliably regardless of WKWebView's state.
func setupNativeTitlebar() {
	// Give the window time to appear before configuring
	go func() {
		time.Sleep(200 * time.Millisecond)
		C.configureTitlebar()
		C.installTitlebarDragMonitor()
	}()
}
