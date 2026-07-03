package db

import (
	"context"
	"database/sql"
	"fmt"
)

// verifyPostCommit checks that the DB is reachable after commit.
// Used for stale connection or Dolt commit failure detection.
func verifyPostCommit(ctx context.Context, db *sql.DB) error {
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connection ping: %w", err)
	}
	return nil
}
