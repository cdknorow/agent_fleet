//go:build dev && !beta && !dropboxers

package config

// Build tier: Dev
// Built with: go build -tags dev
// Skips EULA and license, no demo limits. For internal development only.
var (
	TierSkipEULA    = true
	TierSkipLicense = true
	TierDemoLimits  = false
	TierName        = "dev"
	TierMaxTeams    = 0
	TierMaxAgents   = 0

	// Test store (Lemon Squeezy sandbox) — single product with built-in free trial
	StoreURL = "https://store.coralai.ai/checkout/buy/44df39dc-9891-4094-8b77-f73c1d2596ae"
)
