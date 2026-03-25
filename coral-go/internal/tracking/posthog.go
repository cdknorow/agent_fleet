// Package tracking provides anonymous install/upgrade tracking via PostHog.
// All tracking is non-blocking, fire-and-forget, and never affects app behavior.
package tracking

import (
	"bytes"
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/cdknorow/coral/internal/config"
)

const posthogURL = "https://us.i.posthog.com/capture/"

var (
	cachedInstallID string
	installIDOnce   sync.Once
)

// getInstallID returns the install ID, reading from disk once and caching.
func getInstallID() string {
	installIDOnce.Do(func() {
		idFile := filepath.Join(homeDir(), ".coral", ".install_id")
		cachedInstallID = readFile(idFile)
	})
	return cachedInstallID
}

// TrackInstallAsync checks for first install or version upgrade and sends
// an event to PostHog. Also sends an 'app_opened' heartbeat for DAU.
// Runs in a goroutine, never blocks.
func TrackInstallAsync() {
	if config.PostHogKey == "" {
		return
	}
	go func() {
		defer func() { recover() }()
		trackInstall()
		// Always send app_opened for DAU tracking
		TrackEvent("app_opened", nil)
	}()
}

// TrackEvent sends a named event to PostHog with optional extra properties.
// Non-blocking — runs in a goroutine. Safe to call from any context.
func TrackEvent(eventName string, extraProps map[string]string) {
	if config.PostHogKey == "" {
		return
	}
	go func() {
		defer func() { recover() }()
		id := getInstallID()
		if id == "" {
			return
		}
		props := map[string]any{
			"version": config.Version,
			"edition": config.Edition,
			"os":      runtime.GOOS,
			"arch":    runtime.GOARCH,
		}
		for k, v := range extraProps {
			props[k] = v
		}
		postEvent(eventName, id, props)
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
		// Update cache
		cachedInstallID = installID
		postEvent("install", installID, map[string]any{
			"version": currentVersion,
			"edition": config.Edition,
			"os":      runtime.GOOS,
			"arch":    runtime.GOARCH,
		})
		return
	}

	// Update cache
	cachedInstallID = installID

	if currentVersion != "" && storedVersion != currentVersion {
		// Version upgrade
		os.WriteFile(versionFile, []byte(currentVersion), 0600)
		postEvent("upgrade", installID, map[string]any{
			"version": currentVersion,
			"edition": config.Edition,
			"os":      runtime.GOOS,
			"arch":    runtime.GOARCH,
		})
	}
}

func postEvent(event, distinctID string, properties map[string]any) {
	payload := map[string]any{
		"api_key":     config.PostHogKey,
		"event":       event,
		"distinct_id": distinctID,
		"properties":  properties,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(posthogURL, "application/json", bytes.NewReader(data))
	if err != nil {
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
