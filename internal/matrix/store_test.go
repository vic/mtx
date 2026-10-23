package matrix

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSchemaVersionAndPrune: reopening keeps the schema version; seen IDs are capped at seenKeep.
func TestSchemaVersionAndPrune(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mtx.db")
	db, err := OpenDB(path)
	require.NoError(t, err)
	var v int
	require.NoError(t, db.RawDB.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&v))
	require.Equal(t, schemaVersion, v)
	require.NoError(t, db.Close())

	db, err = OpenDB(path) // reopen: migrate is a no-op
	require.NoError(t, err)
	defer db.Close()
	_, err = db.RawDB.ExecContext(ctx, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i < ?)
		INSERT INTO seen_events(event_id) SELECT '$' || i FROM n`, seenKeep+5)
	require.NoError(t, err)
	require.NoError(t, pruneSeen(ctx, db.RawDB))
	var n int
	require.NoError(t, db.RawDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM seen_events`).Scan(&n))
	require.Equal(t, seenKeep, n)
}
