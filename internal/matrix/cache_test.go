package matrix

import (
	"context"
	"path/filepath"
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

	require.NoError(t, saveLines(ctx, db.RawDB, []CachedLine{line("$2", "!a:s", 2, "two edited")}))
	all, err := loadLines(ctx, db.RawDB, "!a:s", 10)
	require.NoError(t, err)
	require.Len(t, all, 3)
	require.Equal(t, "two edited", all[1].Text)
}
