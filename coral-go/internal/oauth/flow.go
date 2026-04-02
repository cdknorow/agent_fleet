package oauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// TokenResult holds the result of a token exchange or refresh.
type TokenResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int    // seconds until expiry
	TokenType    string
}

// Expiry returns the token expiry time as an ISO string, or nil if ExpiresIn is 0.
func (t *TokenResult) Expiry() *string {
	if t.ExpiresIn <= 0 {
		return nil
	}
	exp := time.Now().UTC().Add(time.Duration(t.ExpiresIn) * time.Second).Format("2006-01-02T15:04:05.000000+00:00")
	return &exp
}

// PendingAuth tracks an in-progress OAuth authorization flow.
type PendingAuth struct {
	State        string
	ProviderID   string
	Name         string
	ClientID     string
	ClientSecret string
	Scopes       string
	RedirectURI  string
	CreatedAt    time.Time
}

// FlowManager handles OAuth2 authorization flows.
type FlowManager struct {
	registry *Registry
	mu       sync.Mutex
	pending  map[string]*PendingAuth // state -> pending auth
}

// NewFlowManager creates a new FlowManager.
func NewFlowManager(registry *Registry) *FlowManager {
	return &FlowManager{
		registry: registry,
		pending:  make(map[string]*PendingAuth),
	}
}

// StartAuth initiates an OAuth2 authorization flow.
// Returns the authorization URL that the user should open in their browser.
func (fm *FlowManager) StartAuth(providerID, name, clientID, clientSecret, redirectURI string, scopes []string) (string, string, error) {
	provider := fm.registry.Get(providerID)
	if provider == nil {
		return "", "", fmt.Errorf("unknown provider: %s", providerID)
	}

	state, err := generateState()
	if err != nil {
		return "", "", fmt.Errorf("generate state: %w", err)
	}

	scopeStr := strings.Join(scopes, " ")

	// Build authorization URL
	params := url.Values{
		"client_id":     {clientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {scopeStr},
		"state":         {state},
		"access_type":   {"offline"}, // Request refresh token (Google-specific but harmless for others)
		"prompt":        {"consent"}, // Force consent screen to get refresh token
	}
	authURL := provider.AuthURL + "?" + params.Encode()

	// Store pending auth
	fm.mu.Lock()
	fm.pending[state] = &PendingAuth{
		State:        state,
		ProviderID:   providerID,
		Name:         name,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       scopeStr,
		RedirectURI:  redirectURI,
		CreatedAt:    time.Now(),
	}
	// Clean up stale pending auths inline (map is small, no need for goroutine)
	cutoff := time.Now().Add(-10 * time.Minute)
	for s, p := range fm.pending {
		if p.CreatedAt.Before(cutoff) {
			delete(fm.pending, s)
		}
	}
	fm.mu.Unlock()

	return authURL, state, nil
}

// CompleteAuth exchanges an authorization code for tokens.
// Returns the pending auth info and tokens, or an error.
func (fm *FlowManager) CompleteAuth(ctx context.Context, state, code string) (*PendingAuth, *TokenResult, error) {
	fm.mu.Lock()
	pending, ok := fm.pending[state]
	if ok {
		delete(fm.pending, state)
	}
	fm.mu.Unlock()

	if !ok {
		return nil, nil, fmt.Errorf("invalid or expired state parameter")
	}

	// Check if the pending auth is too old (10 minute window)
	if time.Since(pending.CreatedAt) > 10*time.Minute {
		return nil, nil, fmt.Errorf("authorization flow expired")
	}

	provider := fm.registry.Get(pending.ProviderID)
	if provider == nil {
		return nil, nil, fmt.Errorf("unknown provider: %s", pending.ProviderID)
	}

	// Exchange code for tokens
	tokens, err := exchangeCode(ctx, provider.TokenURL, pending.ClientID, pending.ClientSecret, code, pending.RedirectURI)
	if err != nil {
		return nil, nil, fmt.Errorf("token exchange failed: %w", err)
	}

	return pending, tokens, nil
}

// RefreshAccessToken uses a refresh token to get a new access token.
func (fm *FlowManager) RefreshAccessToken(ctx context.Context, providerID, clientID, clientSecret, refreshToken string) (string, string, *string, error) {
	provider := fm.registry.Get(providerID)
	if provider == nil {
		return "", "", nil, fmt.Errorf("unknown provider: %s", providerID)
	}

	tokens, err := refreshAccessToken(ctx, provider.TokenURL, clientID, clientSecret, refreshToken)
	if err != nil {
		return "", "", nil, err
	}

	return tokens.AccessToken, tokens.RefreshToken, tokens.Expiry(), nil
}

// BuildRefreshFn returns a refresh function bound to a specific provider,
// suitable for passing to ConnectedAppStore.GetFreshToken.
func (fm *FlowManager) BuildRefreshFn(providerID string) func(clientID, clientSecret, refreshToken string) (string, string, *string, error) {
	return func(clientID, clientSecret, refreshToken string) (string, string, *string, error) {
		return fm.RefreshAccessToken(context.Background(), providerID, clientID, clientSecret, refreshToken)
	}
}

// exchangeCode performs the OAuth2 token exchange.
func exchangeCode(ctx context.Context, tokenURL, clientID, clientSecret, code, redirectURI string) (*TokenResult, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"redirect_uri":  {redirectURI},
	}

	return doTokenRequest(ctx, tokenURL, data)
}

// refreshAccessToken performs an OAuth2 token refresh.
func refreshAccessToken(ctx context.Context, tokenURL, clientID, clientSecret, refreshToken string) (*TokenResult, error) {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	}

	return doTokenRequest(ctx, tokenURL, data)
}

// doTokenRequest sends a POST to the token endpoint and parses the response.
func doTokenRequest(ctx context.Context, tokenURL string, data url.Values) (*TokenResult, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	if tokenResp.Error != "" {
		return nil, fmt.Errorf("oauth error: %s - %s", tokenResp.Error, tokenResp.ErrorDesc)
	}
	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("no access token in response")
	}

	return &TokenResult{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresIn:    tokenResp.ExpiresIn,
		TokenType:    tokenResp.TokenType,
	}, nil
}

// generateState produces a random hex string for OAuth state parameter.
func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
