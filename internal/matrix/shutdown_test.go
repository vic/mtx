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

// TestCancelMidBatchKeepsToken: ctx ends while a batch is being handled. The batch's token is not
// saved, so a restart delivers the whole batch again; events handled before the cancel are not
// handled twice.
func TestCancelMidBatchKeepsToken(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ts := mockserver.Create(t)
	addWhoami(ts)
	ts.Router.HandleFunc("POST /_matrix/client/v3/user/{userID}/filter", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"filter_id":"f1"}`)
	})
	ts.Router.HandleFunc("GET /_matrix/client/v3/sync", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"next_batch":"s1","rooms":{"join":{%q:{"timeline":{"events":[%s,%s]}}}}}`, testRoom,
			msgJSON("$a", "first"), msgJSON("$b", "second"))
	})
	tok := tokenFor(t, ctx, ts, testUser, "DEV1")
	cfg := testConfig(t, ts.Server.URL, tok, "DEV1")

	c1, err := New(ctx, cfg)
	require.NoError(t, err)
	err = c1.Run(ctx, func(_ context.Context, evt *event.Event, _ error) error {
		cancel() // shutdown while the first event is handled
		return nil
	})
	require.Error(t, err)
	tokenAfterCancel, err := c1.store().LoadNextBatch(context.Background(), testUser)
	require.NoError(t, err)
	require.Equal(t, "", tokenAfterCancel, "a batch cut short must not move the token")
	require.NoError(t, c1.Close())

	// restart: the batch comes again; both events are delivered once.
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	c2, err := New(ctx2, cfg)
	require.NoError(t, err)
	defer c2.Close()
	var got []string
	_ = c2.Run(ctx2, func(_ context.Context, evt *event.Event, _ error) error {
		got = append(got, evt.ID.String())
		if len(got) == 2 {
			cancel2()
		}
		return nil
	})
	require.Equal(t, []string{"$a", "$b"}, got)
}
