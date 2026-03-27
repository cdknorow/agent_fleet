//go:build webview

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

// showInDock sets the app to appear in the macOS Dock as a regular application
// and loads the app icon from the bundle's Resources directory.
void showInDock() {
    dispatch_async(dispatch_get_main_queue(), ^{
        NSLog(@"[DOCK] showInDock: setting activation policy");
        NSApplication *app = [NSApplication sharedApplication];
        [app setActivationPolicy:NSApplicationActivationPolicyRegular];
        NSLog(@"[DOCK] activation policy set to Regular");

        // Load the app icon from the bundle
        NSBundle *bundle = [NSBundle mainBundle];
        NSString *iconPath = [bundle pathForResource:@"AppIcon" ofType:@"icns"];
        NSLog(@"[DOCK] icon path: %@", iconPath ?: @"(not found)");
        if (iconPath) {
            NSImage *icon = [[NSImage alloc] initWithContentsOfFile:iconPath];
            if (icon) {
                [app setApplicationIconImage:icon];
                NSLog(@"[DOCK] app icon set");
            }
        }
    });
}

// raiseWindow brings the app window to front (called when Dock icon is clicked).
void raiseWindow() {
    dispatch_async(dispatch_get_main_queue(), ^{
        NSWindow *window = [[NSApplication sharedApplication] keyWindow];
        if (!window) {
            NSArray *windows = [[NSApplication sharedApplication] windows];
            for (NSWindow *w in windows) {
                if ([w isVisible]) {
                    window = w;
                    break;
                }
            }
        }
        if (window) {
            [window makeKeyAndOrderFront:nil];
        }
        [NSApp activateIgnoringOtherApps:YES];
    });
}
*/
import "C"

func showInDock() {
	C.showInDock()
}

// dockRaiseWindow brings the app window to front.
func dockRaiseWindow() {
	C.raiseWindow()
}
