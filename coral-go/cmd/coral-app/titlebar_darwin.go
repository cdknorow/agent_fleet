//go:build webview

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

// Drag state for title bar window dragging.
// WKWebView does not support -webkit-app-region: drag, so we use
// NSEvent local monitors to detect drags in the title bar region
// and call performWindowDragWithEvent: to move the window natively.
static NSEvent * __strong _titleBarMouseDown;
static BOOL _titleBarDragArmed;

static NSWindow *findAppWindow() {
    NSWindow *window = [[NSApplication sharedApplication] keyWindow];
    if (!window) window = [[NSApplication sharedApplication] mainWindow];
    if (!window) {
        for (NSWindow *w in [[NSApplication sharedApplication] windows]) {
            if ([w isVisible]) { window = w; break; }
        }
    }
    NSLog(@"[TITLEBAR] findAppWindow: %@", window ? @"found" : @"nil");
    return window;
}

static void setupWindowDrag(NSWindow *window) {
    NSLog(@"[TITLEBAR] setupWindowDrag: configuring window %@", window);
    window.titlebarAppearsTransparent = YES;
    window.titleVisibility = NSWindowTitleHidden;
    window.styleMask |= NSWindowStyleMaskFullSizeContentView;
    NSLog(@"[TITLEBAR] window style configured (transparent titlebar, full-size content)");

    // Height of the app's top bar that should be draggable.
    CGFloat dragZoneHeight = 42;

    // Arm drag when mouse goes down in the title bar region.
    // Double-click toggles zoom (maximize/restore), matching native macOS behavior.
    [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskLeftMouseDown handler:^NSEvent *(NSEvent *event) {
        @try {
            NSWindow *w = event.window;
            if (!w || !w.contentView) return event;
            // Only handle events on the key window — clicking a background
            // window to focus it should pass through without arming a drag.
            if (![w isKeyWindow]) return event;
            NSPoint loc = event.locationInWindow;
            CGFloat windowHeight = w.contentView.frame.size.height;
            if (loc.y > windowHeight - dragZoneHeight) {
                if (event.clickCount == 2) {
                    NSLog(@"[TITLEBAR] double-click detected at y=%.0f (zone=%.0f-%.0f)", loc.y, windowHeight - dragZoneHeight, windowHeight);
                    NSString *action = [[NSUserDefaults standardUserDefaults]
                        stringForKey:@"AppleActionOnDoubleClick"];
                    NSLog(@"[TITLEBAR] AppleActionOnDoubleClick=%@", action ?: @"(nil/zoom)");
                    if ([action isEqualToString:@"Minimize"]) {
                        [w miniaturize:nil];
                    } else {
                        [w zoom:nil];
                    }
                    _titleBarDragArmed = NO;
                    _titleBarMouseDown = nil;
                    return event;
                }
                _titleBarMouseDown = event;
                _titleBarDragArmed = YES;
            }
        } @catch (NSException *e) {
            NSLog(@"[TITLEBAR] EXCEPTION in mouseDown: %@ — %@", e.name, e.reason);
            _titleBarDragArmed = NO;
            _titleBarMouseDown = nil;
        }
        return event;
    }];

    // Once the mouse has moved more than 3px, initiate a native window drag.
    [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskLeftMouseDragged handler:^NSEvent *(NSEvent *event) {
        @try {
            if (_titleBarDragArmed && _titleBarMouseDown) {
                NSWindow *w = event.window;
                if (!w || !w.contentView || ![w isKeyWindow]) {
                    NSLog(@"[TITLEBAR] drag aborted: window nil/invalid/not key");
                    _titleBarDragArmed = NO;
                    _titleBarMouseDown = nil;
                    return event;
                }
                NSPoint cur   = event.locationInWindow;
                NSPoint start = _titleBarMouseDown.locationInWindow;
                CGFloat dx = cur.x - start.x;
                CGFloat dy = cur.y - start.y;
                if (dx * dx + dy * dy > 9) {
                    NSLog(@"[TITLEBAR] initiating window drag (dx=%.1f dy=%.1f)", dx, dy);
                    _titleBarDragArmed = NO;
                    NSEvent *dragEvent = _titleBarMouseDown;
                    _titleBarMouseDown = nil;
                    // Unzoom before dragging — performWindowDragWithEvent crashes
                    // with EXC_BAD_ACCESS on a zoomed/full-screen window.
                    if ([w isZoomed]) {
                        NSLog(@"[TITLEBAR] window is zoomed, unzooming first");
                        [w zoom:nil];
                    }
                    [w performWindowDragWithEvent:dragEvent];
                    NSLog(@"[TITLEBAR] performWindowDragWithEvent completed");
                }
            }
        } @catch (NSException *e) {
            NSLog(@"[TITLEBAR] EXCEPTION in mouseDragged: %@ — %@", e.name, e.reason);
            _titleBarDragArmed = NO;
            _titleBarMouseDown = nil;
        }
        return event;
    }];

    // Disarm on mouse up (was a click, not a drag — let WKWebView handle it).
    [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskLeftMouseUp handler:^NSEvent *(NSEvent *event) {
        _titleBarDragArmed = NO;
        _titleBarMouseDown = nil;
        return event;
    }];

    NSLog(@"[TITLEBAR] all event monitors installed");
}

void configureTitlebar() {
    NSLog(@"[TITLEBAR] configureTitlebar called");
    dispatch_async(dispatch_get_main_queue(), ^{
        NSWindow *window = findAppWindow();
        if (window) {
            NSLog(@"[TITLEBAR] window found on first attempt");
            setupWindowDrag(window);
        } else {
            NSLog(@"[TITLEBAR] window not found, retrying in 500ms");
            dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 500 * NSEC_PER_MSEC),
                           dispatch_get_main_queue(), ^{
                NSWindow *w = findAppWindow();
                if (w) {
                    NSLog(@"[TITLEBAR] window found on retry");
                    setupWindowDrag(w);
                } else {
                    NSLog(@"[TITLEBAR] ERROR: window still nil after retry — titlebar drag disabled");
                }
            });
        }
    });
}
*/
import "C"

import "time"

// setupNativeTitlebar configures the macOS window for a transparent title bar
// and installs native event monitors for window dragging (since WKWebView
// does not support the -webkit-app-region CSS property).
func setupNativeTitlebar() {
	// Give the window time to appear before configuring
	go func() {
		time.Sleep(200 * time.Millisecond)
		C.configureTitlebar()
	}()
}
