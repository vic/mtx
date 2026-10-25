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
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/mockserver"

	"mtx/internal/ui"
)

// TestTypingExcludesMeAndSeenSendsReadMarkers: typing shows the others; seen sends m.read and
// m.fully_read to the newest message, once.
func TestTypingExcludesMeAndSeenSendsReadMarkers(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var markers []map[string]string
	ts.Router.HandleFunc("POST /_matrix/client/v3/rooms/{roomID}/read_markers", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]string
		_ = json.Unmarshal(b, &m)
		mu.Lock()
		markers = append(markers, m)
		mu.Unlock()
		fmt.Fprint(w, `{}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	var out []tea.Msg
	r := newRunner(cli, func(m tea.Msg) { out = append(out, m) })
	r.register(testRoom, "r")

	r.onTyping(testRoom, []id.UserID{"@alice:mock.test", "@bob:mock.test"})
	require.Equal(t, ui.TypingMsg{Ch: "r", Nicks: []string{"bob"}}, out[0])

	r.noteLast(&event.Event{Type: event.EventMessage, RoomID: testRoom, ID: "$m9"})
	require.NoError(t, r.seen("r"))
	require.NoError(t, r.seen("r"), "same newest message: no second receipt")
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []map[string]string{{"m.read": "$m9", "m.fully_read": "$m9"}}, markers)
}
