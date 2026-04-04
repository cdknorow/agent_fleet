package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/cdknorow/coral/internal/proxy"
)

func testProxyDashboardDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestProxyPricingEndpoint(t *testing.T) {
	db := proxy.NewStore(testProxyDashboardDB(t))
	h := NewProxyDashboardHandler(db, proxy.NewEventHub())

	req := httptest.NewRequest(http.MethodGet, "/api/proxy/pricing", nil)
	w := httptest.NewRecorder()
	h.Pricing(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	models, ok := body["models"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, models)
}

func TestProxyTaskRunCostEndpoint(t *testing.T) {
	db := testProxyDashboardDB(t)
	ps := proxy.NewStore(db)
	db.MustExec(`CREATE TABLE scheduled_runs (id INTEGER PRIMARY KEY, session_id TEXT)`)
	db.MustExec(`INSERT INTO scheduled_runs (id, session_id) VALUES (7, 'task-session-7')`)

	ctx := t.Context()
	err := ps.CreateRequest(ctx, "req-task-7", "task-session-7", proxy.ProviderAnthropic, "claude-sonnet-4-20250514", false)
	require.NoError(t, err)
	usage := proxy.TokenUsage{InputTokens: 1000, OutputTokens: 200}
	err = ps.CompleteRequest(ctx, "req-task-7", usage, proxy.CalculateCostBreakdown("claude-sonnet-4-20250514", usage), 200, "success", "")
	require.NoError(t, err)

	h := NewProxyDashboardHandler(ps, proxy.NewEventHub())
	r := chi.NewRouter()
	r.Get("/api/proxy/tasks/runs/{runID}/cost", h.TaskRunCost)

	req := httptest.NewRequest(http.MethodGet, "/api/proxy/tasks/runs/7/cost", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, float64(7), body["run_id"])
	assert.Equal(t, float64(1), body["total_requests"])
	assert.Greater(t, body["total_cost_usd"].(float64), 0.0)
}
