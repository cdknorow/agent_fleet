package proxy

import "os"

// Provider identifies an LLM provider.
type Provider string

const (
	ProviderAnthropic Provider = "anthropic"
	ProviderOpenAI    Provider = "openai"
	ProviderGoogle    Provider = "google"
)

// ProviderConfig holds the upstream URL and API key for a provider.
type ProviderConfig struct {
	BaseURL string
	APIKey  string
}

// DefaultProviderConfigs returns provider configs from environment variables.
func DefaultProviderConfigs() map[Provider]ProviderConfig {
	configs := map[Provider]ProviderConfig{
		ProviderAnthropic: {
			BaseURL: envOrDefault("CORAL_ANTHROPIC_BASE_URL", "https://api.anthropic.com"),
			APIKey:  os.Getenv("ANTHROPIC_API_KEY"),
		},
		ProviderOpenAI: {
			BaseURL: envOrDefault("CORAL_OPENAI_BASE_URL", "https://api.openai.com"),
			APIKey:  os.Getenv("OPENAI_API_KEY"),
		},
		ProviderGoogle: {
			BaseURL: envOrDefault("CORAL_GOOGLE_BASE_URL", "https://generativelanguage.googleapis.com"),
			APIKey:  os.Getenv("GOOGLE_API_KEY"),
		},
	}
	return configs
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
