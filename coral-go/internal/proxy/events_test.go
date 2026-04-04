package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventHubSubscribePublish(t *testing.T) {
	hub := NewEventHub()
	ch := hub.Subscribe()
	defer hub.Unsubscribe(ch)

	hub.PublishStarted("req-1", "session-1", ProviderAnthropic, "claude-sonnet-4-20250514", true)

	select {
	case event := <-ch:
		assert.Equal(t, EventRequestStarted, event.Type)
		assert.Equal(t, "req-1", event.RequestID)
		assert.Equal(t, "session-1", event.SessionID)
		assert.Equal(t, "anthropic", event.Provider)
		assert.Equal(t, "claude-sonnet-4-20250514", event.Model)
		assert.True(t, event.Streaming)
		assert.NotEmpty(t, event.Timestamp)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestEventHubMultipleSubscribers(t *testing.T) {
	hub := NewEventHub()
	ch1 := hub.Subscribe()
	ch2 := hub.Subscribe()
	defer hub.Unsubscribe(ch1)
	defer hub.Unsubscribe(ch2)

	hub.PublishCompleted("req-2", "session-2", "gpt-4o",
		TokenUsage{InputTokens: 100, OutputTokens: 50}, 0.005, 1200, 200)

	for _, ch := range []chan Event{ch1, ch2} {
		select {
		case event := <-ch:
			assert.Equal(t, EventRequestCompleted, event.Type)
			assert.Equal(t, 100, event.InputTokens)
			assert.Equal(t, 50, event.OutputTokens)
			assert.Equal(t, 0.005, event.CostUSD)
			assert.Equal(t, 1200, event.LatencyMS)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for event")
		}
	}
}

func TestEventHubUnsubscribe(t *testing.T) {
	hub := NewEventHub()
	ch := hub.Subscribe()
	hub.Unsubscribe(ch)

	// Channel should be closed
	_, ok := <-ch
	assert.False(t, ok)

	// Publishing after unsubscribe should not panic
	hub.PublishStarted("req-3", "session-3", ProviderOpenAI, "gpt-4o", false)
}

func TestEventHubDropsSlowConsumer(t *testing.T) {
	hub := NewEventHub()
	ch := hub.Subscribe()
	defer hub.Unsubscribe(ch)

	// Fill the channel buffer (64)
	for i := 0; i < 100; i++ {
		hub.PublishStarted("req", "sess", ProviderAnthropic, "model", false)
	}

	// Should not block or panic - excess events dropped
	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			goto done
		}
	}
done:
	assert.Equal(t, 64, count) // buffer size
}

func TestEventHubPublishError(t *testing.T) {
	hub := NewEventHub()
	ch := hub.Subscribe()
	defer hub.Unsubscribe(ch)

	hub.PublishError("req-4", "session-4", "claude-opus-4-20250514", 429, "rate limited")

	select {
	case event := <-ch:
		assert.Equal(t, EventRequestError, event.Type)
		assert.Equal(t, "error", event.Status)
		assert.Equal(t, "rate limited", event.Error)
		assert.Equal(t, 429, event.HTTPStatus)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestEventHubConcurrentAccess(t *testing.T) {
	hub := NewEventHub()
	var wg sync.WaitGroup

	// Spawn multiple concurrent publishers and subscribers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch := hub.Subscribe()
			time.Sleep(10 * time.Millisecond)
			hub.Unsubscribe(ch)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			hub.PublishStarted("req", "sess", ProviderAnthropic, "model", false)
		}()
	}

	wg.Wait()
}

func TestEventHubEventsFromProxy(t *testing.T) {
	// Test that proxy request lifecycle emits events
	upstream := testUpstreamHandler(200, map[string]any{
		"id": "msg_1", "type": "message", "model": "claude-sonnet-4-20250514",
		"usage": map[string]any{"input_tokens": 500, "output_tokens": 100},
		"content": []map[string]any{{"type": "text", "text": "Hi"}},
	})
	server := startTestServer(t, upstream)
	defer server.Close()

	p := testProxy(t, server)
	ch := p.events.Subscribe()
	defer p.events.Unsubscribe(ch)

	// Make a request through the proxy
	makeAnthropicRequest(t, p, "event-test-session", `{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"Hi"}]}`)

	// Should get started event
	var started, completed bool
	timeout := time.After(2 * time.Second)
	for !(started && completed) {
		select {
		case event := <-ch:
			switch event.Type {
			case EventRequestStarted:
				started = true
				assert.Equal(t, "event-test-session", event.SessionID)
			case EventRequestCompleted:
				completed = true
				assert.Equal(t, "event-test-session", event.SessionID)
				assert.Equal(t, 500, event.InputTokens)
				assert.Equal(t, 100, event.OutputTokens)
				assert.Greater(t, event.CostUSD, 0.0)
			}
		case <-timeout:
			t.Fatalf("timed out waiting for events (started=%v, completed=%v)", started, completed)
		}
	}
}

// Test helpers

func testUpstreamHandler(statusCode int, response map[string]any) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		data, _ := json.Marshal(response)
		w.Write(data)
	})
}

func startTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func makeAnthropicRequest(t *testing.T, p *Proxy, sessionID, body string) *httptest.ResponseRecorder {
	t.Helper()
	router := chi.NewRouter()
	router.Post("/proxy/{sessionID}/v1/messages", p.HandleAnthropicMessages)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/proxy/"+sessionID+"/v1/messages", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, r)
	require.Equal(t, 200, w.Code)
	return w
}
