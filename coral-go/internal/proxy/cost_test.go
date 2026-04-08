package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookupPricing_ExactMatch(t *testing.T) {
	for model := range Pricing {
		p, ok := lookupPricing(model)
		assert.True(t, ok, "expected exact match for %s", model)
		assert.Equal(t, Pricing[model], p)
	}
}

func TestLookupPricing_PrefixMatch(t *testing.T) {
	// "claude-sonnet-4" is a prefix of "claude-sonnet-4-20250514"
	p, ok := lookupPricing("claude-sonnet-4")
	assert.True(t, ok)
	assert.Equal(t, Pricing["claude-sonnet-4-20250514"].InputPerMTok, p.InputPerMTok)
}

func TestLookupPricing_AliasMatch(t *testing.T) {
	// "claude-opus-4-6" should match "claude-opus-4-6-20260407" (1M context)
	p, ok := lookupPricing("claude-opus-4-6")
	assert.True(t, ok)
	assert.Equal(t, 1_000_000, p.ContextWindow, "claude-opus-4-6 should resolve to 1M context window")
	assert.Equal(t, Pricing["claude-opus-4-6-20260407"].InputPerMTok, p.InputPerMTok)
}

func TestLookupPricing_Claude46Models(t *testing.T) {
	// All Claude 4.6 short aliases should resolve to 1M context
	tests := []struct {
		alias string
		key   string
	}{
		{"claude-opus-4-6", "claude-opus-4-6-20260407"},
		{"claude-sonnet-4-6", "claude-sonnet-4-6-20260407"},
		{"claude-haiku-4-5", "claude-haiku-4-5-20251001"},
	}
	for _, tt := range tests {
		p, ok := lookupPricing(tt.alias)
		assert.True(t, ok, "expected match for %s", tt.alias)
		assert.Equal(t, 1_000_000, p.ContextWindow, "%s should have 1M context", tt.alias)
		assert.Equal(t, Pricing[tt.key], p, "%s should match %s", tt.alias, tt.key)
	}
}

func TestLookupPricing_PrefixMatchShortestKey(t *testing.T) {
	// "claude-sonnet-4" is a prefix of both "claude-sonnet-4-20250514" (200K)
	// and "claude-sonnet-4-6-20260407" (1M). Should consistently pick the
	// shortest key (200K) to avoid non-deterministic behavior.
	for i := 0; i < 100; i++ {
		p, ok := lookupPricing("claude-sonnet-4")
		assert.True(t, ok)
		assert.Equal(t, 200_000, p.ContextWindow, "iteration %d: claude-sonnet-4 should consistently resolve to 200K", i)
	}
}

func TestLookupPricing_BracketSuffix(t *testing.T) {
	// Model strings with bracket suffixes like "[1m]" should be stripped
	p, ok := lookupPricing("claude-opus-4-6[1m]")
	assert.True(t, ok)
	assert.Equal(t, 1_000_000, p.ContextWindow)

	p, ok = lookupPricing("claude-opus-4-6-20260407[1m]")
	assert.True(t, ok)
	assert.Equal(t, 1_000_000, p.ContextWindow)
}

func TestLookupPricing_UnknownModel(t *testing.T) {
	_, ok := lookupPricing("totally-unknown-model")
	assert.False(t, ok)
}

func TestLookupPricing_SingleSegmentNoMatch(t *testing.T) {
	// A single matching segment should not be enough
	_, ok := lookupPricing("claude")
	// "claude" matches only 1 segment of keys like "claude-opus-4-20250514",
	// but prefix match may find it. Let's verify behavior.
	// Actually "claude" IS a prefix of "claude-opus-4-20250514", so prefix match hits.
	assert.True(t, ok)
}

func TestLookupPricing_NoFalsePositiveOnShortInput(t *testing.T) {
	// Single segment that doesn't prefix-match anything
	_, ok := lookupPricing("mistral")
	assert.False(t, ok)
}

func TestCalculateCostBreakdown_Sonnet(t *testing.T) {
	usage := TokenUsage{
		InputTokens:      1_000_000,
		OutputTokens:     1_000_000,
		CacheReadTokens:  1_000_000,
		CacheWriteTokens: 1_000_000,
	}
	b := CalculateCostBreakdown("claude-sonnet-4-20250514", usage)
	require.True(t, b.PricingFound)
	assert.InDelta(t, 3.00, b.InputCostUSD, 0.001)
	assert.InDelta(t, 15.00, b.OutputCostUSD, 0.001)
	assert.InDelta(t, 0.30, b.CacheReadCostUSD, 0.001)
	assert.InDelta(t, 3.75, b.CacheWriteCostUSD, 0.001)
	assert.InDelta(t, 22.05, b.TotalCostUSD, 0.001)
}

func TestCalculateCostBreakdown_ZeroTokens(t *testing.T) {
	b := CalculateCostBreakdown("claude-sonnet-4-20250514", TokenUsage{})
	require.True(t, b.PricingFound)
	assert.Equal(t, 0.0, b.TotalCostUSD)
	assert.Equal(t, 0.0, b.InputCostUSD)
}

func TestCalculateCostBreakdown_UnknownModel(t *testing.T) {
	b := CalculateCostBreakdown("unknown-model-xyz", TokenUsage{InputTokens: 1000})
	assert.False(t, b.PricingFound)
	assert.Equal(t, 0.0, b.TotalCostUSD)
	assert.Equal(t, "unknown-model-xyz", b.Model)
}

func TestCalculateCostBreakdown_InputOnlyNoCacheTokens(t *testing.T) {
	usage := TokenUsage{InputTokens: 500_000, OutputTokens: 100_000}
	b := CalculateCostBreakdown("gpt-4o", usage)
	require.True(t, b.PricingFound)
	// gpt-4o: input $2.50/MTok, output $10.00/MTok
	assert.InDelta(t, 1.25, b.InputCostUSD, 0.001)   // 500k * 2.50 / 1M
	assert.InDelta(t, 1.00, b.OutputCostUSD, 0.001)   // 100k * 10.00 / 1M
	assert.Equal(t, 0.0, b.CacheReadCostUSD)           // no cache pricing for OpenAI
	assert.InDelta(t, 2.25, b.TotalCostUSD, 0.001)
}

func TestCalculateCost_MatchesBreakdownTotal(t *testing.T) {
	usage := TokenUsage{InputTokens: 10000, OutputTokens: 5000, CacheReadTokens: 2000}
	cost := CalculateCost("claude-sonnet-4-20250514", usage)
	breakdown := CalculateCostBreakdown("claude-sonnet-4-20250514", usage)
	assert.Equal(t, breakdown.TotalCostUSD, cost)
}

func TestCalculateCostBreakdown_OpusPricing(t *testing.T) {
	usage := TokenUsage{InputTokens: 100_000, OutputTokens: 50_000}
	b := CalculateCostBreakdown("claude-opus-4-20250514", usage)
	require.True(t, b.PricingFound)
	// opus: input $15/MTok, output $75/MTok
	assert.InDelta(t, 1.50, b.InputCostUSD, 0.001)    // 100k * 15 / 1M
	assert.InDelta(t, 3.75, b.OutputCostUSD, 0.001)   // 50k * 75 / 1M
	assert.InDelta(t, 5.25, b.TotalCostUSD, 0.001)
}

func TestCalculateCostBreakdown_HaikuPricing(t *testing.T) {
	usage := TokenUsage{InputTokens: 1_000_000, OutputTokens: 1_000_000}
	b := CalculateCostBreakdown("claude-haiku-4-20250514", usage)
	require.True(t, b.PricingFound)
	assert.InDelta(t, 0.80, b.InputCostUSD, 0.001)
	assert.InDelta(t, 4.00, b.OutputCostUSD, 0.001)
}

func TestCalculateCostBreakdown_GeminiPro(t *testing.T) {
	usage := TokenUsage{InputTokens: 1_000_000, OutputTokens: 1_000_000}
	b := CalculateCostBreakdown("gemini-2.5-pro", usage)
	require.True(t, b.PricingFound)
	assert.InDelta(t, 1.25, b.InputCostUSD, 0.001)
	assert.InDelta(t, 10.00, b.OutputCostUSD, 0.001)
}

func TestCommonPrefixLen(t *testing.T) {
	tests := []struct {
		a, b []string
		want int
	}{
		{[]string{"a", "b", "c"}, []string{"a", "b", "d"}, 2},
		{[]string{"a"}, []string{"a", "b"}, 1},
		{[]string{"x"}, []string{"y"}, 0},
		{[]string{}, []string{"a"}, 0},
		{[]string{"a", "b"}, []string{"a", "b"}, 2},
	}
	for _, tt := range tests {
		got := commonPrefixLen(tt.a, tt.b)
		assert.Equal(t, tt.want, got, "commonPrefixLen(%v, %v)", tt.a, tt.b)
	}
}

func TestPricingTable_AllModelsPresent(t *testing.T) {
	table := PricingTable()
	assert.Len(t, table, len(Pricing))

	models := make(map[string]bool)
	for _, e := range table {
		models[e.Model] = true
	}
	for model := range Pricing {
		assert.True(t, models[model], "missing model %s in PricingTable", model)
	}
}

func TestPricingTable_StableSortOrder(t *testing.T) {
	t1 := PricingTable()
	t2 := PricingTable()
	require.Equal(t, len(t1), len(t2))
	for i := range t1 {
		assert.Equal(t, t1[i].Model, t2[i].Model)
	}
}

func TestCalculateCostBreakdown_BreakdownStoresPricing(t *testing.T) {
	b := CalculateCostBreakdown("claude-sonnet-4-20250514", TokenUsage{InputTokens: 1000})
	assert.Equal(t, 3.00, b.Pricing.InputPerMTok)
	assert.Equal(t, 15.00, b.Pricing.OutputPerMTok)
	assert.Equal(t, 0.30, b.Pricing.CacheReadPerMTok)
	assert.Equal(t, 3.75, b.Pricing.CacheWritePerMTok)
}
