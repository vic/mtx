package matrix

import (
	"encoding/json"
	"errors"
	"fmt"

	"context"
	"maunium.net/go/mautrix/crypto"

	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/mockserver"
)

// TestUndecryptableIsShownLateOnce: batch 1 has the message without its key (placeholder, parked).
// Batch 2 brings the room key; the parked message is decrypted and delivered once, marked late.
func TestUndecryptableIsShownLateOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ts := mockserver.Create(t)
	addWhoami(ts)
	ts.Router.HandleFunc("POST /_matrix/client/v3/user/{userID}/filter", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"filter_id":"f1"}`)
	})

	aliceCli, aliceStore := ts.Login(t, ctx, aliceUser, "ALICE")
	am := aliceCli.Crypto.(*cryptohelper.CryptoHelper).Machine()
	require.NoError(t, aliceCli.StateStore.SetEncryptionEvent(ctx, testRoom, &event.EncryptionEventContent{Algorithm: id.AlgorithmMegolmV1}))
	require.NoError(t, aliceCli.StateStore.SetMembership(ctx, testRoom, aliceUser, event.MembershipJoin))
	require.NoError(t, aliceCli.StateStore.SetMembership(ctx, testRoom, bobUser, event.MembershipJoin))

	tok := tokenFor(t, ctx, ts, bobUser, "BOB")
	cfg := testConfig(t, ts.Server.URL, tok, "BOB")
	cfg.UserID = bobUser.String()
	bob, err := New(ctx, cfg)
	require.NoError(t, err)
	defer bob.Close()
	require.NoError(t, aliceStore.PutDevice(ctx, aliceUser, am.OwnIdentity()))
	require.NoError(t, aliceStore.PutDevice(ctx, bobUser, bob.helper.Machine().OwnIdentity()))

	encAny, err := aliceCli.Crypto.Encrypt(ctx, testRoom, event.EventMessage, &event.MessageEventContent{MsgType: event.MsgText, Body: "later"})
	require.NoError(t, err)
	timeline := mustRaw(map[string]any{"type": "m.room.encrypted", "event_id": "$enc1", "sender": aliceUser, "origin_server_ts": 1, "content": encAny})

	ts.Router.HandleFunc("GET /_matrix/client/v3/sync", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("since") == "s1" { // batch 2: the room key arrives
			var toDev []json.RawMessage
			for _, e := range ts.DeviceInbox[bobUser]["BOB"] {
				toDev = append(toDev, mustRaw(map[string]any{"sender": e.Sender, "type": e.Type.Type, "content": e.Content.VeryRaw}))
			}
			_, _ = w.Write(mustRaw(map[string]any{"next_batch": "s2", "to_device": map[string]any{"events": toDev}}))
			return
		}
		fmt.Fprintf(w, `{"next_batch":"s1","rooms":{"join":{%q:{"timeline":{"events":[%s]}}}}}`, testRoom, timeline)
	})

	var got []string
	var late []bool
	err = bob.Run(ctx, func(hctx context.Context, evt *event.Event, decErr error) error {
		if decErr != nil {
			got = append(got, "placeholder:"+evt.ID.String())
			late = append(late, false)
			return nil
		}
		got = append(got, evt.Content.AsMessage().Body)
		late = append(late, IsLate(hctx))
		if len(got) == 2 {
			cancel()
		}
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, []string{"placeholder:$enc1", "later"}, got)
	require.Equal(t, []bool{false, true}, late)

	var n int
	require.NoError(t, bob.db.RawDB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM pending_events`).Scan(&n))
	require.Equal(t, 0, n, "delivered late: nothing parked")
}

// TestRetryDecision: a withheld key is final (dropped, not an error, sync goes on); a missing
// session stays parked; any other failure is an error.
func TestRetryDecision(t *testing.T) {
	keep, err := retryDecision(crypto.NoSessionFound)
	require.Equal(t, retryKeep, keep)
	require.NoError(t, err)

	drop, err := retryDecision(fmt.Errorf("group session has been withheld: %w", crypto.ErrGroupSessionWithheld))
	require.Equal(t, retryDrop, drop)
	require.NoError(t, err)

	_, err = retryDecision(errors.New("bad mac"))
	require.Error(t, err)
}
