package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	tea "charm.land/bubbletea/v2"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/mockserver"

	"mtx/internal/model"
)

// TestReplyCarriesInReplyTo: a reply to a line with an event ID sends m.relates_to in_reply_to;
// a line without one is sent plain.
func TestReplyCarriesInReplyTo(t *testing.T) {
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

	require.NoError(t, r.reply("r", model.Line{ID: "$m1", Nick: "bob", Text: "hi"}, "yes"))
	require.NoError(t, r.reply("r", model.Line{Nick: "bob", Text: "local"}, "plain"))

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, map[string]any{"m.in_reply_to": map[string]any{"event_id": "$m1"}}, sent[0]["m.relates_to"])
	require.Nil(t, sent[1]["m.relates_to"])
}
