package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenUsageStore_RecordAndGet(t *testing.T) {
	db := openTestDB(t)
	s := NewTokenUsageStore(db)
	ctx := context.Background()

	err := s.RecordUsage(ctx, &TokenUsage{
		SessionID:    "sess-1",
		AgentName:    "my-agent",
		AgentType:    "claude",
		InputTokens:  1000,
		OutputTokens: 500,
		TotalTokens:  1500,
		CostUSD:      0.03,
		NumTurns:     5,
	})
	require.NoError(t, err)

	got, err := s.GetSessionUsage(ctx, "sess-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "sess-1", got.SessionID)
	assert.Equal(t, 1000, got.InputTokens)
	assert.Equal(t, 500, got.OutputTokens)
	assert.Equal(t, 1500, got.TotalTokens)
	assert.InDelta(t, 0.03, got.CostUSD, 0.001)
	assert.Equal(t, 5, got.NumTurns)
}

func TestTokenUsageStore_GetSessionUsage_Latest(t *testing.T) {
	db := openTestDB(t)
	s := NewTokenUsageStore(db)
	ctx := context.Background()

	// Record two snapshots for the same session
	err := s.RecordUsage(ctx, &TokenUsage{
		SessionID: "sess-1", AgentName: "a", InputTokens: 100, OutputTokens: 50, TotalTokens: 150, CostUSD: 0.01,
	})
	require.NoError(t, err)

	err = s.RecordUsage(ctx, &TokenUsage{
		SessionID: "sess-1", AgentName: "a", InputTokens: 200, OutputTokens: 100, TotalTokens: 300, CostUSD: 0.02,
	})
	require.NoError(t, err)

	// Should return the latest
	got, err := s.GetSessionUsage(ctx, "sess-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 300, got.TotalTokens)
}

func TestTokenUsageStore_GetSessionUsage_NotFound(t *testing.T) {
	db := openTestDB(t)
	s := NewTokenUsageStore(db)
	ctx := context.Background()

	got, err := s.GetSessionUsage(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestTokenUsageStore_GetTeamUsage(t *testing.T) {
	db := openTestDB(t)
	s := NewTokenUsageStore(db)
	ctx := context.Background()

	teamID := int64(1)
	// Two sessions in the same team, each with 2 snapshots
	err := s.RecordUsage(ctx, &TokenUsage{
		SessionID: "s1", AgentName: "a1", TeamID: &teamID, InputTokens: 100, TotalTokens: 150, CostUSD: 0.01,
	})
	require.NoError(t, err)
	err = s.RecordUsage(ctx, &TokenUsage{
		SessionID: "s1", AgentName: "a1", TeamID: &teamID, InputTokens: 200, TotalTokens: 300, CostUSD: 0.02,
	})
	require.NoError(t, err)
	err = s.RecordUsage(ctx, &TokenUsage{
		SessionID: "s2", AgentName: "a2", TeamID: &teamID, InputTokens: 500, TotalTokens: 800, CostUSD: 0.05,
	})
	require.NoError(t, err)

	summary, err := s.GetTeamUsage(ctx, teamID)
	require.NoError(t, err)
	// Should sum latest per session: s1(300) + s2(800) = 1100
	assert.Equal(t, int64(1100), summary.TotalTokens)
	assert.InDelta(t, 0.07, summary.CostUSD, 0.001)
	assert.Equal(t, 2, summary.NumSessions)
}

func TestTokenUsageStore_GetBoardUsage(t *testing.T) {
	db := openTestDB(t)
	s := NewTokenUsageStore(db)
	ctx := context.Background()

	board := "my-board"
	err := s.RecordUsage(ctx, &TokenUsage{
		SessionID: "s1", AgentName: "a1", BoardName: &board, TotalTokens: 500, CostUSD: 0.03,
	})
	require.NoError(t, err)
	err = s.RecordUsage(ctx, &TokenUsage{
		SessionID: "s2", AgentName: "a2", BoardName: &board, TotalTokens: 300, CostUSD: 0.02,
	})
	require.NoError(t, err)

	summary, err := s.GetBoardUsage(ctx, "my-board")
	require.NoError(t, err)
	assert.Equal(t, int64(800), summary.TotalTokens)
	assert.InDelta(t, 0.05, summary.CostUSD, 0.001)
}

func TestTokenUsageStore_GetUsageSummary(t *testing.T) {
	db := openTestDB(t)
	s := NewTokenUsageStore(db)
	ctx := context.Background()

	err := s.RecordUsage(ctx, &TokenUsage{
		SessionID: "s1", AgentName: "a1", AgentType: "claude", TotalTokens: 1000, CostUSD: 0.05,
	})
	require.NoError(t, err)
	err = s.RecordUsage(ctx, &TokenUsage{
		SessionID: "s2", AgentName: "a2", AgentType: "gemini", TotalTokens: 2000, CostUSD: 0.01,
	})
	require.NoError(t, err)

	summaries, err := s.GetUsageSummary(ctx, "")
	require.NoError(t, err)
	assert.Len(t, summaries, 2)

	// Find claude summary
	for _, s := range summaries {
		if s.AgentType == "claude" {
			assert.Equal(t, int64(1000), s.TotalTokens)
			assert.Equal(t, 1, s.NumSessions)
		}
		if s.AgentType == "gemini" {
			assert.Equal(t, int64(2000), s.TotalTokens)
		}
	}
}

func TestTokenUsageStore_ListUsage(t *testing.T) {
	db := openTestDB(t)
	s := NewTokenUsageStore(db)
	ctx := context.Background()

	teamID := int64(1)
	board := "b1"
	err := s.RecordUsage(ctx, &TokenUsage{
		SessionID: "s1", AgentName: "a1", TeamID: &teamID, BoardName: &board, TotalTokens: 100,
	})
	require.NoError(t, err)
	err = s.RecordUsage(ctx, &TokenUsage{
		SessionID: "s2", AgentName: "a2", TotalTokens: 200,
	})
	require.NoError(t, err)

	// List all
	results, err := s.ListUsage(ctx, UsageFilter{})
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Filter by session
	results, err = s.ListUsage(ctx, UsageFilter{SessionID: "s1"})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "s1", results[0].SessionID)

	// Filter by team
	results, err = s.ListUsage(ctx, UsageFilter{TeamID: &teamID})
	require.NoError(t, err)
	assert.Len(t, results, 1)

	// Filter by board
	results, err = s.ListUsage(ctx, UsageFilter{BoardName: "b1"})
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestTokenUsageStore_DefaultAgentType(t *testing.T) {
	db := openTestDB(t)
	s := NewTokenUsageStore(db)
	ctx := context.Background()

	err := s.RecordUsage(ctx, &TokenUsage{
		SessionID: "s1", AgentName: "a1", TotalTokens: 100,
	})
	require.NoError(t, err)

	got, err := s.GetSessionUsage(ctx, "s1")
	require.NoError(t, err)
	assert.Equal(t, "claude", got.AgentType)
}
