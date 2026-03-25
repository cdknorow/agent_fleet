//go:build webview

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

// configureTitlebar sets up a transparent titlebar with full-size content view.
// Window dragging is handled by CSS (-webkit-app-region: drag) in layout.css,
// which WKWebView supports on macOS 13+.
void configureTitlebar() {
    NSLog(@"[TITLEBAR] configureTitlebar called");
    dispatch_async(dispatch_get_main_queue(), ^{
        // Find the app window
        NSWindow *window = [[NSApplication sharedApplication] keyWindow];
        if (!window) window = [[NSApplication sharedApplication] mainWindow];
        if (!window) {
            for (NSWindow *w in [[NSApplication sharedApplication] windows]) {
                if ([w isVisible]) { window = w; break; }
            }
        }

        if (!window) {
            NSLog(@"[TITLEBAR] window not found, retrying in 500ms");
            dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 500 * NSEC_PER_MSEC),
                           dispatch_get_main_queue(), ^{
                NSWindow *w = [[NSApplication sharedApplication] keyWindow];
                if (!w) w = [[NSApplication sharedApplication] mainWindow];
                if (!w) {
                    for (NSWindow *win in [[NSApplication sharedApplication] windows]) {
                        if ([win isVisible]) { w = win; break; }
                    }
                }
                if (w) {
                    NSLog(@"[TITLEBAR] window found on retry, configuring");
                    w.titlebarAppearsTransparent = YES;
                    w.titleVisibility = NSWindowTitleHidden;
                    w.styleMask |= NSWindowStyleMaskFullSizeContentView;
                    NSLog(@"[TITLEBAR] titlebar configured");
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
        NSLog(@"[TITLEBAR] titlebar configured");
    });
}
*/
import "C"

import "time"

// setupNativeTitlebar configures the macOS window for a transparent title bar
// with full-size content view. Window dragging is handled by the CSS
// -webkit-app-region: drag property in layout.css (supported on macOS 13+).
func setupNativeTitlebar() {
	// Give the window time to appear before configuring
	go func() {
		time.Sleep(200 * time.Millisecond)
		C.configureTitlebar()
	}()
}
