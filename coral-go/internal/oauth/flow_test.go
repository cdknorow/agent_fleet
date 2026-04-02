package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	providers := r.List()
	if len(providers) != 4 {
		t.Fatalf("expected 4 providers, got %d", len(providers))
	}

	gmail := r.Get("gmail")
	if gmail == nil {
		t.Fatal("expected gmail provider")
	}
	if gmail.AuthURL == "" || gmail.TokenURL == "" {
		t.Error("gmail provider missing URLs")
	}

	unknown := r.Get("unknown")
	if unknown != nil {
		t.Error("expected nil for unknown provider")
	}
}

func TestEmbeddedCredentials(t *testing.T) {
	r := NewRegistry()

	gmail := r.Get("gmail")
	if !gmail.HasEmbeddedCredentials() {
		t.Error("gmail should have embedded credentials")
	}

	gcal := r.Get("google-calendar")
	if !gcal.HasEmbeddedCredentials() {
		t.Error("google-calendar should have embedded credentials")
	}

	github := r.Get("github")
	if github.HasEmbeddedCredentials() {
		t.Error("github should NOT have embedded credentials")
	}

	slack := r.Get("slack")
	if slack.HasEmbeddedCredentials() {
		t.Error("slack should NOT have embedded credentials")
	}
}

func TestStartAuth(t *testing.T) {
	registry := NewRegistry()
	fm := NewFlowManager(registry)

	authURL, state, err := fm.StartAuth("gmail", "My Gmail", "client123", "secret456",
		"http://localhost:8420/callback", []string{"email"})
	if err != nil {
		t.Fatalf("StartAuth: %v", err)
	}

	if state == "" {
		t.Error("expected non-empty state")
	}
	if authURL == "" {
		t.Error("expected non-empty auth URL")
	}

	// Verify URL contains expected params
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}
	if parsed.Query().Get("client_id") != "client123" {
		t.Error("auth URL missing client_id")
	}
	if parsed.Query().Get("state") != state {
		t.Error("auth URL state mismatch")
	}
	if parsed.Query().Get("scope") != "email" {
		t.Error("auth URL missing scope")
	}
}

func TestStartAuthUnknownProvider(t *testing.T) {
	registry := NewRegistry()
	fm := NewFlowManager(registry)

	_, _, err := fm.StartAuth("unknown", "test", "id", "secret", "http://localhost/cb", nil)
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestCompleteAuthInvalidState(t *testing.T) {
	registry := NewRegistry()
	fm := NewFlowManager(registry)

	_, _, err := fm.CompleteAuth(context.Background(), "bogus-state", "code123")
	if err == nil {
		t.Error("expected error for invalid state")
	}
}

func TestCompleteAuthWithMockTokenServer(t *testing.T) {
	// Set up a mock token server
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		r.ParseForm()
		if r.FormValue("grant_type") != "authorization_code" {
			t.Errorf("expected authorization_code grant type, got %s", r.FormValue("grant_type"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "mock-access-token",
			"refresh_token": "mock-refresh-token",
			"expires_in":    3600,
			"token_type":    "Bearer",
		})
	}))
	defer tokenServer.Close()

	// Create a custom provider pointing to our mock server
	registry := &Registry{
		providers: map[string]Provider{
			"test": {
				ID:       "test",
				Name:     "Test",
				AuthURL:  "https://example.com/auth",
				TokenURL: tokenServer.URL,
				Scopes:   []string{"read"},
			},
		},
	}
	fm := NewFlowManager(registry)

	// Start auth
	_, state, err := fm.StartAuth("test", "Test Connection", "client-id", "client-secret",
		"http://localhost:8420/callback", []string{"read"})
	if err != nil {
		t.Fatalf("StartAuth: %v", err)
	}

	// Complete auth
	pending, tokens, err := fm.CompleteAuth(context.Background(), state, "auth-code-123")
	if err != nil {
		t.Fatalf("CompleteAuth: %v", err)
	}

	if pending.ProviderID != "test" {
		t.Errorf("provider = %s, want test", pending.ProviderID)
	}
	if pending.Name != "Test Connection" {
		t.Errorf("name = %s, want Test Connection", pending.Name)
	}
	if tokens.AccessToken != "mock-access-token" {
		t.Errorf("access_token = %s, want mock-access-token", tokens.AccessToken)
	}
	if tokens.RefreshToken != "mock-refresh-token" {
		t.Errorf("refresh_token = %s, want mock-refresh-token", tokens.RefreshToken)
	}
	if tokens.ExpiresIn != 3600 {
		t.Errorf("expires_in = %d, want 3600", tokens.ExpiresIn)
	}
}

func TestRefreshAccessToken(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.FormValue("grant_type") != "refresh_token" {
			t.Errorf("expected refresh_token grant type, got %s", r.FormValue("grant_type"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "new-access-token",
			"expires_in":   7200,
		})
	}))
	defer tokenServer.Close()

	registry := &Registry{
		providers: map[string]Provider{
			"test": {ID: "test", TokenURL: tokenServer.URL},
		},
	}
	fm := NewFlowManager(registry)

	accessToken, refreshToken, expiry, err := fm.RefreshAccessToken(
		context.Background(), "test", "client-id", "client-secret", "old-refresh-token")
	if err != nil {
		t.Fatalf("RefreshAccessToken: %v", err)
	}
	if accessToken != "new-access-token" {
		t.Errorf("access_token = %s, want new-access-token", accessToken)
	}
	if refreshToken != "" {
		t.Errorf("refresh_token = %s, want empty (not returned)", refreshToken)
	}
	if expiry == nil {
		t.Error("expected non-nil expiry")
	}
}

func TestTokenResultExpiry(t *testing.T) {
	tr := &TokenResult{ExpiresIn: 3600}
	exp := tr.Expiry()
	if exp == nil {
		t.Fatal("expected non-nil expiry")
	}
	if !strings.Contains(*exp, "+00:00") {
		t.Errorf("expiry should be UTC, got %s", *exp)
	}

	tr2 := &TokenResult{ExpiresIn: 0}
	if tr2.Expiry() != nil {
		t.Error("expected nil expiry for ExpiresIn=0")
	}
}

func TestTokenExchangeError(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":             "invalid_grant",
			"error_description": "code expired",
		})
	}))
	defer tokenServer.Close()

	registry := &Registry{
		providers: map[string]Provider{
			"test": {ID: "test", AuthURL: "https://example.com/auth", TokenURL: tokenServer.URL},
		},
	}
	fm := NewFlowManager(registry)

	_, state, _ := fm.StartAuth("test", "test", "id", "secret", "http://localhost/cb", []string{"read"})
	_, _, err := fm.CompleteAuth(context.Background(), state, "bad-code")
	if err == nil {
		t.Error("expected error for invalid_grant")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("error should mention invalid_grant, got: %v", err)
	}
}

func TestCleanupStale(t *testing.T) {
	registry := NewRegistry()
	fm := NewFlowManager(registry)

	// Add a stale pending auth manually (zero time = old enough to be cleaned up)
	fm.mu.Lock()
	fm.pending["stale-state"] = &PendingAuth{
		State: "stale-state",
		// CreatedAt zero value is well past the 10-minute cutoff
	}
	fm.mu.Unlock()

	// Cleanup is now triggered inline during StartAuth — use a real provider
	fm.StartAuth("gmail", "test", "id", "secret", "http://localhost/cb", []string{"read"})

	fm.mu.Lock()
	_, exists := fm.pending["stale-state"]
	fm.mu.Unlock()

	if exists {
		t.Error("expected stale pending auth to be cleaned up")
	}
}

func TestGenerateState(t *testing.T) {
	state1, err := generateState()
	if err != nil {
		t.Fatalf("generateState: %v", err)
	}
	state2, _ := generateState()
	if state1 == state2 {
		t.Error("expected unique states")
	}
	if len(state1) != 32 { // 16 bytes = 32 hex chars
		t.Errorf("state length = %d, want 32", len(state1))
	}
}
