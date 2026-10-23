package main

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/stretchr/testify/require"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func TestMessageTextEditMentionAndUndecryptable(t *testing.T) {
	cli, err := mautrix.NewClient("https://mock.test", "@alice:s", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})

	edit := &event.Event{Type: event.EventMessage, Sender: "@bob:s",
		Content: event.Content{Parsed: &event.MessageEventContent{Body: "* fix", NewContent: &event.MessageEventContent{Body: "fix"}}}}
	text, dim, ok := r.messageText(edit, nil)
	require.True(t, ok)
	require.False(t, dim)
	require.Equal(t, "(edited) fix", text)

	mention := &event.Event{Type: event.EventMessage, Sender: "@bob:s",
		Content: event.Content{Parsed: &event.MessageEventContent{Body: "Alice, ping", Mentions: &event.Mentions{UserIDs: []id.UserID{"@alice:s"}}}}}
	text, _, _ = r.messageText(mention, nil)
	require.Equal(t, "@alice Alice, ping", text)

	und, dim, ok := r.messageText(mention, errors.New("no key"))
	require.True(t, ok)
	require.True(t, dim)
	require.Equal(t, "[undecryptable: no key]", und)
}
