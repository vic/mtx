//go:build conduit

package matrix

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// TestConduitWhoamiMismatch: the device check fails against the real server.
func TestConduitWhoamiMismatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg := conduitEnv(t, "A")
	cfg.DeviceID = "NOT-THIS-DEVICE"
	_, err := New(ctx, cfg)
	require.ErrorContains(t, err, "device mismatch")
}

// TestConduitRelationsAndRedaction: reply, thread and edit relations, and a redaction, survive the server.
func TestConduitRelationsAndRedaction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	alice, err := New(ctx, conduitEnv(t, "A"))
	require.NoError(t, err)
	defer alice.Close()
	bob, err := New(ctx, conduitEnv(t, "B"))
	require.NoError(t, err)
	defer bob.Close()
	room := encryptedRoom(t, ctx, alice, bob)

	got := make(chan *event.Event, 64)
	stop, done := collect(ctx, bob, got)
	defer stop()
	// bob joins on his first sync; send once he is in
	waitFor(t, got, 30*time.Second, func(e *event.Event) bool { return e.RoomID == room && e.Type == event.StateMember })

	base, err := alice.MX().SendText(ctx, room, "root")
	require.NoError(t, err)
	root := waitFor(t, got, 30*time.Second, func(e *event.Event) bool { return e.ID == base.EventID })
	require.Equal(t, "root", root.Content.AsMessage().Body)

	rel := (&event.RelatesTo{}).SetThread(base.EventID, base.EventID)
	_, err = alice.MX().SendMessageEvent(ctx, room, event.EventMessage, &event.MessageEventContent{MsgType: event.MsgText, Body: "in thread", RelatesTo: rel})
	require.NoError(t, err)
	th := waitFor(t, got, 30*time.Second, func(e *event.Event) bool {
		return e.Content.AsMessage() != nil && e.Content.AsMessage().Body == "in thread"
	})
	require.Equal(t, base.EventID.String(), string(th.Content.AsMessage().RelatesTo.GetThreadParent()))

	_, err = alice.MX().SendMessageEvent(ctx, room, event.EventMessage, &event.MessageEventContent{MsgType: event.MsgText, Body: "reply",
		RelatesTo: &event.RelatesTo{InReplyTo: &event.InReplyTo{EventID: base.EventID}}})
	require.NoError(t, err)
	rp := waitFor(t, got, 30*time.Second, func(e *event.Event) bool {
		return e.Content.AsMessage() != nil && e.Content.AsMessage().Body == "reply"
	})
	require.Equal(t, base.EventID, rp.Content.AsMessage().RelatesTo.InReplyTo.EventID)

	_, err = alice.MX().RedactEvent(ctx, room, th.ID)
	require.NoError(t, err)
	rd := waitFor(t, got, 30*time.Second, func(e *event.Event) bool { return e.Type == event.EventRedaction })
	require.Equal(t, th.ID, rd.Redacts)

	stop()
	<-done
	_ = id.EventID("")
}
