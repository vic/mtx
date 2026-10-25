package matrix

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"maunium.net/go/mautrix/mockserver"
)

func TestSessionRoundTripAndWrongPickle(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDB(filepath.Join(t.TempDir(), "mtx.db"))
	require.NoError(t, err)
	defer db.Close()

	_, ok, err := loadSession(ctx, db.RawDB, "pk")
	require.NoError(t, err)
	require.False(t, ok)

	want := savedSession{AccessToken: "tok-1", DeviceID: "mtx-ABCDEFGH"}
	require.NoError(t, saveSession(ctx, db.RawDB, "pk", want))
	got, ok, err := loadSession(ctx, db.RawDB, "pk")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, want, got)

	_, _, err = loadSession(ctx, db.RawDB, "other")
	require.ErrorContains(t, err, "wrong MTX_PICKLE_KEY")
}

func TestSessionRowIsNotPlaintext(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDB(filepath.Join(t.TempDir(), "mtx.db"))
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, saveSession(ctx, db.RawDB, "pk", savedSession{AccessToken: "secret-token", DeviceID: "D"}))
	var blob []byte
	require.NoError(t, db.RawDB.QueryRowContext(ctx, `SELECT sealed FROM session`).Scan(&blob))
	require.NotContains(t, string(blob), "secret-token")
}

func TestNewUsesStoredSession(t *testing.T) {
	ctx := context.Background()
	ts := mockserver.Create(t)
	addWhoami(ts)
	tok := tokenFor(t, ctx, ts, testUser, "DEV1")
	cfg := testConfig(t, ts.Server.URL, tok, "")
	c, err := New(ctx, cfg)
	require.NoError(t, err)
	require.NoError(t, c.Close())

	cfg.Homeserver, cfg.UserID, cfg.AccessToken = "", "", ""
	c, err = New(ctx, cfg)
	require.NoError(t, err)
	defer c.Close()
	require.Equal(t, "DEV1", string(c.cli.DeviceID))
	require.Equal(t, testUser.String(), c.user.String())
}

func TestNewWithoutTokenOrSession(t *testing.T) {
	cfg := testConfig(t, "https://h.example", "", "")
	_, err := New(context.Background(), cfg)
	require.ErrorContains(t, err, "no stored session")
}

func TestLoadConfigPickleOnly(t *testing.T) {
	env := map[string]string{"MTX_PICKLE_KEY": "pk"}
	c, err := LoadConfig(func(k string) string { return env[k] }, "/data")
	require.NoError(t, err)
	require.Equal(t, "pk", c.PickleKey)
	require.Empty(t, c.Homeserver)
}
