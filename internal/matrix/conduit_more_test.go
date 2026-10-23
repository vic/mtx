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

// TestConduitRestartKeepsSessions: bob restarts on the same store; a message sent after the
// restart decrypts with the megolm session stored before it.
func TestConduitRestartKeepsSessions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	alice, err := New(ctx, conduitEnv(t, "A"))
	require.NoError(t, err)
	defer alice.Close()
	cfgB := conduitEnv(t, "B")
	bob, err := New(ctx, cfgB)
	require.NoError(t, err)
	room := encryptedRoom(t, ctx, alice, cfgB.UserID)

	got := make(chan *event.Event, 64)
	stop, done := collect(ctx, bob, got)
	waitFor(t, got, 30*time.Second, func(e *event.Event) bool { return e.RoomID == room && e.Type == event.StateMember })
	_, err = alice.MX().SendText(ctx, room, "before restart")
	require.NoError(t, err)
	waitFor(t, got, 30*time.Second, func(e *event.Event) bool {
		return e.Content.AsMessage() != nil && e.Content.AsMessage().Body == "before restart"
	})
	stop()
	<-done
	require.NoError(t, bob.Close())

	bob, err = New(ctx, cfgB) // same store
	require.NoError(t, err)
	defer bob.Close()
	got = make(chan *event.Event, 64)
	stop, done = collect(ctx, bob, got)
	defer func() { stop(); <-done }()
	_, err = alice.MX().SendText(ctx, room, "after restart")
	require.NoError(t, err)
	waitFor(t, got, 30*time.Second, func(e *event.Event) bool {
		return e.Content.AsMessage() != nil && e.Content.AsMessage().Body == "after restart"
	})
}

// TestConduitTypingAndMentionsAndKick: typing reaches bob's hook; a mention survives; a kick
// shows as bob's membership leave by alice.
func TestConduitTypingAndMentionsAndKick(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	alice, err := New(ctx, conduitEnv(t, "A"))
	require.NoError(t, err)
	defer alice.Close()
	cfgB := conduitEnv(t, "B")
	bob, err := New(ctx, cfgB)
	require.NoError(t, err)
	defer bob.Close()
	room := encryptedRoom(t, ctx, alice, cfgB.UserID)

	typing := make(chan []id.UserID, 8)
	bob.OnTyping(func(r id.RoomID, users []id.UserID) {
		if r == room {
			select {
			case typing <- users:
			default:
			}
		}
	})
	got := make(chan *event.Event, 64)
	stop, done := collect(ctx, bob, got)
	defer func() { stop(); <-done }()
	waitFor(t, got, 30*time.Second, func(e *event.Event) bool { return e.RoomID == room && e.Type == event.StateMember })

	_, err = alice.MX().UserTyping(ctx, room, true, 10*time.Second)
	require.NoError(t, err)
	select {
	case users := <-typing:
		require.Contains(t, users, id.UserID(alice.MX().UserID.String()))
	case <-time.After(20 * time.Second):
		t.Fatal("typing not delivered")
	}

	_, err = alice.MX().SendMessageEvent(ctx, room, event.EventMessage, &event.MessageEventContent{MsgType: event.MsgText, Body: "hi bob",
		Mentions: &event.Mentions{UserIDs: []id.UserID{id.UserID(cfgB.UserID)}}})
	require.NoError(t, err)
	m := waitFor(t, got, 30*time.Second, func(e *event.Event) bool {
		return e.Content.AsMessage() != nil && e.Content.AsMessage().Body == "hi bob"
	})
	require.Contains(t, m.Content.AsMessage().Mentions.UserIDs, id.UserID(cfgB.UserID))

	_, err = alice.MX().KickUser(ctx, room, &mautrix.ReqKickUser{UserID: id.UserID(cfgB.UserID)})
	require.NoError(t, err)
	k := waitFor(t, got, 30*time.Second, func(e *event.Event) bool {
		m := e.Content.AsMember()
		return e.Type == event.StateMember && e.GetStateKey() == cfgB.UserID && m != nil && m.Membership == event.MembershipLeave
	})
	require.Equal(t, alice.MX().UserID.String(), k.Sender.String())
}
