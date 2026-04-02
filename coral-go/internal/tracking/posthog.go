// Package tracking provides anonymous install/upgrade tracking via PostHog.
// All tracking is non-blocking, fire-and-forget, and never affects app behavior.
package tracking

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/cdknorow/coral/internal/config"
	"github.com/google/uuid"
)

const posthogURL = "https://us.i.posthog.com/capture/"

var (
	cachedInstallID string
	installIDOnce   sync.Once
	coralDir        string // set by SetCoralDir; falls back to ~/.coral
)

// SetCoralDir sets the data directory used for tracking state files.
// Must be called before TrackInstallAsync(). If not called, falls back to ~/.coral.
func SetCoralDir(dir string) {
	coralDir = dir
}

// getInstallID returns the install ID, reading from disk once and caching.
func getInstallID() string {
	installIDOnce.Do(func() {
		idFile := filepath.Join(resolveCoralDir(), ".install_id")
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
			"edition": config.TierName,
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
	dir := resolveCoralDir()
	os.MkdirAll(dir, 0755)

	idFile := filepath.Join(dir, ".install_id")
	versionFile := filepath.Join(dir, ".install_version")

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
			"edition": config.TierName,
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
			"edition": config.TierName,
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

// resolveCoralDir returns the data directory for tracking state files.
func resolveCoralDir() string {
	if coralDir != "" {
		return coralDir
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".coral")
}

func generateUUID() string {
	return uuid.New().String()
}
