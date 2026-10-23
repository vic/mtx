package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func TestThreadOf(t *testing.T) {
	reply := &event.Event{Type: event.EventMessage, Content: event.Content{Parsed: &event.MessageEventContent{Body: "x",
		RelatesTo: (&event.RelatesTo{}).SetThread(id.EventID("$root"), id.EventID("$root"))}}}
	require.Equal(t, "$root", threadOf(reply))
	plain := &event.Event{Type: event.EventMessage, Content: event.Content{Parsed: &event.MessageEventContent{Body: "y"}}}
	require.Equal(t, "", threadOf(plain))
}
