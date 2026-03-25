// Package tracking provides anonymous install/upgrade tracking via PostHog.
// All tracking is non-blocking, fire-and-forget, and never affects app behavior.
package tracking

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	crand "crypto/rand"
	"fmt"

	"github.com/cdknorow/coral/internal/config"
)

const posthogURL = "https://us.i.posthog.com/capture/"

// TrackInstallAsync checks for first install or version upgrade and sends
// an event to PostHog. Runs in a goroutine, never blocks.
func TrackInstallAsync() {
	if config.PostHogKey == "" {
		return
	}
	go func() {
		defer func() { recover() }() // never crash the app
		trackInstall()
	}()
}

func trackInstall() {
	coralDir := filepath.Join(homeDir(), ".coral")
	os.MkdirAll(coralDir, 0755)

	idFile := filepath.Join(coralDir, ".install_id")
	versionFile := filepath.Join(coralDir, ".install_version")

	installID := readFile(idFile)
	storedVersion := readFile(versionFile)
	currentVersion := config.Version

	if installID == "" {
		// New install
		installID = generateUUID()
		os.WriteFile(idFile, []byte(installID), 0600)
		os.WriteFile(versionFile, []byte(currentVersion), 0600)
		sendEvent("install", installID, currentVersion)
		return
	}

	if currentVersion != "" && storedVersion != currentVersion {
		// Version upgrade
		os.WriteFile(versionFile, []byte(currentVersion), 0600)
		sendEvent("upgrade", installID, currentVersion)
	}
}

func sendEvent(event, distinctID, version string) {
	payload := map[string]any{
		"api_key":     config.PostHogKey,
		"event":       event,
		"distinct_id": distinctID,
		"properties": map[string]any{
			"version": version,
			"edition": config.Edition,
			"os":      runtime.GOOS,
			"arch":    runtime.GOARCH,
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(posthogURL, "application/json", bytes.NewReader(data))
	if err != nil {
		log.Printf("[tracking] %s event failed: %v", event, err)
		return
	}
	resp.Body.Close()
}

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

func generateUUID() string {
	b := make([]byte, 16)
	crand.Read(b)
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
