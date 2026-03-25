package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

// WithTx runs fn inside a transaction, handling Begin/Rollback/Commit boilerplate.
func (d *DB) WithTx(ctx context.Context, fn func(*sqlx.Tx) error) error {
	tx, err := d.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// dynamicUpdate builds and executes an UPDATE statement from a map of field values,
// filtering to only allowed columns. Bool values in boolCols are converted to 0/1 for SQLite.
// If addUpdatedAt is true, "updated_at" is set to the current time.
func dynamicUpdate(ctx context.Context, db *DB, table string, id interface{}, fields map[string]interface{}, allowed map[string]bool, boolCols map[string]bool, addUpdatedAt bool) error {
	var sets []string
	var args []interface{}
	if addUpdatedAt {
		sets = append(sets, "updated_at = ?")
		args = append(args, nowUTC())
	}
	for k, v := range fields {
		if !allowed[k] {
			continue
		}
		if boolCols[k] {
			if b, ok := v.(bool); ok {
				if b {
					v = 1
				} else {
					v = 0
				}
			}
		}
		sets = append(sets, fmt.Sprintf("%s = ?", k))
		args = append(args, v)
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, id)
	_, err := db.ExecContext(ctx,
		fmt.Sprintf("UPDATE %s SET %s WHERE id = ?", table, strings.Join(sets, ", ")),
		args...)
	return err
}

// sessionFilter returns a SQL fragment and args for optional session_id filtering.
// When sessionID is non-nil, returns " AND session_id = ?" with the value.
// When nil, returns empty string and nil args.
func sessionFilter(sessionID *string) (string, []interface{}) {
	if sessionID != nil {
		return " AND session_id = ?", []interface{}{*sessionID}
	}
	return "", nil
}
