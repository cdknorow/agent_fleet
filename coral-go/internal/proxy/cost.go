package proxy

import (
	"sort"
	"strings"
)

// ModelPricing holds per-million-token pricing for a model.
type ModelPricing struct {
	InputPerMTok      float64 // $ per 1M input tokens
	OutputPerMTok     float64 // $ per 1M output tokens
	CacheReadPerMTok  float64 // $ per 1M cache-read tokens (Anthropic)
	CacheWritePerMTok float64 // $ per 1M cache-write tokens (Anthropic)
}

// Pricing maps model names to their pricing. Keys should be checked with
// lookupPricing() which handles prefix matching for versioned model names.
var Pricing = map[string]ModelPricing{
	// Anthropic
	"claude-opus-4-20250514":   {InputPerMTok: 15.00, OutputPerMTok: 75.00, CacheReadPerMTok: 1.50, CacheWritePerMTok: 18.75},
	"claude-sonnet-4-20250514": {InputPerMTok: 3.00, OutputPerMTok: 15.00, CacheReadPerMTok: 0.30, CacheWritePerMTok: 3.75},
	"claude-haiku-4-20250514":  {InputPerMTok: 0.80, OutputPerMTok: 4.00, CacheReadPerMTok: 0.08, CacheWritePerMTok: 1.00},

	// OpenAI
	"gpt-4o":      {InputPerMTok: 2.50, OutputPerMTok: 10.00},
	"gpt-4o-mini": {InputPerMTok: 0.15, OutputPerMTok: 0.60},
	"o3":          {InputPerMTok: 2.00, OutputPerMTok: 8.00},

	// Google
	"gemini-2.5-pro":   {InputPerMTok: 1.25, OutputPerMTok: 10.00},
	"gemini-2.5-flash": {InputPerMTok: 0.15, OutputPerMTok: 0.60},
}

// lookupPricing finds pricing for a model, trying exact match first, then
// prefix matching. Handles shortened model names like "claude-opus-4" matching
// "claude-opus-4-20250514" by checking if the model is a prefix of a known key.
func lookupPricing(model string) (ModelPricing, bool) {
	// Exact match
	if p, ok := Pricing[model]; ok {
		return p, true
	}
	// Prefix match: the incoming model name is a prefix of a known key.
	// Pick the longest matching key to avoid ambiguity.
	var best ModelPricing
	bestLen := 0
	for key, p := range Pricing {
		if strings.HasPrefix(key, model) {
			if len(key) > bestLen {
				best = p
				bestLen = len(key)
			}
		}
	}
	if bestLen > 0 {
		return best, true
	}
	return ModelPricing{}, false
}

// TokenUsage holds token counts from a provider response.
type TokenUsage struct {
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
}

// CostBreakdown stores the applied pricing and computed dollar breakdown.
type CostBreakdown struct {
	Model             string       `json:"model"`
	PricingFound      bool         `json:"pricing_found"`
	Pricing           ModelPricing `json:"pricing"`
	InputCostUSD      float64      `json:"input_cost_usd"`
	OutputCostUSD     float64      `json:"output_cost_usd"`
	CacheReadCostUSD  float64      `json:"cache_read_cost_usd"`
	CacheWriteCostUSD float64      `json:"cache_write_cost_usd"`
	TotalCostUSD      float64      `json:"total_cost_usd"`
}

// PricingEntry is a serializable pricing table row.
type PricingEntry struct {
	Model string `json:"model"`
	ModelPricing
}

// PricingTable returns the current pricing table in stable model order.
func PricingTable() []PricingEntry {
	keys := make([]string, 0, len(Pricing))
	for model := range Pricing {
		keys = append(keys, model)
	}
	sort.Strings(keys)

	rows := make([]PricingEntry, 0, len(keys))
	for _, model := range keys {
		rows = append(rows, PricingEntry{
			Model:        model,
			ModelPricing: Pricing[model],
		})
	}
	return rows
}

// CalculateCostBreakdown computes the dollar cost breakdown for a request.
func CalculateCostBreakdown(model string, usage TokenUsage) CostBreakdown {
	pricing, ok := lookupPricing(model)
	if !ok {
		return CostBreakdown{Model: model}
	}

	breakdown := CostBreakdown{
		Model:             model,
		PricingFound:      true,
		Pricing:           pricing,
		InputCostUSD:      float64(usage.InputTokens) * pricing.InputPerMTok / 1_000_000,
		OutputCostUSD:     float64(usage.OutputTokens) * pricing.OutputPerMTok / 1_000_000,
		CacheReadCostUSD:  float64(usage.CacheReadTokens) * pricing.CacheReadPerMTok / 1_000_000,
		CacheWriteCostUSD: float64(usage.CacheWriteTokens) * pricing.CacheWritePerMTok / 1_000_000,
	}
	breakdown.TotalCostUSD = breakdown.InputCostUSD + breakdown.OutputCostUSD +
		breakdown.CacheReadCostUSD + breakdown.CacheWriteCostUSD
	return breakdown
}

// CalculateCost computes the dollar cost for a request.
func CalculateCost(model string, usage TokenUsage) float64 {
	return CalculateCostBreakdown(model, usage).TotalCostUSD
}
