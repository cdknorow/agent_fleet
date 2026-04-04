package proxy

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestGetRequestByID(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ctx := context.Background()

	// Create a request
	err := store.CreateRequest(ctx, "req-abc", "session-1", ProviderAnthropic, "claude-sonnet-4-20250514", false)
	require.NoError(t, err)

	// Complete it
	err = store.CompleteRequest(ctx, "req-abc", TokenUsage{InputTokens: 500, OutputTokens: 100}, CalculateCostBreakdown("claude-sonnet-4-20250514", TokenUsage{InputTokens: 500, OutputTokens: 100}), 200, "success", "")
	require.NoError(t, err)

	// Get by ID
	req, err := store.GetRequestByID(ctx, "req-abc")
	require.NoError(t, err)
	assert.Equal(t, "req-abc", req.RequestID)
	assert.Equal(t, "session-1", req.SessionID)
	assert.Equal(t, 500, req.InputTokens)
	assert.Equal(t, 100, req.OutputTokens)
	assert.Equal(t, "success", req.Status)

	// Not found
	_, err = store.GetRequestByID(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestGetStatsByAgent(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ctx := context.Background()

	// Create requests for different sessions
	store.CreateRequest(ctx, "r1", "agent-1", ProviderAnthropic, "claude-sonnet-4-20250514", false)
	store.CompleteRequest(ctx, "r1", TokenUsage{InputTokens: 1000}, CalculateCostBreakdown("claude-sonnet-4-20250514", TokenUsage{InputTokens: 1000}), 200, "success", "")

	store.CreateRequest(ctx, "r2", "agent-1", ProviderAnthropic, "claude-sonnet-4-20250514", false)
	store.CompleteRequest(ctx, "r2", TokenUsage{InputTokens: 2000}, CalculateCostBreakdown("claude-sonnet-4-20250514", TokenUsage{InputTokens: 2000}), 200, "success", "")

	store.CreateRequest(ctx, "r3", "agent-2", ProviderOpenAI, "gpt-4o", false)
	store.CompleteRequest(ctx, "r3", TokenUsage{InputTokens: 500}, CalculateCostBreakdown("gpt-4o", TokenUsage{InputTokens: 500}), 200, "success", "")

	byAgent, err := store.GetStatsByAgent(ctx, "1970-01-01")
	require.NoError(t, err)
	require.Len(t, byAgent, 2)

	// Ordered by cost descending
	assert.Equal(t, "agent-1", byAgent[0].SessionID)
	assert.Equal(t, 2, byAgent[0].Requests)
	assert.InDelta(t, 0.009, byAgent[0].CostUSD, 0.0001)

	assert.Equal(t, "agent-2", byAgent[1].SessionID)
	assert.Equal(t, 1, byAgent[1].Requests)
}

func TestGetStatsByAgentEmpty(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ctx := context.Background()

	byAgent, err := store.GetStatsByAgent(ctx, "1970-01-01")
	require.NoError(t, err)
	assert.NotNil(t, byAgent) // Empty slice, not nil
	assert.Len(t, byAgent, 0)
}

func TestCompleteRequestStoresCostBreakdown(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ctx := context.Background()

	err := store.CreateRequest(ctx, "req-breakdown", "session-1", ProviderAnthropic, "claude-sonnet-4-20250514", false)
	require.NoError(t, err)

	usage := TokenUsage{InputTokens: 1000, OutputTokens: 500, CacheReadTokens: 200, CacheWriteTokens: 100}
	breakdown := CalculateCostBreakdown("claude-sonnet-4-20250514", usage)
	err = store.CompleteRequest(ctx, "req-breakdown", usage, breakdown, 200, "success", "")
	require.NoError(t, err)

	req, err := store.GetRequestByID(ctx, "req-breakdown")
	require.NoError(t, err)
	assert.InDelta(t, breakdown.InputCostUSD, req.InputCostUSD, 0.000001)
	assert.InDelta(t, breakdown.OutputCostUSD, req.OutputCostUSD, 0.000001)
	assert.InDelta(t, breakdown.CacheReadCostUSD, req.CacheReadCostUSD, 0.000001)
	assert.InDelta(t, breakdown.CacheWriteCostUSD, req.CacheWriteCostUSD, 0.000001)
	assert.InDelta(t, breakdown.Pricing.InputPerMTok, req.PricingInputPerMTok, 0.000001)
	assert.InDelta(t, breakdown.TotalCostUSD, req.CostUSD, 0.000001)
}

func TestGetTaskRunCost(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	ctx := context.Background()

	db.MustExec(`CREATE TABLE scheduled_runs (id INTEGER PRIMARY KEY, session_id TEXT)`)
	db.MustExec(`INSERT INTO scheduled_runs (id, session_id) VALUES (42, 'task-session-1')`)

	err := store.CreateRequest(ctx, "task-r1", "task-session-1", ProviderAnthropic, "claude-sonnet-4-20250514", false)
	require.NoError(t, err)
	err = store.CompleteRequest(ctx, "task-r1", TokenUsage{InputTokens: 1000}, CalculateCostBreakdown("claude-sonnet-4-20250514", TokenUsage{InputTokens: 1000}), 200, "success", "")
	require.NoError(t, err)

	err = store.CreateRequest(ctx, "task-r2", "task-session-1", ProviderOpenAI, "gpt-4o", false)
	require.NoError(t, err)
	err = store.CompleteRequest(ctx, "task-r2", TokenUsage{InputTokens: 500, OutputTokens: 100}, CalculateCostBreakdown("gpt-4o", TokenUsage{InputTokens: 500, OutputTokens: 100}), 200, "success", "")
	require.NoError(t, err)

	cost, err := store.GetTaskRunCost(ctx, 42)
	require.NoError(t, err)
	assert.Equal(t, int64(42), cost.RunID)
	assert.Equal(t, 2, cost.TotalRequests)
	assert.Equal(t, 1500, cost.TotalInputTokens)
	assert.Equal(t, 100, cost.TotalOutputTokens)
	assert.Greater(t, cost.TotalCostUSD, 0.0)
}

func TestPricingTableStableOrder(t *testing.T) {
	rows := PricingTable()
	require.NotEmpty(t, rows)
	raw, err := json.Marshal(rows)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"model":"claude-haiku-4-20250514"`)
}
