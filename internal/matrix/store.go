package matrix

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix/id"
	_ "modernc.org/sqlite"
)

// sidecars are SQLite's WAL files next to the DB; they inherit the DB's security.
var sidecars = []string{"-wal", "-shm", "-journal"}

// OpenDB creates or opens the store at path. Dir 0700, file 0600. Fails if an existing
// DB or sidecar is not 0600.
func OpenDB(path string) (*dbutil.Database, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("store dir: %w", err)
	}
	if err := checkMode(path); err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, fmt.Errorf("create store: %w", err)
		}
		_ = f.Close()
	}
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	db, err := dbutil.NewWithDB(raw, "sqlite3")
	if err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("store dialect: %w", err)
	}
	if err := migrate(context.Background(), db.RawDB); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store migrate: %w", err)
	}
	// WAL sidecars appear after the first write; set them to 0600 right away.
	for _, s := range sidecars {
		if _, err := os.Stat(path + s); err == nil {
			if err := os.Chmod(path+s, 0o600); err != nil {
				_ = db.Close()
				return nil, fmt.Errorf("chmod %s: %w", s, err)
			}
		}
	}
	return db, nil
}

// checkMode fails if the DB or a sidecar exists with a mode other than 0600.
func checkMode(path string) error {
	for _, p := range append([]string{path}, sidecarPaths(path)...) {
		st, err := os.Stat(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("stat %s: %w", p, err)
		}
		if m := st.Mode().Perm(); m != 0o600 {
			return fmt.Errorf("%s has mode %o, want 600", p, m)
		}
	}
	return nil
}

func sidecarPaths(path string) []string {
	var out []string
	for _, s := range sidecars {
		out = append(out, path+s)
	}
	return out
}

// SyncStore is mautrix's SyncStore on the same DB: filter ID and next_batch per user.
type SyncStore struct{ db *sql.DB }

func NewSyncStore(db *dbutil.Database) *SyncStore { return &SyncStore{db: db.RawDB} }

func (s *SyncStore) SaveFilterID(ctx context.Context, user id.UserID, v string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sync_state(user_id, filter_id) VALUES(?, ?)
		ON CONFLICT(user_id) DO UPDATE SET filter_id = excluded.filter_id`, user.String(), v)
	return err
}

func (s *SyncStore) LoadFilterID(ctx context.Context, user id.UserID) (string, error) {
	var v sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT filter_id FROM sync_state WHERE user_id = ?`, user.String()).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v.String, err
}

func (s *SyncStore) SaveNextBatch(ctx context.Context, user id.UserID, v string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sync_state(user_id, next_batch) VALUES(?, ?)
		ON CONFLICT(user_id) DO UPDATE SET next_batch = excluded.next_batch`, user.String(), v)
	return err
}

func (s *SyncStore) LoadNextBatch(ctx context.Context, user id.UserID) (string, error) {
	var v sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT next_batch FROM sync_state WHERE user_id = ?`, user.String()).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v.String, err
}

// schemaVersion is PRAGMA user_version. Each step is one migration; a newer DB than this is an error.
const schemaVersion = 6 // 2: rooms_seen (a room first seen in a sync gets its history); 3: session (sealed login); 4: no table, accepts stores from an uncommitted build that had room_backfill; 5: cache_rooms, cache_lines (start from disk), cache_kv, cache_input; 6: cache_kv, cache_input

// migrate brings mtx's own tables to schemaVersion. cryptohelper creates its tables on Init.
func migrate(ctx context.Context, db *sql.DB) error {
	var v int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	if v > schemaVersion {
		return fmt.Errorf("store schema %d is newer than this mtx (%d)", v, schemaVersion)
	}
	if v == schemaVersion {
		return nil
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS sync_state (
	user_id    TEXT PRIMARY KEY,
	filter_id  TEXT,
	next_batch TEXT
);
CREATE TABLE IF NOT EXISTS pending_events (
	event_id TEXT PRIMARY KEY,
	body     TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS seen_events (
	event_id TEXT PRIMARY KEY
);
CREATE TABLE IF NOT EXISTS session (
	id     INTEGER PRIMARY KEY CHECK (id = 1),
	sealed BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS rooms_seen (
	room_id TEXT PRIMARY KEY
);
CREATE TABLE IF NOT EXISTS cache_rooms (
	room_id TEXT PRIMARY KEY,
	label   TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS cache_lines (
	event_id TEXT PRIMARY KEY,
	room_id  TEXT NOT NULL,
	ts       INTEGER NOT NULL,
	nick     TEXT NOT NULL,
	text     TEXT NOT NULL,
	dim      INTEGER NOT NULL,
	thread   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS cache_lines_room_ts ON cache_lines(room_id, ts);
CREATE TABLE IF NOT EXISTS cache_kv (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS cache_input (
	id   INTEGER PRIMARY KEY AUTOINCREMENT,
	chan TEXT NOT NULL,
	text TEXT NOT NULL
);`); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion))
	return err
}

// seenKeep is how many delivered event IDs are kept for redelivery dedupe; a replay window never
// reaches back this far.
const seenKeep = 10000

// pruneSeen drops the oldest delivered IDs beyond seenKeep.
func pruneSeen(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `DELETE FROM seen_events WHERE rowid NOT IN (SELECT rowid FROM seen_events ORDER BY rowid DESC LIMIT ?)`, seenKeep)
	return err
}

// RemoveStore deletes the store at path and its WAL sidecars. Missing files are fine.
func RemoveStore(path string) error {
	for _, p := range append([]string{path}, withSuffix(path, sidecars)...) {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", p, err)
		}
	}
	return nil
}

func withSuffix(path string, suffixes []string) []string {
	out := make([]string, 0, len(suffixes))
	for _, s := range suffixes {
		out = append(out, path+s)
	}
	return out
}
