//go:build webview

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

// DragOverlayView is a transparent NSView placed over the titlebar region.
// It handles window dragging natively, bypassing WKWebView's event handling
// which breaks CSS -webkit-app-region: drag after the window gains focus.
@interface DragOverlayView : NSView
@end

@implementation DragOverlayView

- (BOOL)acceptsFirstMouse:(NSEvent *)event {
    return YES;
}

- (void)mouseDown:(NSEvent *)event {
    [self.window performWindowDragWithEvent:event];
}

- (void)mouseUp:(NSEvent *)event {
    // Check for double-click to toggle zoom (standard macOS behavior)
    if (event.clickCount == 2) {
        [self.window performZoom:nil];
    }
}

// Allow clicks to pass through to buttons underneath
- (NSView *)hitTest:(NSPoint)point {
    // Convert to window coordinates
    NSPoint windowPoint = [self convertPoint:point toView:nil];
    NSPoint screenPoint = [self.window convertPointToScreen:windowPoint];

    // Check if any button/control under this point should receive the click.
    // Traffic light buttons (close/minimize/zoom) are in the titlebar area
    // and handled by the window frame, not by this view.
    // For web-rendered buttons, we rely on the height of the drag region
    // being just the titlebar height (~37px), so buttons below it get clicks.
    return [super hitTest:point];
}

@end

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

// configureTitlebar sets up a transparent titlebar with full-size content view
// and adds a native drag overlay so window dragging works reliably in WKWebView.
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

        // Add native drag overlay above the webview for the titlebar region.
        // Height matches the CSS top-bar (~37px). The overlay handles drag
        // natively, which is more reliable than CSS -webkit-app-region in
        // embedded WKWebView.
        NSView *contentView = [window contentView];
        CGFloat dragHeight = 37.0;
        NSRect overlayFrame = NSMakeRect(0,
            contentView.bounds.size.height - dragHeight,
            contentView.bounds.size.width,
            dragHeight);

        DragOverlayView *overlay = [[DragOverlayView alloc] initWithFrame:overlayFrame];
        overlay.autoresizingMask = NSViewWidthSizable | NSViewMinYMargin;
        [contentView addSubview:overlay];

        NSLog(@"[TITLEBAR] titlebar + drag overlay configured (height=%.0f)", dragHeight);
    });
}
*/
import "C"

import "time"

// setupNativeTitlebar configures the macOS window for a transparent title bar
// with full-size content view and a native drag overlay. The drag overlay is
// a transparent NSView that handles window dragging natively, bypassing
// WKWebView's broken CSS -webkit-app-region: drag behavior.
func setupNativeTitlebar() {
	// Give the window time to appear before configuring
	go func() {
		time.Sleep(200 * time.Millisecond)
		C.configureTitlebar()
	}()
}
