package proxy

import (
	"sync"
	"time"
)

// EventType identifies the kind of proxy event.
type EventType string

const (
	EventRequestStarted   EventType = "request_started"
	EventRequestCompleted EventType = "request_completed"
	EventRequestError     EventType = "request_error"
)

// Event represents a proxy event broadcast to WebSocket subscribers.
type Event struct {
	Type      EventType `json:"type"`
	RequestID string    `json:"request_id"`
	SessionID string    `json:"session_id"`
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	Streaming bool      `json:"streaming"`
	Timestamp string    `json:"timestamp"`

	// Populated on completion/error
	InputTokens  int     `json:"input_tokens,omitempty"`
	OutputTokens int     `json:"output_tokens,omitempty"`
	CostUSD      float64 `json:"cost_usd,omitempty"`
	LatencyMS    int     `json:"latency_ms,omitempty"`
	Status       string  `json:"status,omitempty"`
	Error        string  `json:"error,omitempty"`
	HTTPStatus   int     `json:"http_status,omitempty"`
}

// EventHub broadcasts proxy events to subscribers. Thread-safe.
type EventHub struct {
	mu   sync.RWMutex
	subs map[chan Event]struct{}
}

// NewEventHub creates a new event hub.
func NewEventHub() *EventHub {
	return &EventHub{
		subs: make(map[chan Event]struct{}),
	}
}

// Subscribe returns a channel that receives proxy events.
// The caller must call Unsubscribe when done.
func (h *EventHub) Subscribe() chan Event {
	ch := make(chan Event, 64)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel and closes it.
func (h *EventHub) Unsubscribe(ch chan Event) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
	close(ch)
}

// Publish sends an event to all subscribers. Non-blocking — drops events
// for slow consumers to avoid blocking the proxy request path.
func (h *EventHub) Publish(e Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs {
		select {
		case ch <- e:
		default:
			// Slow consumer, drop event
		}
	}
}

// PublishStarted emits a request_started event.
func (h *EventHub) PublishStarted(reqID, sessionID string, provider Provider, model string, streaming bool) {
	h.Publish(Event{
		Type:      EventRequestStarted,
		RequestID: reqID,
		SessionID: sessionID,
		Provider:  string(provider),
		Model:     model,
		Streaming: streaming,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// PublishCompleted emits a request_completed event.
func (h *EventHub) PublishCompleted(reqID, sessionID, model string, usage TokenUsage, cost float64, latencyMS int, httpStatus int) {
	h.Publish(Event{
		Type:         EventRequestCompleted,
		RequestID:    reqID,
		SessionID:    sessionID,
		Model:        model,
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		CostUSD:      cost,
		LatencyMS:    latencyMS,
		Status:       "success",
		HTTPStatus:   httpStatus,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	})
}

// PublishError emits a request_error event.
func (h *EventHub) PublishError(reqID, sessionID, model string, httpStatus int, errMsg string) {
	h.Publish(Event{
		Type:       EventRequestError,
		RequestID:  reqID,
		SessionID:  sessionID,
		Model:      model,
		Status:     "error",
		Error:      errMsg,
		HTTPStatus: httpStatus,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	})
}
