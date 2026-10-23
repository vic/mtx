package matrix

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/mockserver"
)

const inviteRoom = id.RoomID("!inv:mock.test")

// addInviteSync: since "" -> invite to inviteRoom; s1 -> s2 empty, then onEmpty.
func addInviteSync(ts *mockserver.MockServer, onEmpty func()) {
	ts.Router.HandleFunc("POST /_matrix/client/v3/user/{userID}/filter", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"filter_id":"f1"}`)
	})
	ts.Router.HandleFunc("GET /_matrix/client/v3/sync", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("since") == "s1" {
			onEmpty()
			fmt.Fprint(w, `{"next_batch":"s2"}`)
			return
		}
		fmt.Fprintf(w, `{"next_batch":"s1","rooms":{"invite":{%q:{"invite_state":{"events":[]}}}}}`, inviteRoom)
	})
}

// TestInviteAutoJoin: an invite in sync is joined by the loop.
func TestInviteAutoJoin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ts := mockserver.Create(t)
	addWhoami(ts)
	var mu sync.Mutex
	var joined []string
	ts.Router.HandleFunc("POST /_matrix/client/v3/rooms/{roomID}/join", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		joined = append(joined, r.PathValue("roomID"))
		mu.Unlock()
		fmt.Fprintf(w, `{"room_id":%q}`, r.PathValue("roomID"))
	})
	addInviteSync(ts, cancel)

	tok := tokenFor(t, ctx, ts, testUser, "DEV1")
	c, err := New(ctx, testConfig(t, ts.Server.URL, tok, "DEV1"))
	require.NoError(t, err)
	defer c.Close()
	require.ErrorIs(t, c.Run(ctx, func(context.Context, *event.Event, error) error { return nil }), context.Canceled)
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{string(inviteRoom)}, joined)
}

// TestInviteJoinFailureStopsLoud: a failed auto-join is an error, and the token does not move.
func TestInviteJoinFailureStopsLoud(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ts := mockserver.Create(t)
	addWhoami(ts)
	ts.Router.HandleFunc("POST /_matrix/client/v3/rooms/{roomID}/join", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"errcode":"M_FORBIDDEN","error":"no"}`)
	})
	addInviteSync(ts, cancel)

	tok := tokenFor(t, ctx, ts, testUser, "DEV1")
	cfg := testConfig(t, ts.Server.URL, tok, "DEV1")
	c, err := New(ctx, cfg)
	require.NoError(t, err)
	defer c.Close()
	err = c.Run(ctx, func(context.Context, *event.Event, error) error { return nil })
	require.ErrorContains(t, err, "auto-join")
	tokenNow, err := c.store().LoadNextBatch(ctx, testUser)
	require.NoError(t, err)
	require.Equal(t, "", tokenNow, "a failed batch must not move the token")
}
