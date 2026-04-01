package routes

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/cdknorow/coral/internal/config"
	"github.com/cdknorow/coral/internal/oauth"
	"github.com/cdknorow/coral/internal/store"
)

// ConnectedAppsHandler handles connected apps API endpoints.
type ConnectedAppsHandler struct {
	as       *store.ConnectedAppStore
	cfg      *config.Config
	registry *oauth.Registry
	flow     *oauth.FlowManager
}

// NewConnectedAppsHandler creates a new ConnectedAppsHandler.
func NewConnectedAppsHandler(db *store.DB, cfg *config.Config) *ConnectedAppsHandler {
	registry := oauth.NewRegistry()
	return &ConnectedAppsHandler{
		as:       store.NewConnectedAppStore(db),
		cfg:      cfg,
		registry: registry,
		flow:     oauth.NewFlowManager(registry),
	}
}

// providerResponse is a provider with an additional field indicating embedded credentials.
type providerResponse struct {
	oauth.Provider
	OneClick bool `json:"one_click"`
}

// ListProviders returns all available OAuth providers.
// GET /api/connected-apps/providers
func (h *ConnectedAppsHandler) ListProviders(w http.ResponseWriter, r *http.Request) {
	providers := h.registry.List()
	result := make([]providerResponse, len(providers))
	for i, p := range providers {
		result[i] = providerResponse{
			Provider: p,
			OneClick: p.HasEmbeddedCredentials(),
		}
	}
	callbackURL := fmt.Sprintf("http://localhost:%d/api/connected-apps/callback", h.cfg.Port)
	writeJSON(w, http.StatusOK, map[string]any{
		"providers":    result,
		"callback_url": callbackURL,
	})
}

// ListConnections returns all connected apps (tokens redacted).
// GET /api/connected-apps
func (h *ConnectedAppsHandler) ListConnections(w http.ResponseWriter, r *http.Request) {
	apps, err := h.as.List(r.Context())
	if err != nil {
		errInternalServer(w, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": emptyIfNil(apps)})
}

// GetConnection returns a single connected app by ID (tokens redacted).
// GET /api/connected-apps/{id}
func (h *ConnectedAppsHandler) GetConnection(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		errBadRequest(w, "invalid connection ID")
		return
	}
	app, err := h.as.Get(r.Context(), id)
	if err != nil {
		errInternalServer(w, err.Error())
		return
	}
	if app == nil {
		errNotFound(w, "connection not found")
		return
	}
	writeJSON(w, http.StatusOK, app)
}

// StartAuth initiates an OAuth2 authorization flow.
// POST /api/connected-apps/auth/start
func (h *ConnectedAppsHandler) StartAuth(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProviderID   string   `json:"provider_id"`
		Name         string   `json:"name"`
		ClientID     string   `json:"client_id"`
		ClientSecret string   `json:"client_secret"`
		Scopes       []string `json:"scopes"`
	}
	if err := decodeJSON(r, &body); err != nil {
		errBadRequest(w, "invalid JSON")
		return
	}

	if body.ProviderID == "" {
		errBadRequest(w, "provider_id is required")
		return
	}
	if body.Name == "" {
		errBadRequest(w, "name is required")
		return
	}

	provider := h.registry.Get(body.ProviderID)
	if provider == nil {
		errBadRequest(w, "unknown provider")
		return
	}

	// Use embedded credentials if available, otherwise require user-supplied ones
	clientID := body.ClientID
	clientSecret := body.ClientSecret
	if provider.HasEmbeddedCredentials() {
		clientID = provider.ClientID
		clientSecret = provider.ClientSecret
	} else {
		if clientID == "" {
			errBadRequest(w, "client_id is required for this provider")
			return
		}
		if clientSecret == "" {
			errBadRequest(w, "client_secret is required for this provider")
			return
		}
	}

	scopes := body.Scopes
	if len(scopes) == 0 {
		scopes = provider.Scopes
	}

	existing, _ := h.as.GetByProviderAndName(r.Context(), body.ProviderID, body.Name)
	if existing != nil {
		errConflict(w, "a connection with this provider and name already exists")
		return
	}

	redirectURI := fmt.Sprintf("http://localhost:%d/api/connected-apps/callback", h.cfg.Port)

	authURL, state, err := h.flow.StartAuth(body.ProviderID, body.Name, clientID, clientSecret, redirectURI, scopes)
	if err != nil {
		errInternalServer(w, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"auth_url": authURL,
		"state":    state,
	})
}

// callbackTmpl is the HTML template for OAuth callback responses.
// Uses html/template for automatic XSS escaping.
var callbackTmpl = template.Must(template.New("callback").Parse(`<!DOCTYPE html>
<html><body>
<h2>{{.Title}}</h2>
<p>{{.Message}}</p>
{{if .Success}}
<script>
if (window.opener) {
	window.opener.postMessage(
		{type: 'oauth-complete', provider: {{.Provider}}, name: {{.Name}}},
		window.location.origin
	);
}
setTimeout(function() { window.close(); }, 2000);
</script>
{{else}}
<script>window.close();</script>
{{end}}
</body></html>`))

type callbackData struct {
	Title    string
	Message  string
	Success  bool
	Provider string
	Name     string
}

// Callback handles the OAuth2 redirect callback.
// GET /api/connected-apps/callback
func (h *ConnectedAppsHandler) Callback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	oauthErr := r.URL.Query().Get("error")

	w.Header().Set("Content-Type", "text/html")

	if oauthErr != "" {
		desc := r.URL.Query().Get("error_description")
		callbackTmpl.Execute(w, callbackData{
			Title:   "Authorization Failed",
			Message: oauthErr + ": " + desc,
		})
		return
	}

	if state == "" || code == "" {
		callbackTmpl.Execute(w, callbackData{
			Title:   "Invalid Callback",
			Message: "Missing state or code parameter.",
		})
		return
	}

	pending, tokens, err := h.flow.CompleteAuth(r.Context(), state, code)
	if err != nil {
		callbackTmpl.Execute(w, callbackData{
			Title:   "Authorization Failed",
			Message: err.Error(),
		})
		return
	}

	app := &store.ConnectedApp{
		ProviderID:   pending.ProviderID,
		Name:         pending.Name,
		ClientID:     pending.ClientID,
		ClientSecret: pending.ClientSecret,
		Scopes:       pending.Scopes,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		TokenExpiry:  tokens.Expiry(),
		Status:       "active",
	}

	created, err := h.as.Create(r.Context(), app)
	if err != nil {
		callbackTmpl.Execute(w, callbackData{
			Title:   "Failed to Save Connection",
			Message: err.Error(),
		})
		return
	}

	// Fetch account info in background (with timeout)
	go h.fetchAccountInfo(created.ID, pending.ProviderID, tokens.AccessToken)

	callbackTmpl.Execute(w, callbackData{
		Title:    "Connected!",
		Message:  "Successfully connected " + pending.Name + ". You can close this window.",
		Success:  true,
		Provider: pending.ProviderID,
		Name:     pending.Name,
	})
}

// GetToken returns a fresh access token for a connection (auto-refreshes if needed).
// GET /api/connected-apps/{id}/token
func (h *ConnectedAppsHandler) GetToken(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		errBadRequest(w, "invalid connection ID")
		return
	}

	app, err := h.as.Get(r.Context(), id)
	if err != nil {
		errInternalServer(w, err.Error())
		return
	}
	if app == nil {
		errNotFound(w, "connection not found")
		return
	}

	refreshFn := h.flow.BuildRefreshFn(app.ProviderID)
	token, err := h.as.GetFreshToken(r.Context(), id, refreshFn)
	if err != nil {
		errInternalServer(w, "failed to get fresh token: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": token,
		"provider_id":  app.ProviderID,
		"name":         app.Name,
	})
}

// DeleteConnection revokes tokens at the provider (best-effort) and deletes the connection.
// DELETE /api/connected-apps/{id}
func (h *ConnectedAppsHandler) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		errBadRequest(w, "invalid connection ID")
		return
	}
	app, err := h.as.Get(r.Context(), id)
	if err != nil {
		errInternalServer(w, err.Error())
		return
	}
	if app == nil {
		errNotFound(w, "connection not found")
		return
	}

	// Best-effort token revocation before deletion
	if app.AccessToken != "" {
		go revokeToken(app.ProviderID, app.AccessToken, app.ClientID, app.ClientSecret)
	}

	if err := h.as.Delete(r.Context(), id); err != nil {
		errInternalServer(w, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// TestConnection tests a connection by making a simple API call.
// POST /api/connected-apps/{id}/test
func (h *ConnectedAppsHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		errBadRequest(w, "invalid connection ID")
		return
	}

	app, err := h.as.Get(r.Context(), id)
	if err != nil {
		errInternalServer(w, err.Error())
		return
	}
	if app == nil {
		errNotFound(w, "connection not found")
		return
	}

	refreshFn := h.flow.BuildRefreshFn(app.ProviderID)
	token, err := h.as.GetFreshToken(r.Context(), id, refreshFn)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": "failed to get token: " + err.Error(),
		})
		return
	}

	email, name, testErr := fetchProfileInfo(r.Context(), app.ProviderID, token)
	if testErr != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": testErr.Error(),
		})
		return
	}

	if email != "" || name != "" {
		h.as.UpdateAccountInfo(r.Context(), id, email, name)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"account_email": email,
		"account_name":  name,
	})
}

// fetchAccountInfo fetches and stores account info from the provider (best effort, background).
func (h *ConnectedAppsHandler) fetchAccountInfo(appID int64, providerID, accessToken string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	email, name, err := fetchProfileInfo(ctx, providerID, accessToken)
	if err != nil {
		slog.Warn("failed to fetch account info", "provider", providerID, "error", err)
		return
	}
	if email != "" || name != "" {
		h.as.UpdateAccountInfo(ctx, appID, email, name)
	}
}
