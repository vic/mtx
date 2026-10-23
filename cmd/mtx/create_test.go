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

// TestCreateFocusesInApp: the whole UI path: CmdMsg -> command -> RoomsMsg and FocusMsg -> focus.
func TestCreateFocusesInApp(t *testing.T) {
	ts := mockserver.Create(t)
	ts.Router.HandleFunc("POST /_matrix/client/v3/createRoom", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"room_id":%q}`, testRoom)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	var out []tea.Msg
	r := newRunner(cli, func(m tea.Msg) { out = append(out, m) })
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}
	m, err := model.New(100)
	require.NoError(t, err)
	app := ui.New(m, r.send).WithCmd(cs.cmd)

	cm, err := model.ParseCmd("/create &review")
	require.NoError(t, err)
	next, cmd := app.Update(ui.CmdMsg{Cmd: cm})
	app = next.(ui.App)
	require.NotNil(t, cmd)
	require.Nil(t, cmd()) // success: the command returns nothing itself
	for _, msg := range out {
		next, _ := app.Update(msg)
		app = next.(ui.App)
	}
	require.Equal(t, "&review", app.Channel())
}
