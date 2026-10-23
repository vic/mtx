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

// TestFirstSyncBackfillsOldestFirst: the first sync replays events before the timeline (oldest
// first), then the timeline; all of it is history. A resumed sync replays nothing.
func TestFirstSyncBackfillsOldestFirst(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ts := mockserver.Create(t)
	addWhoami(ts)
	ts.Router.HandleFunc("POST /_matrix/client/v3/user/{userID}/filter", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"filter_id":"f1"}`)
	})
	ts.Router.HandleFunc("GET /_matrix/client/v3/rooms/{roomID}/messages", func(w http.ResponseWriter, r *http.Request) {
		// backward pagination returns newest first
		fmt.Fprintf(w, `{"start":"p0","chunk":[%s,%s]}`, msgJSON("$h2", "two"), msgJSON("$h1", "one"))
	})
	ts.Router.HandleFunc("GET /_matrix/client/v3/sync", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("since") == "s1" {
			fmt.Fprint(w, `{"next_batch":"s2"}`)
			cancel()
			return
		}
		fmt.Fprintf(w, `{"next_batch":"s1","rooms":{"join":{%q:{"timeline":{"prev_batch":"p0","events":[%s]}}}}}`,
			testRoom, msgJSON("$t1", "now"))
	})

	tok := tokenFor(t, ctx, ts, testUser, "DEV1")
	c, err := New(ctx, testConfig(t, ts.Server.URL, tok, "DEV1"))
	require.NoError(t, err)
	defer c.Close()

	var mu sync.Mutex
	var ids []string
	var hist []bool
	require.ErrorIs(t, c.Run(ctx, func(ctx context.Context, evt *event.Event, _ error) error {
		mu.Lock()
		defer mu.Unlock()
		ids = append(ids, evt.ID.String())
		hist = append(hist, IsHistory(ctx))
		return nil
	}), context.Canceled)
	require.Equal(t, []string{"$h1", "$h2", "$t1"}, ids)
	require.Equal(t, []bool{true, true, true}, hist)
}

func msgJSON(id, body string) string {
	return fmt.Sprintf(`{"type":"m.room.message","event_id":%q,"sender":"@bob:mock.test","room_id":%q,"origin_server_ts":1,"content":{"msgtype":"m.text","body":%q}}`, id, testRoom, body)
}
