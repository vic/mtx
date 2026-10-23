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

	"mtx/internal/ui"
)

// TestCreateRoomAliasAndFocus: /create &r asks for alias r, encrypted, and focuses the new room.
func TestCreateRoomAliasAndFocus(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var req map[string]any
	ts.Router.HandleFunc("POST /_matrix/client/v3/createRoom", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		_ = json.Unmarshal(b, &req)
		mu.Unlock()
		fmt.Fprintf(w, `{"room_id":%q}`, testRoom)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	var out []tea.Msg
	r := newRunner(cli, func(m tea.Msg) { out = append(out, m) })
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}

	require.NoError(t, cs.create("&review"))
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, "review", req["room_alias_name"])
	require.Equal(t, "private_chat", req["preset"])
	require.Equal(t, ui.FocusMsg("&review"), out[len(out)-1])
	name, ok := r.nameOf(testRoom)
	require.True(t, ok)
	require.Equal(t, "&review", name)
}
