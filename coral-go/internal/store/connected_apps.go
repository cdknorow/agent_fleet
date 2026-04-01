package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// ConnectedApp represents an OAuth2 connection to an external service.
type ConnectedApp struct {
	ID            int64   `db:"id" json:"id"`
	ProviderID    string  `db:"provider_id" json:"provider_id"`
	Name          string  `db:"name" json:"name"`
	ClientID      string  `db:"client_id" json:"-"`
	ClientSecret  string  `db:"client_secret" json:"-"`
	Scopes        string  `db:"scopes" json:"scopes"`
	AccessToken   string  `db:"access_token" json:"-"`
	RefreshToken  string  `db:"refresh_token" json:"-"`
	TokenExpiry   *string `db:"token_expiry" json:"-"`
	AccountEmail  string  `db:"account_email" json:"account_email,omitempty"`
	AccountName   string  `db:"account_name" json:"account_name,omitempty"`
	Status        string  `db:"status" json:"status"`
	CreatedAt     string  `db:"created_at" json:"created_at"`
	UpdatedAt     string  `db:"updated_at" json:"updated_at"`
}

// ScopesList returns the scopes as a string slice.
func (c *ConnectedApp) ScopesList() []string {
	if c.Scopes == "" {
		return nil
	}
	return strings.Split(c.Scopes, " ")
}

// IsTokenExpired returns true if the access token is expired or will expire within the buffer duration.
func (c *ConnectedApp) IsTokenExpired(buffer time.Duration) bool {
	if c.TokenExpiry == nil || *c.TokenExpiry == "" {
		return true
	}
	expiry, err := time.Parse("2006-01-02T15:04:05.000000+00:00", *c.TokenExpiry)
	if err != nil {
		// Try alternate format
		expiry, err = time.Parse(time.RFC3339, *c.TokenExpiry)
		if err != nil {
			return true
		}
	}
	return time.Now().UTC().Add(buffer).After(expiry)
}

// ConnectedAppStore provides CRUD for connected apps.
type ConnectedAppStore struct {
	db *DB
}

// NewConnectedAppStore creates a new ConnectedAppStore.
func NewConnectedAppStore(db *DB) *ConnectedAppStore {
	return &ConnectedAppStore{db: db}
}

// DB returns the underlying database connection.
func (s *ConnectedAppStore) DB() *DB {
	return s.db
}

// ── CRUD ──────────────────────────────────────────────────────────────

// Create creates a new connected app record.
func (s *ConnectedAppStore) Create(ctx context.Context, app *ConnectedApp) (*ConnectedApp, error) {
	now := nowUTC()
	app.CreatedAt = now
	app.UpdatedAt = now
	if app.Status == "" {
		app.Status = "active"
	}

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO connected_apps
		 (provider_id, name, client_id, client_secret, scopes,
		  access_token, refresh_token, token_expiry,
		  account_email, account_name, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		app.ProviderID, app.Name, app.ClientID, app.ClientSecret, app.Scopes,
		app.AccessToken, app.RefreshToken, app.TokenExpiry,
		app.AccountEmail, app.AccountName, app.Status, now, now)
	if err != nil {
		return nil, err
	}
	app.ID, _ = result.LastInsertId()
	return app, nil
}

// Get returns a connected app by ID.
func (s *ConnectedAppStore) Get(ctx context.Context, id int64) (*ConnectedApp, error) {
	var app ConnectedApp
	err := s.db.GetContext(ctx, &app, "SELECT * FROM connected_apps WHERE id = ?", id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &app, nil
}

// GetByProviderAndName returns a connected app by provider_id and name.
func (s *ConnectedAppStore) GetByProviderAndName(ctx context.Context, providerID, name string) (*ConnectedApp, error) {
	var app ConnectedApp
	err := s.db.GetContext(ctx, &app,
		"SELECT * FROM connected_apps WHERE provider_id = ? AND name = ?",
		providerID, name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &app, nil
}

// GetByName returns a connected app by name (for workflow token injection).
func (s *ConnectedAppStore) GetByName(ctx context.Context, name string) (*ConnectedApp, error) {
	var app ConnectedApp
	err := s.db.GetContext(ctx, &app,
		"SELECT * FROM connected_apps WHERE name = ? AND status = 'active'", name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &app, nil
}

// List returns all connected apps (tokens redacted via json:"-" tags).
func (s *ConnectedAppStore) List(ctx context.Context) ([]ConnectedApp, error) {
	var apps []ConnectedApp
	err := s.db.SelectContext(ctx, &apps,
		"SELECT * FROM connected_apps ORDER BY provider_id, name")
	return apps, err
}

// Delete deletes a connected app by ID.
func (s *ConnectedAppStore) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM connected_apps WHERE id = ?", id)
	return err
}

// ── Token Management ─────────────────────────────────────────────────

// UpdateTokens updates the access token, refresh token, and expiry for a connection.
func (s *ConnectedAppStore) UpdateTokens(ctx context.Context, id int64, accessToken, refreshToken string, expiry *string) error {
	now := nowUTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE connected_apps
		 SET access_token = ?, refresh_token = ?, token_expiry = ?,
		     status = 'active', updated_at = ?
		 WHERE id = ?`,
		accessToken, refreshToken, expiry, now, id)
	return err
}

// UpdateAccountInfo updates the account email and name after successful auth.
func (s *ConnectedAppStore) UpdateAccountInfo(ctx context.Context, id int64, email, name string) error {
	now := nowUTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE connected_apps SET account_email = ?, account_name = ?, updated_at = ?
		 WHERE id = ?`,
		email, name, now, id)
	return err
}

// SetStatus updates the status of a connection.
func (s *ConnectedAppStore) SetStatus(ctx context.Context, id int64, status string) error {
	now := nowUTC()
	_, err := s.db.ExecContext(ctx,
		"UPDATE connected_apps SET status = ?, updated_at = ? WHERE id = ?",
		status, now, id)
	return err
}

// GetFreshToken returns the access token for a connection, refreshing if needed.
// The caller provides a refreshFn that performs the actual token refresh via the OAuth provider.
// refreshFn receives (clientID, clientSecret, refreshToken) and returns (newAccessToken, newRefreshToken, expiry, error).
func (s *ConnectedAppStore) GetFreshToken(ctx context.Context, id int64,
	refreshFn func(clientID, clientSecret, refreshToken string) (string, string, *string, error),
) (string, error) {
	app, err := s.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if app == nil {
		return "", sql.ErrNoRows
	}
	if app.Status != "active" {
		return "", sql.ErrNoRows
	}

	// Check if token is still fresh (5-minute buffer)
	if !app.IsTokenExpired(5 * time.Minute) {
		return app.AccessToken, nil
	}

	// Token is expired or about to expire — refresh it
	if app.RefreshToken == "" {
		if err := s.SetStatus(ctx, id, "expired"); err != nil {
			return "", err
		}
		return "", sql.ErrNoRows
	}

	newAccess, newRefresh, expiry, err := refreshFn(app.ClientID, app.ClientSecret, app.RefreshToken)
	if err != nil {
		s.SetStatus(ctx, id, "expired")
		return "", err
	}

	if newRefresh == "" {
		newRefresh = app.RefreshToken
	}

	if err := s.UpdateTokens(ctx, id, newAccess, newRefresh, expiry); err != nil {
		return "", err
	}

	return newAccess, nil
}
