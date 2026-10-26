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
	require.Equal(t, "fix (edited)", text)

	mention := &event.Event{Type: event.EventMessage, Sender: "@bob:s",
		Content: event.Content{Parsed: &event.MessageEventContent{Body: "Alice, ping", Mentions: &event.Mentions{UserIDs: []id.UserID{"@alice:s"}}}}}
	text, _, _ = r.messageText(mention, nil)
	require.Equal(t, "@alice Alice, ping", text)

	und, dim, ok := r.messageText(mention, errors.New("no key"))
	require.True(t, ok)
	require.True(t, dim)
	require.Equal(t, "[undecryptable: no key]", und)
}

func TestMessageTextHTML(t *testing.T) {
	cli, err := mautrix.NewClient("https://mock.test", "@alice:s", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})

	html := &event.Event{Type: event.EventMessage, Sender: "@bob:s",
		Content: event.Content{Parsed: &event.MessageEventContent{Body: "plain bold", MsgType: event.MsgText,
			Format: event.FormatHTML, FormattedBody: `plain <strong>bold</strong> and <a href="https://x.org">link</a>`}}}
	text, dim, ok := r.messageText(html, nil)
	require.True(t, ok)
	require.False(t, dim)
	require.Equal(t, "plain **bold** and [link](https://x.org)", text)

	plain := &event.Event{Type: event.EventMessage, Sender: "@bob:s",
		Content: event.Content{Parsed: &event.MessageEventContent{Body: "just <b>text</b>", MsgType: event.MsgText}}}
	text, _, _ = r.messageText(plain, nil)
	require.Equal(t, "just <b>text</b>", text)
}

func TestReplyTo(t *testing.T) {
	reply := &event.Event{Type: event.EventMessage, Sender: "@bob:s",
		Content: event.Content{Parsed: &event.MessageEventContent{Body: "yes", RelatesTo: &event.RelatesTo{InReplyTo: &event.InReplyTo{EventID: "$p"}}}}}
	require.Equal(t, "$p", replyTo(reply))

	plain := &event.Event{Type: event.EventMessage, Sender: "@bob:s",
		Content: event.Content{Parsed: &event.MessageEventContent{Body: "no"}}}
	require.Equal(t, "", replyTo(plain))
}
