//go:build conduit

package matrix

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// conduitEnv reads the MTX_CONDUIT_* session of the test users (set by CI or a local run).
func conduitEnv(t *testing.T, side string) Config {
	t.Helper()
	get := func(k string) string {
		v := os.Getenv(k)
		require.NotEmpty(t, v, k)
		return v
	}
	return Config{Homeserver: get("MTX_CONDUIT_HOMESERVER"), UserID: get("MTX_CONDUIT_" + side + "_USER"),
		AccessToken: get("MTX_CONDUIT_" + side + "_TOKEN"), DeviceID: get("MTX_CONDUIT_" + side + "_DEVICE"),
		PickleKey: "pickle-conduit", DBPath: filepath.Join(t.TempDir(), side, "mtx.db")}
}

// encryptedRoom creates an encrypted private room from alice's client and invites bob.
func encryptedRoom(t *testing.T, ctx context.Context, alice *Client, bobUser string) id.RoomID {
	t.Helper()
	var invite []id.UserID
	if bobUser != "" {
		invite = []id.UserID{id.UserID(bobUser)}
	}
	resp, err := alice.MX().CreateRoom(ctx, &mautrix.ReqCreateRoom{Invite: invite, Preset: "private_chat",
		InitialState: []*event.Event{{Type: event.StateEncryption, StateKey: ptr(""),
			Content: event.Content{Parsed: &event.EncryptionEventContent{Algorithm: id.AlgorithmMegolmV1}}}}})
	require.NoError(t, err)
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
