package matrix

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/mockserver"
)

// TestSyncFeedsStateStore: the encryption state from sync reaches the client's state store, so
// sends to that room are encrypted (IsEncrypted is what SendMessageEvent checks).
func TestSyncFeedsStateStore(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ts := mockserver.Create(t)
	addWhoami(ts)
	ts.Router.HandleFunc("POST /_matrix/client/v3/user/{userID}/filter", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"filter_id":"f1"}`)
	})
	ts.Router.HandleFunc("GET /_matrix/client/v3/sync", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("since") == "s1" {
			cancel()
			fmt.Fprint(w, `{"next_batch":"s2"}`)
			return
		}
		fmt.Fprintf(w, `{"next_batch":"s1","rooms":{"join":{%q:{"state":{"events":[{"type":"m.room.encryption","state_key":"","sender":"@alice:mock.test","content":{"algorithm":"m.megolm.v1.aes-sha2"}}]},"timeline":{"events":[]}}}}}`, testRoom)
	})
	tok := tokenFor(t, ctx, ts, testUser, "DEV1")
	c, err := New(ctx, testConfig(t, ts.Server.URL, tok, "DEV1"))
	require.NoError(t, err)
	defer c.Close()
	require.ErrorIs(t, c.Run(ctx, func(context.Context, *event.Event, error) error { return nil }), context.Canceled)
	enc, err := c.cli.StateStore.IsEncrypted(context.Background(), testRoom)
	require.NoError(t, err)
	require.True(t, enc, "sends to this room must be encrypted")
}
