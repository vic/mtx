package matrix

import (
	"context"
	"database/sql"
	"errors"

	"maunium.net/go/mautrix/id"
)

// CachedLine is one scrollback line kept on disk, so a start shows it before the server answers.
type CachedLine struct {
	Room       id.RoomID
	EventID    id.EventID
	TS         int64 // ms since epoch
	Nick, Text string
	Dim        bool
	Thread     string
}

// SaveRooms stores each room's channel label, replacing older labels.
func (c *Client) SaveRooms(ctx context.Context, labels map[id.RoomID]string) error {
	return saveRooms(ctx, c.db.RawDB, labels)
}

func saveRooms(ctx context.Context, db *sql.DB, labels map[id.RoomID]string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for r, l := range labels {
		if _, err := tx.ExecContext(ctx, `INSERT INTO cache_rooms(room_id, label) VALUES(?, ?) ON CONFLICT(room_id) DO UPDATE SET label = excluded.label`, r.String(), l); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// LoadRooms returns every cached room label.
func (c *Client) LoadRooms(ctx context.Context) (map[id.RoomID]string, error) {
	return loadRooms(ctx, c.db.RawDB)
}

func loadRooms(ctx context.Context, db *sql.DB) (map[id.RoomID]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT room_id, label FROM cache_rooms`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[id.RoomID]string{}
	for rows.Next() {
		var r, l string
		if err := rows.Scan(&r, &l); err != nil {
			return nil, err
		}
		out[id.RoomID(r)] = l
	}
	return out, rows.Err()
}

// SaveLines stores lines in one transaction; a line already stored (same event ID) gets the new text and keeps its time and thread.
func (c *Client) SaveLines(ctx context.Context, ls []CachedLine) error {
	return saveLines(ctx, c.db.RawDB, ls)
}

func saveLines(ctx context.Context, db *sql.DB, ls []CachedLine) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, l := range ls {
		if _, err := tx.ExecContext(ctx, `INSERT INTO cache_lines(event_id, room_id, ts, nick, text, dim, thread) VALUES(?, ?, ?, ?, ?, ?, ?) ON CONFLICT(event_id) DO UPDATE SET nick = excluded.nick, text = excluded.text, dim = excluded.dim`,
			l.EventID.String(), l.Room.String(), l.TS, l.Nick, l.Text, l.Dim, l.Thread); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// LoadLines returns the newest limit lines of room, oldest first.
func (c *Client) LoadLines(ctx context.Context, room id.RoomID, limit int) ([]CachedLine, error) {
	return loadLines(ctx, c.db.RawDB, room, limit)
}

func loadLines(ctx context.Context, db *sql.DB, room id.RoomID, limit int) ([]CachedLine, error) {
	rows, err := db.QueryContext(ctx, `SELECT event_id, ts, nick, text, dim, thread FROM (SELECT * FROM cache_lines WHERE room_id = ? ORDER BY ts DESC LIMIT ?) ORDER BY ts`, room.String(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CachedLine
	for rows.Next() {
		l := CachedLine{Room: room}
		var ev string
		if err := rows.Scan(&ev, &l.TS, &l.Nick, &l.Text, &l.Dim, &l.Thread); err != nil {
			return nil, err
		}
		l.EventID = id.EventID(ev)
		out = append(out, l)
	}
	return out, rows.Err()
}

// SetKV stores value under key (UI state such as the focused channel).
func (c *Client) SetKV(ctx context.Context, key, value string) error {
	_, err := c.db.RawDB.ExecContext(ctx, `INSERT INTO cache_kv(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// KV is the value stored under key, "" when none.
func (c *Client) KV(ctx context.Context, key string) (string, error) {
	var v string
	err := c.db.RawDB.QueryRowContext(ctx, `SELECT value FROM cache_kv WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// inputKeep is how many sent input lines per channel are kept.
const inputKeep = 100

// AddInput stores one sent input line of channel ch and drops that channel's lines beyond inputKeep.
func (c *Client) AddInput(ctx context.Context, ch, text string) error {
	if _, err := c.db.RawDB.ExecContext(ctx, `INSERT INTO cache_input(chan, text) VALUES(?, ?)`, ch, text); err != nil {
		return err
	}
	_, err := c.db.RawDB.ExecContext(ctx, `DELETE FROM cache_input WHERE chan = ? AND id NOT IN (SELECT id FROM cache_input WHERE chan = ? ORDER BY id DESC LIMIT ?)`, ch, ch, inputKeep)
	return err
}

// Inputs is every channel's sent input lines, oldest first.
func (c *Client) Inputs(ctx context.Context) (map[string][]string, error) {
	rows, err := c.db.RawDB.QueryContext(ctx, `SELECT chan, text FROM cache_input ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var ch, t string
		if err := rows.Scan(&ch, &t); err != nil {
			return nil, err
		}
		out[ch] = append(out[ch], t)
	}
	return out, rows.Err()
}
