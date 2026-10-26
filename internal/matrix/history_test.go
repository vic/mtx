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

func msgJSON(id, body string) string {
	return fmt.Sprintf(`{"type":"m.room.message","event_id":%q,"sender":"@bob:mock.test","room_id":%q,"origin_server_ts":1,"content":{"msgtype":"m.text","body":%q}}`, id, testRoom, body)
}

// TestLoadThread: opening a thread delivers its root and then its replies oldest first, once per run.
func TestLoadThread(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ts := mockserver.Create(t)
	addWhoami(ts)
	ts.Router.HandleFunc("GET /_matrix/client/v3/rooms/{roomID}/event/{eventID}", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, msgJSON("$root", "root"))
	})
	ts.Router.HandleFunc("GET /_matrix/client/v1/rooms/{roomID}/relations/{eventID}/{relType}", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"chunk":[`+msgJSON("$r2", "two")+`,`+msgJSON("$r1", "one")+`]}`)
	})
	tok := tokenFor(t, ctx, ts, testUser, "DEV1")
	c, err := New(ctx, testConfig(t, ts.Server.URL, tok, "DEV1"))
	require.NoError(t, err)
	defer c.Close()
	var mu sync.Mutex
	var got []string
	h := func(_ context.Context, ev *event.Event, _ error) error {
		mu.Lock()
		got = append(got, ev.ID.String())
		mu.Unlock()
		return nil
	}
	require.NoError(t, c.LoadThread(ctx, "!r:mock.test", "$root", h))
	require.NoError(t, c.LoadThread(ctx, "!r:mock.test", "$root", h))
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"$root", "$r1", "$r2"}, got)
}

// TestLoadContext: a message's neighbours arrive as history, oldest first, with the message between them.
func TestLoadContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ts := mockserver.Create(t)
	addWhoami(ts)
	ts.Router.HandleFunc("GET /_matrix/client/v3/rooms/{roomID}/context/{eventID}", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"event":`+msgJSON("$e", "mid")+`,"events_before":[`+msgJSON("$b2", "two")+`,`+msgJSON("$b1", "one")+`],"events_after":[`+msgJSON("$a1", "after")+`]}`)
	})
	tok := tokenFor(t, ctx, ts, testUser, "DEV1")
	c, err := New(ctx, testConfig(t, ts.Server.URL, tok, "DEV1"))
	require.NoError(t, err)
	defer c.Close()
	var mu sync.Mutex
	var got []string
	h := func(_ context.Context, ev *event.Event, _ error) error {
		mu.Lock()
		got = append(got, ev.ID.String())
		mu.Unlock()
		return nil
	}
	require.NoError(t, c.LoadContext(ctx, "!r:mock.test", "$e", h))
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"$b1", "$b2", "$e", "$a1"}, got)
}
