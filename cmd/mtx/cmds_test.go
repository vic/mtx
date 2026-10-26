package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	tea "charm.land/bubbletea/v2"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/mockserver"

	"mtx/internal/matrix"
	"mtx/internal/model"
	"mtx/internal/ui"
)

// TestReactSends: /react sends a reaction event to the room of the channel.
func TestReactSends(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var path string
	ts.Router.HandleFunc("PUT /_matrix/client/v3/rooms/{room}/send/m.reaction/{txn}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		path = r.URL.Path
		mu.Unlock()
		fmt.Fprint(w, `{"event_id":"$r"}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})
	r.register(testRoom, "#r")
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}
	require.NoError(t, cs.react("#r", "👍", "$x"))
	mu.Lock()
	defer mu.Unlock()
	require.Contains(t, path, "/send/m.reaction/")
}

// TestRedactSends: /redact sends a redaction of the event to the room of the channel.
func TestRedactSends(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var path string
	ts.Router.HandleFunc("PUT /_matrix/client/v3/rooms/{room}/redact/{event}/{txn}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		path = r.URL.Path
		mu.Unlock()
		fmt.Fprint(w, `{"event_id":"$r"}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})
	r.register(testRoom, "#r")
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}
	require.NoError(t, cs.redact("#r", "$x", "spam"))
	mu.Lock()
	defer mu.Unlock()
	require.Contains(t, path, "/redact/")
}

// TestRenameSends: /name sends a room name state event to the room of the channel.
func TestRenameSends(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var path string
	ts.Router.HandleFunc("PUT /_matrix/client/v3/rooms/{room}/state/m.room.name/{key...}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		path = r.URL.Path
		mu.Unlock()
		fmt.Fprint(w, `{"event_id":"$r"}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})
	r.register(testRoom, "#r")
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}
	require.NoError(t, cs.rename("#r", "My Room"))
	mu.Lock()
	defer mu.Unlock()
	require.Contains(t, path, "/state/m.room.name")
}

// TestTagSends: /tag fav sets the favourite tag and emits its group; a bad group is the usage error.
func TestTagSends(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var path string
	ts.Router.HandleFunc("PUT /_matrix/client/v3/user/{user}/rooms/{room}/tags/{tag}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		path = r.URL.Path
		mu.Unlock()
		fmt.Fprint(w, `{}`)
	})
	ts.Router.HandleFunc("DELETE /_matrix/client/v3/user/{user}/rooms/{room}/tags/{tag}", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	var out []tea.Msg
	r := newRunner(cli, func(m tea.Msg) { out = append(out, m) })
	r.register(testRoom, "#r")
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}
	require.NoError(t, cs.tag("#r", "fav"))
	mu.Lock()
	require.Contains(t, path, "/tags/m.favourite")
	mu.Unlock()
	require.Equal(t, ui.TagsMsg{"#r": "favourite"}, out[len(out)-1])
	require.Error(t, cs.tag("#r", "bogus"))
}

// TestJoinTarget: a room ID, a full alias and a matrix.to link join as given; a bare name is an alias here.
func TestJoinTarget(t *testing.T) {
	require.Equal(t, "#den:s.org", joinTarget("den", "s.org"))
	require.Equal(t, "#den:matrix.org", joinTarget("#den:matrix.org", "s.org"))
	require.Equal(t, "!abc:matrix.org", joinTarget("!abc:matrix.org", "s.org"))
	require.Equal(t, "#den:matrix.org", joinTarget("https://matrix.to/#/%23den%3Amatrix.org?via=x", "s.org"))
	require.Equal(t, "#den:s.org", joinTarget("&den", "s.org"))
}

// TestNickSends: /nick sets the display name on the server.
func TestNickSends(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var path string
	ts.Router.HandleFunc("PUT /_matrix/client/v3/profile/{user}/displayname", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		path = r.URL.Path
		mu.Unlock()
		fmt.Fprint(w, `{}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}
	cm, err := model.ParseCmd("/nick Alice")
	require.NoError(t, err)
	require.NoError(t, cs.do("#r", cm))
	mu.Lock()
	defer mu.Unlock()
	require.Contains(t, path, "/displayname")
}

// TestIgnoreAdds: /ignore adds the user to m.ignored_user_list, starting from an empty list.
func TestIgnoreAdds(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var body string
	ts.Router.HandleFunc("GET /_matrix/client/v3/user/{user}/account_data/m.ignored_user_list", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"errcode":"M_NOT_FOUND","error":"no list"}`)
	})
	ts.Router.HandleFunc("PUT /_matrix/client/v3/user/{user}/account_data/m.ignored_user_list", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		body = string(b)
		mu.Unlock()
		fmt.Fprint(w, `{}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}
	cm, err := model.ParseCmd("/ignore @bob")
	require.NoError(t, err)
	require.NoError(t, cs.do("#r", cm))
	mu.Lock()
	defer mu.Unlock()
	require.Contains(t, body, "@bob:mock.test")
}

// TestEmoteSends: /me sends an m.emote message to the room of the channel.
func TestEmoteSends(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var path, body string
	ts.Router.HandleFunc("PUT /_matrix/client/v3/rooms/{room}/send/m.room.message/{txn}", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		path, body = r.URL.Path, string(b)
		mu.Unlock()
		fmt.Fprint(w, `{"event_id":"$e"}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})
	r.register(testRoom, "#r")
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}
	cm, err := model.ParseCmd("/me waves")
	require.NoError(t, err)
	require.NoError(t, cs.do("#r", cm))
	mu.Lock()
	defer mu.Unlock()
	require.Contains(t, path, "/send/m.room.message/")
	require.Contains(t, body, `"msgtype":"m.emote"`)
}

// fakeInv records accepted rooms in place of the matrix client.
type fakeInv struct{ accepted []id.RoomID }

func (f *fakeInv) Accept(_ context.Context, room id.RoomID) error {
	f.accepted = append(f.accepted, room)
	return nil
}

func (f *fakeInv) Reject(context.Context, id.RoomID) error { return nil }

// TestAcceptInvite: /accept joins the pending invite by its alias and forgets it; a second accept finds none.
func TestAcceptInvite(t *testing.T) {
	var out []tea.Msg
	r := newRunner(nil, func(m tea.Msg) { out = append(out, m) })
	r.toastInvite(matrix.Invite{Room: "!x:s", Alias: "#a:s", Inviter: "@bob:s"})
	fake := &fakeInv{}
	cs := &cmds{r: r, me: "@alice:s", server: "s", inv: fake}
	cm, err := model.ParseCmd("/accept #a:s")
	require.NoError(t, err)
	require.NoError(t, cs.do("", cm))
	require.Equal(t, []id.RoomID{"!x:s"}, fake.accepted)
	require.Error(t, cs.do("", cm))
}

// TestReactToggles: reacting again with my own emoji redacts my reaction instead of sending another.
func TestReactToggles(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var redacted string
	ts.Router.HandleFunc("PUT /_matrix/client/v3/rooms/{room}/redact/{event}/{txn}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		redacted = r.URL.Path
		mu.Unlock()
		fmt.Fprint(w, `{"event_id":"$x"}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})
	r.register(testRoom, "#r")
	r.addReact("$x", "👍", "@alice:mock.test", "$mine")
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}
	require.NoError(t, cs.react("#r", "👍", "$x"))
	mu.Lock()
	defer mu.Unlock()
	require.Contains(t, redacted, "/redact/$mine")
}

// TestMuteSends: /mute sets a room push rule with no actions, so the room sends no notifications.
func TestMuteSends(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var path, body string
	ts.Router.HandleFunc("PUT /_matrix/client/v3/pushrules/global/room/{rule}", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		path, body = r.URL.Path, string(b)
		mu.Unlock()
		fmt.Fprint(w, `{}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})
	r.register(testRoom, "#r")
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}
	cm, err := model.ParseCmd("/mute")
	require.NoError(t, err)
	require.NoError(t, cs.do("#r", cm))
	mu.Lock()
	defer mu.Unlock()
	require.Contains(t, path, "/pushrules/global/room/")
	require.Contains(t, body, `"actions":[]`)
}

// TestUnmuteSends: /unmute deletes that room push rule.
func TestUnmuteSends(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var path string
	ts.Router.HandleFunc("DELETE /_matrix/client/v3/pushrules/global/room/{rule}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		path = r.URL.Path
		mu.Unlock()
		fmt.Fprint(w, `{}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})
	r.register(testRoom, "#r")
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}
	cm, err := model.ParseCmd("/unmute")
	require.NoError(t, err)
	require.NoError(t, cs.do("#r", cm))
	mu.Lock()
	defer mu.Unlock()
	require.Contains(t, path, "/pushrules/global/room/")
}
