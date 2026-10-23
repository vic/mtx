package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	tea "charm.land/bubbletea/v2"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/mockserver"

	"mtx/internal/model"
	"mtx/internal/ui"
)

func threadMsg(id, body string, root id.EventID) *event.Event {
	rel := (&event.RelatesTo{}).SetThread(root, root)
	content := &event.MessageEventContent{MsgType: event.MsgText, Body: body}
	if root != "" {
		content.RelatesTo = rel
	}
	return &event.Event{Type: event.EventMessage, RoomID: testRoom, ID: id1(id), Sender: "@bob:mock.test",
		Content: event.Content{Parsed: content}}
}

func id1(s string) id.EventID { return id.EventID(s) }

// TestThreadHeadsAndSendRelation: a thread root shows as a head with its text and reply count;
// a send while a head is selected carries the m.thread relation to that root.
func TestThreadHeadsAndSendRelation(t *testing.T) {
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
	var out []tea.Msg
	r := newRunner(cli, func(m tea.Msg) { out = append(out, m) })
	r.register(testRoom, "#r:mock.test")

	require.NoError(t, r.onEvent(context.Background(), threadMsg("$root", "start", ""), nil))
	require.NoError(t, r.onEvent(context.Background(), threadMsg("$t1", "reply", "$root"), nil))
	require.NoError(t, r.onEvent(context.Background(), threadMsg("$t2", "again", "$root"), nil))

	last := out[len(out)-1].(ui.ThreadsMsg)
	require.Equal(t, "&r", last.Ch)
	require.Equal(t, []model.Thread{{Root: "$root", Nick: "bob", Text: "start", Count: 2, Live: 2}}, last.Heads)

	require.NoError(t, r.reply("&r", model.Line{ID: "$root", Thread: "$root"}, "in thread"))
	mu.Lock()
	defer mu.Unlock()
	rel, ok := sent[0]["m.relates_to"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "m.thread", rel["rel_type"])
	require.Equal(t, "$root", rel["event_id"])
}
