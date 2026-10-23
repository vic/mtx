package matrix

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/mockserver"
)

const (
	bobUser   = id.UserID("@bob:mock.test")
	aliceUser = testUser
)

func mustRaw(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// addEncryptedSync serves: since "" -> bob's to-device room keys + one encrypted timeline event;
// since s1 -> s2 empty, then onEmpty runs.
func addEncryptedSync(ts *mockserver.MockServer, enc *event.EncryptedEventContent, onEmpty func()) {
	ts.Router.HandleFunc("POST /_matrix/client/v3/user/{userID}/filter", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"filter_id":"f1"}`)
	})
	ts.Router.HandleFunc("GET /_matrix/client/v3/sync", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("since") == "s1" {
			onEmpty()
			fmt.Fprintf(w, `{"next_batch":"s2","rooms":{"join":{%q:{"timeline":{"events":[]}}}}}`, testRoom)
			return
		}
		var toDev []json.RawMessage
		for _, e := range ts.DeviceInbox[bobUser]["BOB"] {
			toDev = append(toDev, mustRaw(map[string]any{"sender": e.Sender, "type": e.Type.Type, "content": e.Content.VeryRaw}))
		}
		timeline := []json.RawMessage{mustRaw(map[string]any{"type": "m.room.encrypted", "event_id": "$enc1", "sender": aliceUser, "origin_server_ts": 1, "content": enc})}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"next_batch": "s1",
			"to_device":  map[string]any{"events": toDev},
			"rooms":      map[string]any{"join": map[string]any{testRoom: map[string]any{"timeline": map[string]any{"events": timeline}}}},
		})
	})
}

// TestEncryptedSendReceive: alice sends in an encrypted room; bob's Run decrypts it.
func TestEncryptedSendReceive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ts := mockserver.Create(t)
	addWhoami(ts)

	// alice: in-memory store (test-only sender).
	aliceCli, aliceStore := ts.Login(t, ctx, aliceUser, "ALICE")
	am := aliceCli.Crypto.(*cryptohelper.CryptoHelper).Machine()
	ss := aliceCli.StateStore
	require.NoError(t, ss.SetEncryptionEvent(ctx, testRoom, &event.EncryptionEventContent{Algorithm: id.AlgorithmMegolmV1}))
	require.NoError(t, ss.SetMembership(ctx, testRoom, aliceUser, event.MembershipJoin))
	require.NoError(t, ss.SetMembership(ctx, testRoom, bobUser, event.MembershipJoin))

	// bob: our client on a file store. New uploads his identity to the server.
	bobTok := tokenFor(t, ctx, ts, bobUser, "BOB")
	cfg := Config{Homeserver: ts.Server.URL, UserID: bobUser.String(), AccessToken: bobTok, DeviceID: "BOB",
		PickleKey: "pickle-test", DBPath: filepath.Join(t.TempDir(), "bob", "mtx.db")}
	bob, err := New(ctx, cfg)
	require.NoError(t, err)
	defer bob.Close()

	require.NoError(t, aliceStore.PutDevice(ctx, aliceUser, am.OwnIdentity()))
	require.NoError(t, aliceStore.PutDevice(ctx, bobUser, bob.helper.Machine().OwnIdentity()))

	encAny, err := aliceCli.Crypto.Encrypt(ctx, testRoom, event.EventMessage, &event.MessageEventContent{MsgType: event.MsgText, Body: "hello bob"})
	require.NoError(t, err)
	require.NotEmpty(t, ts.DeviceInbox[bobUser]["BOB"], "room key must reach bob's device")

	addEncryptedSync(ts, encAny, cancel)

	var got []string
	err = bob.Run(ctx, func(_ context.Context, evt *event.Event, decErr error) error {
		require.NoError(t, decErr)
		got = append(got, evt.Content.AsMessage().Body)
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, []string{"hello bob"}, got)
}
