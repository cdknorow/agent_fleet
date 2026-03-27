//go:build beta && !dev

package config

// Build tier: Beta
// Built with: go build -tags beta
// Requires EULA, skips license, enforces demo limits (2 teams / 8 agents).
var (
	TierSkipEULA    = false
	TierSkipLicense = true
	TierDemoLimits  = true
	TierName        = "beta"
)
