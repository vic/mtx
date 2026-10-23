package main

import (
	"encoding/json"
	"fmt"
	"io"
	"maunium.net/go/mautrix/event"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	tea "charm.land/bubbletea/v2"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/mockserver"

	"mtx/internal/model"
)

// TestEditSendsReplace: an edit sends m.replace for the target, with the new text in m.new_content.
func TestEditSendsReplace(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var sent []map[string]any
	ts.Router.HandleFunc("PUT /_matrix/client/v3/rooms/{roomID}/send/{type}/{txn}", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		mu.Lock()
		sent = append(sent, m)
		mu.Unlock()
		fmt.Fprint(w, `{"event_id":"$edit"}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})
	r.register(testRoom, "#r:mock.test")
	require.NoError(t, r.edit("&r", model.Line{ID: "$m1", Nick: "alice", Text: "old"}, "new"))

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, "* new", sent[0]["body"])
	require.Equal(t, map[string]any{"rel_type": "m.replace", "event_id": "$m1"}, sent[0]["m.relates_to"])
	require.Equal(t, "new", sent[0]["m.new_content"].(map[string]any)["body"])
}

// TestImageTextHasDownloadLink: an unencrypted image shows its name and a clickable download URL.
func TestImageTextHasDownloadLink(t *testing.T) {
	cli, err := mautrix.NewClient("https://mock.test", "@alice:mock.test", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})
	got := r.mediaText(&event.MessageEventContent{MsgType: event.MsgImage, Body: "cat.png", URL: "mxc://example.org/abc"})
	require.Equal(t, "[image] cat.png https://mock.test/_matrix/media/v3/download/example.org/abc", got)
	enc := r.mediaText(&event.MessageEventContent{MsgType: event.MsgFile, Body: "x.pdf", File: &event.EncryptedFileInfo{}})
	require.Equal(t, "[file encrypted] x.pdf", enc)
}
