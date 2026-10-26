package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	tea "charm.land/bubbletea/v2"
	"testing"

	"github.com/stretchr/testify/require"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/mockserver"
)

// TestSendMentionsMembersOnly: @bob in a send becomes m.mentions for bob; @bo (not a member) does not.
func TestSendMentionsMembersOnly(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var sent []map[string]any
	ts.Router.HandleFunc("GET /_matrix/client/v3/rooms/{roomID}/joined_members", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"joined":{"@alice:mock.test":{},"@bob:mock.test":{}}}`)
	})
	ts.Router.HandleFunc("PUT /_matrix/client/v3/rooms/{roomID}/send/{type}/{txn}", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		mu.Lock()
		sent = append(sent, m)
		mu.Unlock()
		fmt.Fprint(w, `{"event_id":"$sent"}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})
	r.register(testRoom, "r")
	require.NoError(t, r.send("r", "hi @bob and @bo"))
	require.NoError(t, r.send("r", "no mention"))

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, sent, 2)
	require.Equal(t, "hi @bob and @bo", sent[0]["body"])
	require.Equal(t, map[string]any{"user_ids": []any{"@bob:mock.test"}}, sent[0]["m.mentions"])
	require.Nil(t, sent[1]["m.mentions"])
}

// TestContentMarkdown: markdown is sent as formatted HTML; the plain body stays what was typed.
func TestContentMarkdown(t *testing.T) {
	ts := mockserver.Create(t)
	ts.Router.HandleFunc("GET /_matrix/client/v3/rooms/{roomID}/joined_members", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"joined":{"@alice:mock.test":{}}}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})
	ctx := context.Background()
	c, err := r.content(ctx, testRoom, "hi **there**")
	require.NoError(t, err)
	require.Equal(t, "hi **there**", c.Body)
	require.Equal(t, event.FormatHTML, c.Format)
	require.Contains(t, c.FormattedBody, "<strong>there</strong>")
	plain, err := r.content(ctx, testRoom, "plain")
	require.NoError(t, err)
	require.Equal(t, "", plain.FormattedBody)
}
