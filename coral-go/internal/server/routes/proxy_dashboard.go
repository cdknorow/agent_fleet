package routes

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"github.com/cdknorow/coral/internal/proxy"
)

// ProxyDashboardHandler serves the proxy cost/request dashboard API.
type ProxyDashboardHandler struct {
	store  *proxy.Store
	events *proxy.EventHub
}

// NewProxyDashboardHandler creates a handler for proxy dashboard endpoints.
func NewProxyDashboardHandler(store *proxy.Store, events *proxy.EventHub) *ProxyDashboardHandler {
	return &ProxyDashboardHandler{store: store, events: events}
}

// Stats returns aggregated proxy cost statistics.
// GET /api/proxy/stats
func (h *ProxyDashboardHandler) Stats(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	sessionID := r.URL.Query().Get("session_id")

	since := periodToSince(period)
	stats, byModel, err := h.store.GetStats(r.Context(), since, sessionID)
	if err != nil {
		errInternalServer(w, err.Error())
		return
	}

	resp := map[string]any{
		"period":                   period,
		"total_requests":           stats.TotalRequests,
		"total_cost_usd":           stats.TotalCostUSD,
		"total_input_tokens":       stats.TotalInputTokens,
		"total_output_tokens":      stats.TotalOutputTokens,
		"total_cache_read_tokens":  stats.TotalCacheReadTokens,
		"total_cache_write_tokens": stats.TotalCacheWriteTokens,
		"by_model":                 byModel,
	}

	// Include per-agent breakdown when not filtering by session
	if sessionID == "" {
		byAgent, err := h.store.GetStatsByAgent(r.Context(), since)
		if err == nil {
			resp["by_agent"] = byAgent
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// ListRequests returns recent proxy requests.
// GET /api/proxy/requests
func (h *ProxyDashboardHandler) ListRequests(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)

	requests, total, err := h.store.ListRequests(r.Context(), sessionID, limit, offset)
	if err != nil {
		errInternalServer(w, err.Error())
		return
	}
	if requests == nil {
		requests = []proxy.ProxyRequest{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"requests": requests,
		"total":    total,
	})
}

// GetRequest returns a single proxy request by ID.
// GET /api/proxy/requests/{requestID}
func (h *ProxyDashboardHandler) GetRequest(w http.ResponseWriter, r *http.Request) {
	requestID := chi.URLParam(r, "requestID")
	req, err := h.store.GetRequestByID(r.Context(), requestID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			errNotFound(w, "request not found")
			return
		}
		errInternalServer(w, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, req)
}

// SessionCost returns cost summary for a session.
// GET /api/proxy/session/{sessionID}/cost
func (h *ProxyDashboardHandler) SessionCost(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	stats, byModel, err := h.store.SessionCost(r.Context(), sessionID)
	if err != nil {
		errInternalServer(w, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id":               sessionID,
		"total_requests":           stats.TotalRequests,
		"total_cost_usd":           stats.TotalCostUSD,
		"total_input_tokens":       stats.TotalInputTokens,
		"total_output_tokens":      stats.TotalOutputTokens,
		"total_cache_read_tokens":  stats.TotalCacheReadTokens,
		"total_cache_write_tokens": stats.TotalCacheWriteTokens,
		"by_model":                 byModel,
	})
}

// Pricing returns the current model pricing table used for proxy cost calculation.
// GET /api/proxy/pricing
func (h *ProxyDashboardHandler) Pricing(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"models": proxy.PricingTable(),
	})
}

// TaskRunCost returns proxy cost totals for a one-shot task run.
// GET /api/proxy/tasks/runs/{runID}/cost
func (h *ProxyDashboardHandler) TaskRunCost(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.ParseInt(chi.URLParam(r, "runID"), 10, 64)
	if err != nil {
		errBadRequest(w, "invalid run ID")
		return
	}
	cost, err := h.store.GetTaskRunCost(r.Context(), runID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			errNotFound(w, "run not found")
			return
		}
		errInternalServer(w, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cost)
}

// WSProxy streams proxy events in real time via WebSocket.
// GET /ws/proxy
func (h *ProxyDashboardHandler) WSProxy(w http.ResponseWriter, r *http.Request) {
	opts := &websocket.AcceptOptions{
		OriginPatterns: []string{
			"localhost", "localhost:*",
			"127.0.0.1", "127.0.0.1:*",
			"[::1]", "[::1]:*",
		},
	}
	if host := r.Host; host != "" {
		opts.OriginPatterns = append(opts.OriginPatterns, host)
	}

	conn, err := websocket.Accept(w, r, opts)
	if err != nil {
		slog.Debug("ws/proxy accept failed", "error", err)
		return
	}
	defer conn.CloseNow()

	ctx := conn.CloseRead(r.Context())
	ch := h.events.Subscribe()
	defer h.events.Unsubscribe(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			if err := wsjson.Write(ctx, conn, event); err != nil {
				return
			}
		}
	}
}

func periodToSince(period string) string {
	now := time.Now().UTC()
	switch period {
	case "hour":
		return now.Add(-1 * time.Hour).Format(time.RFC3339)
	case "week":
		return now.AddDate(0, 0, -7).Format(time.RFC3339)
	case "month":
		return now.AddDate(0, -1, 0).Format(time.RFC3339)
	default: // "day" or unspecified
		return now.AddDate(0, 0, -1).Format(time.RFC3339)
	}
}
