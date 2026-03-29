//go:build staging && !dev && !beta && !dropboxers

package config

// Build tier: Staging
// Built with: go build -tags staging
// Shows EULA and license pages (like prod) but uses the test/sandbox store.
var (
	TierSkipEULA    = false
	TierSkipLicense = false
	TierDemoLimits  = false
	TierName        = "staging"
	TierMaxTeams    = 0
	TierMaxAgents   = 0

	// Test store (Lemon Squeezy sandbox) — single product with built-in free trial
	StoreURL = "https://store.coralai.ai/checkout/buy/44df39dc-9891-4094-8b77-f73c1d2596ae"
)
