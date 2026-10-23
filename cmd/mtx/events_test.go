package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func member(sender, target string, now, prev event.Membership) *event.Event {
	evt := &event.Event{Type: event.StateMember, Sender: id.UserID(sender), StateKey: ptr(target),
		Content: event.Content{Parsed: &event.MemberEventContent{Membership: now}}}
	if prev != "" {
		evt.Unsigned.PrevContent = &event.Content{Parsed: &event.MemberEventContent{Membership: prev}}
	}
	return evt
}

func TestMembershipText(t *testing.T) {
	cases := []struct {
		name string
		evt  *event.Event
		want string
	}{
		{"join after invite", member("@bob:s", "@bob:s", event.MembershipJoin, event.MembershipInvite), "bob joined"},
		{"invite", member("@alice:s", "@bob:s", event.MembershipInvite, event.MembershipLeave), "bob invited by alice"},
		{"self leave", member("@bob:s", "@bob:s", event.MembershipLeave, event.MembershipJoin), "bob left"},
		{"kick", member("@alice:s", "@bob:s", event.MembershipLeave, event.MembershipJoin), "bob kicked by alice"},
		{"ban", member("@alice:s", "@bob:s", event.MembershipBan, event.MembershipJoin), "bob banned by alice"},
		{"unban", member("@alice:s", "@bob:s", event.MembershipLeave, event.MembershipBan), "bob unbanned by alice"},
	}
	for _, c := range cases {
		got, ok := membershipText(c.evt)
		require.True(t, ok, c.name)
		require.Equal(t, c.want, got, c.name)
	}
}

func TestMembershipTextNoChange(t *testing.T) {
	_, ok := membershipText(member("@bob:s", "@bob:s", event.MembershipJoin, event.MembershipJoin))
	require.False(t, ok, "same membership is not a change")
}

func TestTopicText(t *testing.T) {
	evt := &event.Event{Type: event.StateTopic, Sender: "@alice:s",
		Content: event.Content{Parsed: &event.TopicEventContent{Topic: "release day"}}}
	got, ok := topicText(evt)
	require.True(t, ok)
	require.Equal(t, "alice set topic: release day", got)
}

func TestLeftRoom(t *testing.T) {
	require.True(t, leftRoom(member("@alice:s", "@alice:s", event.MembershipLeave, event.MembershipJoin)))
	require.True(t, leftRoom(member("@alice:s", "@alice:s", event.MembershipBan, event.MembershipJoin)))
	require.False(t, leftRoom(member("@alice:s", "@alice:s", event.MembershipJoin, event.MembershipInvite)))
}
