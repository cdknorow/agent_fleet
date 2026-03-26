//go:build webview

package main

import "github.com/cdknorow/coral/internal/license"

// checkAndShowEULA checks if the user has accepted the EULA.
// Delegates to the shared license package with a platform-specific dialog.
func checkAndShowEULA() bool {
	return license.CheckAndPromptEULA(showEULADialog)
}
