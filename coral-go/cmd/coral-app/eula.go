//go:build webview

package main

import (
	"github.com/cdknorow/coral/internal/config"
	"github.com/cdknorow/coral/internal/license"
)

// checkAndShowEULA checks if the user has accepted the EULA.
// Skipped in dev tier (compile-time build tag).
func checkAndShowEULA() bool {
	if !config.EULARequired() {
		return true
	}
	return license.CheckAndPromptEULA(showEULADialog)
}
