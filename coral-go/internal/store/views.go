package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CustomView represents a user-created sidebar view.
type CustomView struct {
	ID        int64  `db:"id" json:"id"`
	Name      string `db:"name" json:"name"`
	Prompt    string `db:"prompt" json:"prompt"`
	HTML      string `db:"html" json:"html"`
	TabOrder  int    `db:"tab_order" json:"tab_order"`
	Scope     string `db:"scope" json:"scope"`
	CreatedAt string `db:"created_at" json:"created_at"`
	UpdatedAt string `db:"updated_at" json:"updated_at"`
}

// ViewStore provides custom view CRUD operations.
type ViewStore struct {
	db *DB
}

// NewViewStore creates a new ViewStore.
func NewViewStore(db *DB) *ViewStore {
	return &ViewStore{db: db}
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// ListViews returns all custom views ordered by tab_order.
func (s *ViewStore) ListViews(ctx context.Context) ([]CustomView, error) {
	var views []CustomView
	err := s.db.SelectContext(ctx, &views,
		"SELECT id, name, prompt, html, tab_order, scope, created_at, updated_at FROM custom_views ORDER BY tab_order, id")
	return views, err
}

// GetView returns a single view by ID.
func (s *ViewStore) GetView(ctx context.Context, id int64) (*CustomView, error) {
	var v CustomView
	err := s.db.GetContext(ctx, &v,
		"SELECT id, name, prompt, html, tab_order, scope, created_at, updated_at FROM custom_views WHERE id = ?", id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &v, err
}

// CreateView inserts a new custom view.
func (s *ViewStore) CreateView(ctx context.Context, v *CustomView) (int64, error) {
	ts := now()
	result, err := s.db.ExecContext(ctx,
		"INSERT INTO custom_views (name, prompt, html, tab_order, scope, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		v.Name, v.Prompt, v.HTML, v.TabOrder, v.Scope, ts, ts)
	if err != nil {
		return 0, fmt.Errorf("insert view: %w", err)
	}
	return result.LastInsertId()
}

// UpdateView updates an existing view.
func (s *ViewStore) UpdateView(ctx context.Context, id int64, v *CustomView) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE custom_views SET name = ?, prompt = ?, html = ?, tab_order = ?, scope = ?, updated_at = ? WHERE id = ?",
		v.Name, v.Prompt, v.HTML, v.TabOrder, v.Scope, now(), id)
	return err
}

// DeleteView removes a view by ID.
func (s *ViewStore) DeleteView(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM custom_views WHERE id = ?", id)
	return err
}
