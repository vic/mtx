//go:build conduit

package matrix

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// conduitEnv logs the test user of side in as a fresh device, so each test starts with an empty crypto store
// and no device keys left on the server from an earlier test.
func conduitEnv(t *testing.T, side string) Config {
	t.Helper()
	get := func(k string) string {
		v := os.Getenv(k)
		require.NotEmpty(t, v, k)
		return v
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	dev := fmt.Sprintf("%s%d", side, time.Now().UnixNano())
	return loginDevice(t, ctx, get("MTX_CONDUIT_HOMESERVER"), id.UserID(get("MTX_CONDUIT_"+side+"_USER")),
		get("MTX_CONDUIT_"+side+"_PASSWORD"), dev)
}

// encryptedRoom creates an encrypted private room from alice's client, invites bob and accepts as bob.
func encryptedRoom(t *testing.T, ctx context.Context, alice, bob *Client) id.RoomID {
	t.Helper()
	var invite []id.UserID
	if bob != nil {
		invite = []id.UserID{bob.cli.UserID}
	}
	resp, err := alice.MX().CreateRoom(ctx, &mautrix.ReqCreateRoom{Invite: invite, Preset: "private_chat",
		InitialState: []*event.Event{{Type: event.StateEncryption, StateKey: ptr(""),
			Content: event.Content{Parsed: &event.EncryptionEventContent{Algorithm: id.AlgorithmMegolmV1}}}}})
	require.NoError(t, err)
	if bob != nil {
		require.NoError(t, bob.Accept(ctx, resp.RoomID))
	}
	return resp.RoomID
}

// collect runs c.Run in the background and hands each delivered event to got until stop.
func collect(ctx context.Context, c *Client, got chan<- *event.Event) (stop func(), done <-chan error) {
	runCtx, cancel := context.WithCancel(ctx)
	d := make(chan error, 1)
	go func() {
		d <- c.Run(runCtx, func(_ context.Context, evt *event.Event, decErr error) error {
			if decErr == nil {
				select {
				case got <- evt:
				default:
				}
			}
			return nil
		})
	}()
	return cancel, d
}

// waitFor returns the first event matching ok, or fails after d.
func waitFor(t *testing.T, got <-chan *event.Event, d time.Duration, ok func(*event.Event) bool) *event.Event {
	t.Helper()
	deadline := time.After(d)
	for {
		select {
		case e := <-got:
			if ok(e) {
				return e
			}
		case <-deadline:
			t.Fatal("event not delivered in time")
		}
	}
}
