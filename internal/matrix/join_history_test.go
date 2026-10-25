package matrix

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/mockserver"
)

// TestJoinLaterGetsHistory: a room that appears after the first sync is backfilled with
// /messages from its prev_batch (history), then its timeline (history), then live events.
func TestJoinLaterGetsHistory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ts := mockserver.Create(t)
	addWhoami(ts)
	ts.Router.HandleFunc("POST /_matrix/client/v3/user/{userID}/filter", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"filter_id":"f1"}`)
	})
	var mu sync.Mutex
	var froms []string
	ts.Router.HandleFunc("GET /_matrix/client/v3/rooms/{roomID}/messages", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		froms = append(froms, r.URL.Query().Get("from"))
		mu.Unlock()
		fmt.Fprintf(w, `{"start":"p0","chunk":[%s]}`, msgJSON("$h1", "older"))
	})
	ts.Router.HandleFunc("GET /_matrix/client/v3/sync", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("since") {
		case "":
			fmt.Fprint(w, `{"next_batch":"s1"}`)
		case "s1":
			fmt.Fprintf(w, `{"next_batch":"s2","rooms":{"join":{%q:{"timeline":{"prev_batch":"p0","events":[%s]}}}}}`, testRoom, msgJSON("$t1", "now"))
		case "s2":
			fmt.Fprintf(w, `{"next_batch":"s3","rooms":{"join":{%q:{"timeline":{"events":[%s]}}}}}`, testRoom, msgJSON("$t2", "live"))
		default:
			cancel()
			fmt.Fprint(w, `{"next_batch":"s4"}`)
		}
	})

	tok := tokenFor(t, ctx, ts, testUser, "DEV1")
	c, err := New(ctx, testConfig(t, ts.Server.URL, tok, "DEV1"))
	require.NoError(t, err)
	defer c.Close()

	var ids []string
	var hist []bool
	require.ErrorIs(t, c.Run(ctx, func(hctx context.Context, evt *event.Event, _ error) error {
		ids = append(ids, evt.ID.String())
		hist = append(hist, IsHistory(hctx))
		return nil
	}), context.Canceled)
	require.Equal(t, []string{"$t1", "$t2"}, ids) // no startup backfill: $h1 loads when the room is opened
	require.Equal(t, []bool{true, false}, hist)
	mu.Lock()
	defer mu.Unlock()
	require.Empty(t, froms, "no backfill at join: history loads when the room opens")
}
