package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/mockserver"
)

const (
	testUser = id.UserID("@alice:mock.test")
	testRoom = "!r:mock.test"
)

// tokenFor logs user/device in on the mock server and returns the access token.
func tokenFor(t *testing.T, ctx context.Context, ts *mockserver.MockServer, user id.UserID, dev id.DeviceID) string {
	t.Helper()
	cli, err := mautrix.NewClient(ts.Server.URL, "", "")
	require.NoError(t, err)
	cli.Client = ts.Server.Client()
	resp, err := cli.Login(ctx, &mautrix.ReqLogin{
		Type:       mautrix.AuthTypePassword,
		Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: user.String()},
		DeviceID:   dev, Password: "password",
	})
	require.NoError(t, err)
	return resp.AccessToken
}

// addWhoami gives the mock server a /whoami that reports the token's user and device.
func addWhoami(ts *mockserver.MockServer) {
	ts.Router.HandleFunc("GET /_matrix/client/v3/account/whoami", func(w http.ResponseWriter, r *http.Request) {
		ud := ts.AccessTokenToUserID[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
		_ = json.NewEncoder(w).Encode(mautrix.RespWhoami{UserID: ud.UserID, DeviceID: ud.DeviceID})
	})
}

// addScriptedSync: since "" -> s1; s1 -> s2 with message $e1; s2 -> s3 empty.
// onSince is called with each since value served.
func addScriptedSync(ts *mockserver.MockServer, onSince func(string)) {
	ts.Router.HandleFunc("POST /_matrix/client/v3/user/{userID}/filter", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"filter_id":"f1"}`)
	})
	ts.Router.HandleFunc("GET /_matrix/client/v3/sync", func(w http.ResponseWriter, r *http.Request) {
		since := r.URL.Query().Get("since")
		onSince(since)
		next, events := "s1", ""
		switch since {
		case "s1":
			next = "s2"
			events = `{"type":"m.room.message","event_id":"$e1","sender":"@bob:mock.test","origin_server_ts":1,"content":{"msgtype":"m.text","body":"hello"}}`
		case "s2":
			next = "s3"
		}
		fmt.Fprintf(w, `{"next_batch":%q,"rooms":{"join":{%q:{"timeline":{"events":[%s]}}}}}`, next, testRoom, events)
	})
}

func testConfig(t *testing.T, url string, tok string, dev string) Config {
	t.Helper()
	return Config{Homeserver: url, UserID: testUser.String(), AccessToken: tok, DeviceID: dev,
		PickleKey: "pickle-test", DBPath: filepath.Join(t.TempDir(), "store", "mtx.db")}
}

func TestLoadConfigMissingNamesVariable(t *testing.T) {
	env := map[string]string{"MTX_HOMESERVER": "https://h.example", "MTX_USER_ID": "@a:h.example", "MTX_ACCESS_TOKEN": "t", "MTX_PICKLE_KEY": "p"}
	_, err := LoadConfig(func(k string) string { return env[k] }, "/data")
	require.ErrorContains(t, err, "MTX_DEVICE_ID")
}

func TestLoadConfigDefaultDB(t *testing.T) {
	env := map[string]string{"MTX_HOMESERVER": "https://h.example", "MTX_USER_ID": "@a:h.example", "MTX_ACCESS_TOKEN": "t", "MTX_DEVICE_ID": "D", "MTX_PICKLE_KEY": "p"}
	c, err := LoadConfig(func(k string) string { return env[k] }, "/data")
	require.NoError(t, err)
	require.Equal(t, filepath.Join("/data", "mtx", "mtx.db"), c.DBPath)
}

func TestOpenDBRejectsWrongMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mtx.db")
	require.NoError(t, os.WriteFile(path, nil, 0o644))
	_, err := OpenDB(path)
	require.ErrorContains(t, err, "want 600")
}

func TestOpenDBModes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d", "mtx.db")
	db, err := OpenDB(path)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	st, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), st.Mode().Perm())
	dir, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), dir.Mode().Perm())
}

func TestNewWhoamiMismatch(t *testing.T) {
	ctx := context.Background()
	ts := mockserver.Create(t)
	addWhoami(ts)
	tok := tokenFor(t, ctx, ts, testUser, "DEV1")
	_, err := New(ctx, testConfig(t, ts.Server.URL, tok, "WRONG"))
	require.ErrorContains(t, err, "device mismatch")
}

// Crash in the handler on $e1, restart, $e1 delivered once, token not moved past the failed batch.
func TestRunCrashThenResume(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ts := mockserver.Create(t)
	addWhoami(ts)
	var served []string
	addScriptedSync(ts, func(s string) {
		served = append(served, s)
		if s == "s3" {
			cancel()
		}
	})
	tok := tokenFor(t, ctx, ts, testUser, "DEV1")
	cfg := testConfig(t, ts.Server.URL, tok, "DEV1")

	c1, err := New(ctx, cfg)
	require.NoError(t, err)
	var seen []string
	err = c1.Run(ctx, func(_ context.Context, evt *event.Event, _ error) error {
		seen = append(seen, evt.ID.String())
		return errors.New("crash")
	})
	require.ErrorContains(t, err, "crash")
	require.NoError(t, c1.Close())
	require.Equal(t, []string{"$e1"}, seen)

	// restart on the same store
	c2, err := New(ctx, cfg)
	require.NoError(t, err)
	defer c2.Close()
	seen = nil
	err = c2.Run(ctx, func(_ context.Context, evt *event.Event, _ error) error {
		seen = append(seen, evt.ID.String())
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, []string{"$e1"}, seen)
	require.Equal(t, []string{"", "s1", "s1", "s2", "s3"}, served)
}
