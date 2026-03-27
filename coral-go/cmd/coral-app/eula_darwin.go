//go:build webview && darwin

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

// cocoaShowEULA displays a modal EULA acceptance dialog with a scrollable
// text view. Returns 1 if the user clicks Accept, 0 if they click Decline.
int cocoaShowEULA(const char *tosText) {
	// Run directly — this is called from main() which is already on the main
	// thread (goroutine 1 locked to OS thread 0 via init()/runtime.LockOSThread()).
	// Using dispatch_sync to the main queue from the main thread would DEADLOCK.
	NSString *text = [NSString stringWithUTF8String:tosText];

	NSAlert *alert = [[NSAlert alloc] init];
	alert.messageText = @"Coral — Terms of Service";
	alert.informativeText = @"Please read and accept the Terms of Service to continue.";
	alert.alertStyle = NSAlertStyleInformational;
	[alert addButtonWithTitle:@"Accept"];
	[alert addButtonWithTitle:@"Decline"];

	// Scrollable text view for the TOS content
	NSScrollView *scrollView = [[NSScrollView alloc] initWithFrame:NSMakeRect(0, 0, 500, 300)];
	scrollView.hasVerticalScroller = YES;
	scrollView.borderType = NSBezelBorder;

	NSTextView *textView = [[NSTextView alloc] initWithFrame:NSMakeRect(0, 0, 480, 300)];
	textView.editable = NO;
	textView.selectable = YES;
	textView.font = [NSFont systemFontOfSize:12];
	textView.textColor = [NSColor labelColor];
	textView.backgroundColor = [NSColor textBackgroundColor];
	[textView setString:text];
	textView.autoresizingMask = NSViewWidthSizable;

	scrollView.documentView = textView;
	alert.accessoryView = scrollView;

	[alert layout];

	NSModalResponse response = [alert runModal];
	return (response == NSAlertFirstButtonReturn) ? 1 : 0;
}
*/
import "C"

import "unsafe"

// showEULADialog shows a native Cocoa dialog with the TOS text.
// Returns true if the user accepts, false if they decline.
func showEULADialog(tosText string) bool {
	cText := C.CString(tosText)
	defer C.free(unsafe.Pointer(cText))
	return C.cocoaShowEULA(cText) == 1
}
