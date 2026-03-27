//go:build !dev && !beta

package config

// Build tier: Prod (default)
// Built with: go build (no tags)
// Requires EULA and license. Demo limits controlled by LS plan at runtime.
var (
	TierSkipEULA    = false
	TierSkipLicense = false
	TierDemoLimits  = false
	TierName        = "prod"
)
