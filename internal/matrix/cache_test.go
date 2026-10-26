package matrix

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"maunium.net/go/mautrix/id"
)

func TestCacheRoundTrip(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDB(filepath.Join(t.TempDir(), "mtx.db"))
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, saveRooms(ctx, db.RawDB, map[id.RoomID]string{"!a:s": "den"}))
	require.NoError(t, saveRooms(ctx, db.RawDB, map[id.RoomID]string{"!a:s": "den2"}))
	rooms, err := loadRooms(ctx, db.RawDB)
	require.NoError(t, err)
	require.Equal(t, map[id.RoomID]string{"!a:s": "den2"}, rooms)

	line := func(ev string, room id.RoomID, ts int64, text string) CachedLine {
		return CachedLine{Room: room, EventID: id.EventID(ev), TS: ts, Nick: "n", Text: text}
	}
	require.NoError(t, saveLines(ctx, db.RawDB, []CachedLine{
		line("$3", "!a:s", 3, "three"),
		line("$1", "!a:s", 1, "one"),
		line("$2", "!a:s", 2, "two"),
		line("$9", "!b:s", 5, "other room"),
	}))
	got, err := loadLines(ctx, db.RawDB, "!a:s", 2)
	require.NoError(t, err)
	require.Equal(t, []int64{2, 3}, []int64{got[0].TS, got[1].TS})

	require.NoError(t, saveLines(ctx, db.RawDB, []CachedLine{line("$2", "!a:s", 99, "two edited")}))
	all, err := loadLines(ctx, db.RawDB, "!a:s", 10)
	require.NoError(t, err)
	require.Len(t, all, 3)
	require.Equal(t, "two edited", all[1].Text)
	require.Equal(t, int64(2), all[1].TS, "an edit keeps the first stored time")
}

func TestKVAndInputHistory(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDB(filepath.Join(t.TempDir(), "mtx.db"))
	require.NoError(t, err)
	defer db.Close()
	c := &Client{db: db}

	require.NoError(t, c.SetKV(ctx, "focus", "#a"))
	require.NoError(t, c.SetKV(ctx, "focus", "#b"))
	v, err := c.KV(ctx, "focus")
	require.NoError(t, err)
	require.Equal(t, "#b", v)
	none, err := c.KV(ctx, "none")
	require.NoError(t, err)
	require.Equal(t, "", none)

	for i := range 102 {
		require.NoError(t, c.AddInput(ctx, "#a", "l"+strconv.Itoa(i)))
	}
	require.NoError(t, c.AddInput(ctx, "#b", "x"))
	in, err := c.Inputs(ctx)
	require.NoError(t, err)
	require.Len(t, in["#a"], 100)
	require.Equal(t, "l2", in["#a"][0])
	require.Equal(t, "l101", in["#a"][99])
	require.Equal(t, []string{"x"}, in["#b"])
}

// TestMediaSurvivesRestart: a cached line keeps its media JSON, and a store already at schema 7 (the
// column present) migrates again without error.
func TestMediaSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mtx.db")
	db, err := OpenDB(path)
	require.NoError(t, err)
	const media = `{"msgtype":"m.image","body":"a.png","url":"mxc://s/x"}`
	require.NoError(t, saveLines(ctx, db.RawDB, []CachedLine{{Room: "!a:s", EventID: "$e", TS: 1, Nick: "n", Text: "a.png", Media: media}}))
	require.NoError(t, db.Close())

	db, err = OpenDB(path) // a restart: the migration runs again on a v7 store
	require.NoError(t, err)
	defer db.Close()
	ls, err := loadLines(ctx, db.RawDB, "!a:s", 10)
	require.NoError(t, err)
	require.Len(t, ls, 1)
	require.Equal(t, media, ls[0].Media)

	_, err = db.RawDB.ExecContext(ctx, `PRAGMA user_version = 6`) // as if the store were a v6 one
	require.NoError(t, err)
	require.NoError(t, migrate(ctx, db.RawDB)) // the ADD COLUMN fails with a duplicate column, which is ignored
}
