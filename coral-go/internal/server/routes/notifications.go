package routes

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// NotificationLink is an optional link shown in the notification.
type NotificationLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// Notification represents a UI notification pushed via the API.
type Notification struct {
	ID        int64             `json:"id"`
	Title     string            `json:"title"`
	Message   string            `json:"message"`
	Type      string            `json:"type"`  // "toast" (default) or "alert" (centered modal, requires OK click)
	Level     string            `json:"level"` // "info", "success", "warning", "error"
	Link      *NotificationLink `json:"link,omitempty"`
	CreatedAt string            `json:"created_at"`
}

// NotificationStore is a simple in-memory queue for UI notifications.
// Notifications are ephemeral — they're delivered via WebSocket and
// discarded after being read or after expiry.
type NotificationStore struct {
	mu      sync.Mutex
	items   []Notification
	nextID  int64
	maxAge  time.Duration
}

// NewNotificationStore creates a new notification store.
func NewNotificationStore() *NotificationStore {
	return &NotificationStore{
		maxAge: 5 * time.Minute,
	}
}

// Push adds a notification to the queue.
func (ns *NotificationStore) Push(n Notification) Notification {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	ns.nextID++
	n.ID = ns.nextID
	n.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	if n.Type == "" {
		n.Type = "toast"
	}
	if n.Level == "" {
		n.Level = "info"
	}
	ns.items = append(ns.items, n)
	return n
}

// Drain returns and removes all pending notifications, pruning expired ones.
func (ns *NotificationStore) Drain() []Notification {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	if len(ns.items) == 0 {
		return nil
	}
	cutoff := time.Now().Add(-ns.maxAge)
	var live []Notification
	for _, n := range ns.items {
		if t, err := time.Parse(time.RFC3339, n.CreatedAt); err == nil && t.After(cutoff) {
			live = append(live, n)
		}
	}
	ns.items = nil
	return live
}

// NotificationHandler handles the POST /api/notifications endpoint.
type NotificationHandler struct {
	store *NotificationStore
}

// NewNotificationHandler creates a new handler with the given store.
func NewNotificationHandler(store *NotificationStore) *NotificationHandler {
	return &NotificationHandler{store: store}
}

// Create handles POST /api/notifications.
// Accepts {"title": "...", "message": "...", "level": "info|success|warning|error"}
func (h *NotificationHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title   string            `json:"title"`
		Message string            `json:"message"`
		Type    string            `json:"type"`
		Level   string            `json:"level"`
		Link    *NotificationLink `json:"link,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errBadRequest(w, "invalid JSON")
		return
	}
	if body.Message == "" && body.Title == "" {
		errBadRequest(w, "title or message is required")
		return
	}

	n := h.store.Push(Notification{
		Title:   body.Title,
		Message: body.Message,
		Type:    body.Type,
		Level:   body.Level,
		Link:    body.Link,
	})
	writeJSON(w, http.StatusOK, n)
}
