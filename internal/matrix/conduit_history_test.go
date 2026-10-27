//go:build conduit

package matrix

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// TestConduitHistoryOnJoin: messages sent before bob was invited reach him on join, as history.
// The room is not encrypted: keys for pre-join history are not shared with a later joiner.
func TestConduitHistoryOnJoin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	alice, err := New(ctx, conduitEnv(t, "A"))
	require.NoError(t, err)
	defer alice.Close()
	cfgB := conduitEnv(t, "B")
	bob, err := New(ctx, cfgB)
	require.NoError(t, err)
	defer bob.Close()

	room, err := alice.MX().CreateRoom(ctx, &mautrix.ReqCreateRoom{Preset: "private_chat"})
	require.NoError(t, err)
	for _, body := range []string{"one", "two", "three"} {
		_, err := alice.MX().SendText(ctx, room.RoomID, body)
		require.NoError(t, err)
	}
	_, err = alice.MX().InviteUser(ctx, room.RoomID, &mautrix.ReqInviteUser{UserID: id.UserID(cfgB.UserID)})
	require.NoError(t, err)
	require.NoError(t, bob.Accept(ctx, room.RoomID))

	got := make(chan struct {
		body string
		hist bool
	}, 64)
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	go func() {
		_ = bob.Run(runCtx, func(hctx context.Context, evt *event.Event, decErr error) error {
			if m := evt.Content.AsMessage(); evt.Type == event.EventMessage && m != nil && decErr == nil && evt.RoomID == room.RoomID {
				select {
				case got <- struct {
					body string
					hist bool
				}{m.Body, IsHistory(hctx)}:
				default:
				}
			}
			return nil
		})
	}()
	deadline := time.After(60 * time.Second)
	seen := map[string]bool{}
	for len(seen) < 3 {
		select {
		case g := <-got:
			require.True(t, g.hist, "pre-join message %q must be history", g.body)
			seen[g.body] = true
		case <-deadline:
			t.Fatalf("history not delivered, have %v", seen)
		}
	}
}

// TestConduitResumeNoReplay: after a restart on the same store, an old message is not delivered
// again; a new one is.
func TestConduitResumeNoReplay(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	alice, err := New(ctx, conduitEnv(t, "A"))
	require.NoError(t, err)
	defer alice.Close()
	cfgB := conduitEnv(t, "B")
	bob, err := New(ctx, cfgB)
	require.NoError(t, err)
	room, err := alice.MX().CreateRoom(ctx, &mautrix.ReqCreateRoom{Preset: "private_chat", Invite: []id.UserID{id.UserID(cfgB.UserID)}})
	require.NoError(t, err)
	require.NoError(t, bob.Accept(ctx, room.RoomID))
	_, err = alice.MX().SendText(ctx, room.RoomID, "old")
	require.NoError(t, err)

	got := make(chan string, 64)
	run := func(c *Client, runCtx context.Context) {
		_ = c.Run(runCtx, func(_ context.Context, evt *event.Event, decErr error) error {
			if m := evt.Content.AsMessage(); evt.Type == event.EventMessage && m != nil && decErr == nil && evt.RoomID == room.RoomID {
				got <- m.Body
			}
			return nil
		})
	}
	first, stop1 := context.WithCancel(ctx)
	go run(bob, first)
	waitBody(t, got, "old")
	stop1()
	require.NoError(t, bob.Close())
	time.Sleep(500 * time.Millisecond)

	bob, err = New(ctx, cfgB) // same store, resumed from next_batch
	require.NoError(t, err)
	defer bob.Close()
	second, stop2 := context.WithCancel(ctx)
	defer stop2()
	go run(bob, second)
	_, err = alice.MX().SendText(ctx, room.RoomID, "new")
	require.NoError(t, err)
	select {
	case b := <-got:
		require.Equal(t, "new", b, "a replayed message must not arrive after resume")
	case <-time.After(30 * time.Second):
		t.Fatal("new message not delivered after resume")
	}
}

func waitBody(t *testing.T, got <-chan string, want string) {
	t.Helper()
	deadline := time.After(60 * time.Second)
	for {
		select {
		case b := <-got:
			if b == want {
				return
			}
		case <-deadline:
			t.Fatalf("%q not delivered", want)
		}
	}
}
