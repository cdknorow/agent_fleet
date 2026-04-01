// Package oauth provides the OAuth2 provider registry and flow handling for Connected Apps.
package oauth

// Provider describes an OAuth2 service that Coral can connect to.
type Provider struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	AuthURL      string   `json:"auth_url"`
	TokenURL     string   `json:"token_url"`
	Scopes       []string `json:"scopes"`
	Icon         string   `json:"icon"`
	Instructions string   `json:"instructions"`
	// Embedded credentials — when set, users don't need to provide their own OAuth app.
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"-"` // never expose in API responses
}

// HasEmbeddedCredentials returns true if the provider ships with built-in OAuth credentials.
func (p *Provider) HasEmbeddedCredentials() bool {
	return p.ClientID != "" && p.ClientSecret != ""
}

// builtinProviders is the set of providers known to Coral out of the box.
var builtinProviders = []Provider{
	{
		ID:           "gmail",
		Name:         "Gmail",
		AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:     "https://oauth2.googleapis.com/token",
		ClientID:     "GOOGLE_OAUTH_CLIENT_ID",
		ClientSecret: "GOOGLE_OAUTH_CLIENT_SECRET",
		Scopes: []string{
			"https://www.googleapis.com/auth/gmail.readonly",
		},
		Icon:         "mail",
		Instructions: "Connect your Gmail account to let Coral read your emails.",
	},
	{
		ID:           "google-calendar",
		Name:         "Google Calendar",
		AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:     "https://oauth2.googleapis.com/token",
		ClientID:     "GOOGLE_OAUTH_CLIENT_ID",
		ClientSecret: "GOOGLE_OAUTH_CLIENT_SECRET",
		Scopes: []string{
			"https://www.googleapis.com/auth/calendar.readonly",
		},
		Icon:         "calendar_today",
		Instructions: "Connect your Google Calendar to let Coral check your schedule.",
	},
	{
		ID:       "github",
		Name:     "GitHub",
		AuthURL:  "https://github.com/login/oauth/authorize",
		TokenURL: "https://github.com/login/oauth/access_token",
		Scopes:   []string{"repo", "read:org"},
		Icon:     "code",
		Instructions: "Create an OAuth App in GitHub (Settings > Developer settings > OAuth Apps). Set the authorization callback URL to the callback URL shown below.",
	},
	{
		ID:       "slack",
		Name:     "Slack",
		AuthURL:  "https://slack.com/oauth/v2/authorize",
		TokenURL: "https://slack.com/api/oauth.v2.access",
		Scopes:   []string{"channels:read", "chat:write"},
		Icon:     "chat",
		Instructions: "Create a Slack App at api.slack.com/apps. Add the OAuth redirect URL in OAuth & Permissions, then install to your workspace.",
	},
}

// Registry holds available OAuth providers.
type Registry struct {
	providers map[string]Provider
}

// NewRegistry creates a registry with built-in providers.
func NewRegistry() *Registry {
	r := &Registry{
		providers: make(map[string]Provider, len(builtinProviders)),
	}
	for _, p := range builtinProviders {
		r.providers[p.ID] = p
	}
	return r
}

// Get returns a provider by ID, or nil if not found.
func (r *Registry) Get(id string) *Provider {
	p, ok := r.providers[id]
	if !ok {
		return nil
	}
	return &p
}

// List returns all available providers.
func (r *Registry) List() []Provider {
	result := make([]Provider, 0, len(r.providers))
	for _, p := range builtinProviders {
		if _, ok := r.providers[p.ID]; ok {
			result = append(result, p)
		}
	}
	return result
}
