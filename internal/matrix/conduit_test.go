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

// TestConduitEncryptedRoundTrip runs against a real homeserver (MTX_CONDUIT_* env, build tag conduit).
// alice creates an encrypted room and invites bob. bob's client auto-joins and must receive
// alice's encrypted message, decrypted.
func TestConduitEncryptedRoundTrip(t *testing.T) {
	env := func(k string) string {
		v := os.Getenv(k)
		require.NotEmpty(t, v, k)
		return v
	}
	home := env("MTX_CONDUIT_HOMESERVER")
	cfgA := Config{Homeserver: home, UserID: env("MTX_CONDUIT_A_USER"), AccessToken: env("MTX_CONDUIT_A_TOKEN"),
		DeviceID: env("MTX_CONDUIT_A_DEVICE"), PickleKey: "pickle-conduit", DBPath: filepath.Join(t.TempDir(), "a", "mtx.db")}
	cfgB := Config{Homeserver: home, UserID: env("MTX_CONDUIT_B_USER"), AccessToken: env("MTX_CONDUIT_B_TOKEN"),
		DeviceID: env("MTX_CONDUIT_B_DEVICE"), PickleKey: "pickle-conduit", DBPath: filepath.Join(t.TempDir(), "b", "mtx.db")}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	alice, err := New(ctx, cfgA)
	require.NoError(t, err)
	defer alice.Close()
	bob, err := New(ctx, cfgB)
	require.NoError(t, err)
	defer bob.Close()

	room, err := alice.MX().CreateRoom(ctx, &mautrix.ReqCreateRoom{Invite: []id.UserID{id.UserID(cfgB.UserID)}, Preset: "private_chat",
		InitialState: []*event.Event{{Type: event.StateEncryption, StateKey: ptr(""),
			Content: event.Content{Parsed: &event.EncryptionEventContent{Algorithm: id.AlgorithmMegolmV1}}}}})
	require.NoError(t, err)
	require.NoError(t, err)

	got := make(chan string, 1)
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	go func() {
		runErr := bob.Run(runCtx, func(hctx context.Context, evt *event.Event, decErr error) error {
			t.Logf("BOB EVENT type=%s room=%s err=%v", evt.Type, evt.RoomID, decErr)
			if evt.Type == event.EventMessage && evt.RoomID == room.RoomID && decErr == nil {
				select {
				case got <- evt.Content.AsMessage().Body:
				default:
				}
			}
			return nil
		})
		t.Logf("BOB RUN ERR: %v", runErr)
	}()

	// bob joins on his next sync; keys reach him when alice sends after the join. Retry until heard.
	deadline := time.After(60 * time.Second)
	for {
		_, err := alice.MX().SendText(ctx, room.RoomID, "hello from alice")
		require.NoError(t, err)
		select {
		case body := <-got:
			require.Equal(t, "hello from alice", body)
			return
		case <-time.After(2 * time.Second):
		case <-deadline:
			t.Fatal("bob did not receive alice's encrypted message")
		}
	}
}
func ptr[T any](v T) *T { return &v }
