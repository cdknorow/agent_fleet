//go:build dev && !beta

package config

// Build tier: Dev
// Built with: go build -tags dev
// Skips EULA and license, no demo limits. For internal development only.
var (
	TierSkipEULA    = true
	TierSkipLicense = true
	TierDemoLimits  = false
	TierName        = "dev"
)
