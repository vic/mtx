package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
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

// TestUnreadSends: /unread sets the m.marked_unread room account data flag.
func TestUnreadSends(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var path, body string
	ts.Router.HandleFunc("PUT /_matrix/client/v3/user/{user}/rooms/{room}/account_data/{type}", func(w http.ResponseWriter, r *http.Request) {
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
	cm, err := model.ParseCmd("/unread")
	require.NoError(t, err)
	require.NoError(t, cs.do("#r", cm))
	mu.Lock()
	defer mu.Unlock()
	require.Contains(t, path, "/rooms/")
	require.Contains(t, path, "/account_data/m.marked_unread")
	require.JSONEq(t, `{"unread":true}`, body)
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

// TestPinSends: pin adds to the pinned list and unpin removes from it, sent as the room's pinned_events state.
func TestPinSends(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var body string
	ts.Router.HandleFunc("GET /_matrix/client/v3/rooms/{room}/state/m.room.pinned_events/{key...}", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"pinned":["$a"]}`)
	})
	ts.Router.HandleFunc("PUT /_matrix/client/v3/rooms/{room}/state/m.room.pinned_events/{key...}", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		body = string(b)
		mu.Unlock()
		fmt.Fprint(w, `{"event_id":"$s"}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})
	r.register(testRoom, "#r")
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}
	require.NoError(t, cs.pin("#r", "$b", true))
	mu.Lock()
	require.Contains(t, body, `"$a","$b"`)
	mu.Unlock()
	require.NoError(t, cs.pin("#r", "$a", false))
	mu.Lock()
	defer mu.Unlock()
	require.Contains(t, body, `"pinned":[]`)
}

// TestBanSends: /ban posts the ban with the user and the reason.
func TestBanSends(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var path, body string
	ts.Router.HandleFunc("POST /_matrix/client/v3/rooms/{room}/ban", func(w http.ResponseWriter, r *http.Request) {
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
	cm, err := model.ParseCmd("/ban @bob:mock.test spam here")
	require.NoError(t, err)
	require.NoError(t, cs.do("#r", cm))
	mu.Lock()
	defer mu.Unlock()
	require.Contains(t, path, "/ban")
	require.JSONEq(t, `{"user_id":"@bob:mock.test","reason":"spam here"}`, body)
}

// TestOpSends: /op reads the power levels, sets the user's level and sends them back.
func TestOpSends(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var put string
	ts.Router.HandleFunc("GET /_matrix/client/v3/rooms/{room}/state/m.room.power_levels/{key...}", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"users":{}}`)
	})
	ts.Router.HandleFunc("PUT /_matrix/client/v3/rooms/{room}/state/m.room.power_levels/{key...}", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		put = string(b)
		mu.Unlock()
		fmt.Fprint(w, `{"event_id":"$pl"}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})
	r.register(testRoom, "#r")
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}
	cm, err := model.ParseCmd("/op @bob:mock.test 50")
	require.NoError(t, err)
	require.NoError(t, cs.do("#r", cm))
	mu.Lock()
	defer mu.Unlock()
	require.Contains(t, put, `"@bob:mock.test":50`)
}

// TestJoinRuleSends: /joinrule sets m.room.join_rules; a bad rule is a usage error and sends nothing.
func TestJoinRuleSends(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var path, body string
	ts.Router.HandleFunc("PUT /_matrix/client/v3/rooms/{room}/state/m.room.join_rules/{key...}", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		path, body = r.URL.Path, string(b)
		mu.Unlock()
		fmt.Fprint(w, `{"event_id":"$jr"}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})
	r.register(testRoom, "#r")
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}
	cm, err := model.ParseCmd("/joinrule public")
	require.NoError(t, err)
	require.NoError(t, cs.do("#r", cm))
	mu.Lock()
	require.Contains(t, path, "/m.room.join_rules/")
	require.JSONEq(t, `{"join_rule":"public"}`, body)
	mu.Unlock()

	bad, err := model.ParseCmd("/joinrule everyone")
	require.NoError(t, err)
	require.ErrorContains(t, cs.do("#r", bad), "usage: :joinrule")
}

// TestHistorySends: /history world sets m.room.history_visibility to world_readable; a bad value is a usage error.
func TestHistorySends(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var path, body string
	ts.Router.HandleFunc("PUT /_matrix/client/v3/rooms/{room}/state/m.room.history_visibility/{key...}", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		path, body = r.URL.Path, string(b)
		mu.Unlock()
		fmt.Fprint(w, `{"event_id":"$hv"}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})
	r.register(testRoom, "#r")
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}
	cm, err := model.ParseCmd("/history world")
	require.NoError(t, err)
	require.NoError(t, cs.do("#r", cm))
	mu.Lock()
	require.Contains(t, path, "/m.room.history_visibility/")
	require.JSONEq(t, `{"history_visibility":"world_readable"}`, body)
	mu.Unlock()

	bad, err := model.ParseCmd("/history everyone")
	require.NoError(t, err)
	require.ErrorContains(t, cs.do("#r", bad), "usage: :history")
}

// TestAliasSends: /alias publishes the alias in the directory, then sets m.room.canonical_alias; a bad alias is a usage error.
func TestAliasSends(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var dir, canon string
	ts.Router.HandleFunc("PUT /_matrix/client/v3/directory/room/{alias}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		dir = r.URL.Path
		mu.Unlock()
		fmt.Fprint(w, `{}`)
	})
	ts.Router.HandleFunc("PUT /_matrix/client/v3/rooms/{room}/state/m.room.canonical_alias/{key...}", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		canon = string(b)
		mu.Unlock()
		fmt.Fprint(w, `{"event_id":"$ca"}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})
	r.register(testRoom, "#r")
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}
	cm, err := model.ParseCmd("/alias #den:mock.test")
	require.NoError(t, err)
	require.NoError(t, cs.do("#r", cm))
	mu.Lock()
	require.NotEmpty(t, dir)
	require.JSONEq(t, `{"alias":"#den:mock.test"}`, canon)
	mu.Unlock()

	bad, err := model.ParseCmd("/alias den")
	require.NoError(t, err)
	require.ErrorContains(t, cs.do("#r", bad), "usage: :alias")
}

// TestCreatePublic: /create &room public asks for a public_chat room in the directory, with no encryption.
func TestCreatePublic(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var body string
	ts.Router.HandleFunc("POST /_matrix/client/v3/createRoom", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		body = string(b)
		mu.Unlock()
		fmt.Fprint(w, `{"room_id":"!new:mock.test"}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}
	cm, err := model.ParseCmd("/create &den public")
	require.NoError(t, err)
	require.NoError(t, cs.do("#r", cm))
	mu.Lock()
	defer mu.Unlock()
	require.Contains(t, body, `"preset":"public_chat"`)
	require.Contains(t, body, `"visibility":"public"`)
	require.NotContains(t, body, "m.room.encryption")
}

// TestExploreEmits: /explore lists the server's public rooms as an ExploreMsg, filtered by the term.
func TestExploreEmits(t *testing.T) {
	ts := mockserver.Create(t)
	ts.Router.HandleFunc("GET /_matrix/client/v3/publicRooms", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"chunk":[{"room_id":"!a:mock.test","canonical_alias":"#cats:mock.test","name":"Cats","num_joined_members":3},{"room_id":"!b:mock.test","name":"Dogs","num_joined_members":1}],"total_room_count_estimate":2}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	var mu sync.Mutex
	var got []tea.Msg
	r := newRunner(cli, func(m tea.Msg) {
		mu.Lock()
		got = append(got, m)
		mu.Unlock()
	})
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}
	cm, err := model.ParseCmd("/explore")
	require.NoError(t, err)
	require.NoError(t, cs.do("#r", cm))
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, got, 1)
	em, ok := got[0].(ui.ExploreMsg)
	require.True(t, ok)
	require.Equal(t, []string{"#cats:mock.test  Cats (3)", "!b:mock.test  Dogs (1)"}, em.Entries)
	require.Equal(t, []string{"#cats:mock.test", "!b:mock.test"}, em.Targets)
}

// TestSearchEmits: /search asks the server for the room's messages and emits them as a SearchMsg.
func TestSearchEmits(t *testing.T) {
	ts := mockserver.Create(t)
	ts.Router.HandleFunc("POST /_matrix/client/v3/search", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"search_categories":{"room_events":{"results":[`+
			`{"result":{"event_id":"$1","type":"m.room.message","sender":"@bob:mock.test","origin_server_ts":1,"room_id":"`+string(testRoom)+`","content":{"msgtype":"m.text","body":"hello one"}}},`+
			`{"result":{"event_id":"$2","type":"m.room.message","sender":"@bob:mock.test","origin_server_ts":2,"room_id":"`+string(testRoom)+`","content":{"msgtype":"m.text","body":"hello two"}}}]}}}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	var mu sync.Mutex
	var got []tea.Msg
	r := newRunner(cli, func(m tea.Msg) {
		mu.Lock()
		got = append(got, m)
		mu.Unlock()
	})
	r.register(testRoom, "#r")
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}
	cm, err := model.ParseCmd("/search hello")
	require.NoError(t, err)
	require.NoError(t, cs.do("#r", cm))
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, got, 1)
	sm, ok := got[0].(ui.SearchMsg)
	require.True(t, ok)
	require.Equal(t, []string{"<bob> hello one", "<bob> hello two"}, sm.Entries)
	require.Equal(t, []string{"$1", "$2"}, sm.IDs)
}

// TestDevicesEmits: /devices lists my devices, this one marked, most recently seen first.
func TestDevicesEmits(t *testing.T) {
	ts := mockserver.Create(t)
	ts.Router.HandleFunc("GET /_matrix/client/v3/devices", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"devices":[{"device_id":"OLD","display_name":"old phone","last_seen_ts":1000},{"device_id":"THIS","display_name":"mtx","last_seen_ts":2000}]}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	cli.DeviceID = "THIS"
	var mu sync.Mutex
	var got []tea.Msg
	r := newRunner(cli, func(m tea.Msg) {
		mu.Lock()
		got = append(got, m)
		mu.Unlock()
	})
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}
	cm, err := model.ParseCmd("/devices")
	require.NoError(t, err)
	require.NoError(t, cs.do("#r", cm))
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, got, 1)
	lm, ok := got[0].(ui.ListMsg)
	require.True(t, ok)
	require.Equal(t, "devices", lm.Title)
	require.Len(t, lm.Entries, 2)
	require.True(t, strings.HasPrefix(lm.Entries[0], "THIS  mtx  last seen "))
	require.True(t, strings.HasSuffix(lm.Entries[0], "  (this device)"))
	require.True(t, strings.HasPrefix(lm.Entries[1], "OLD  old phone  last seen "))
}

// TestPreviewEmits: /preview asks for the link's preview and lists its title and description.
func TestPreviewEmits(t *testing.T) {
	ts := mockserver.Create(t)
	ts.Router.HandleFunc("GET /_matrix/client/v1/media/preview_url", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"og:title":"T","og:description":"D"}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	var mu sync.Mutex
	var got []tea.Msg
	r := newRunner(cli, func(m tea.Msg) {
		mu.Lock()
		got = append(got, m)
		mu.Unlock()
	})
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}
	cm, err := model.ParseCmd("/preview https://x.org")
	require.NoError(t, err)
	require.NoError(t, cs.do("#r", cm))
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, got, 1)
	lm, ok := got[0].(ui.ListMsg)
	require.True(t, ok)
	require.Equal(t, []string{"title: T", "description: D"}, lm.Entries)
}

// TestDMOpensExisting: /dm opens the direct message room already in m.direct, without creating one.
func TestDMOpensExisting(t *testing.T) {
	ts := mockserver.Create(t)
	ts.Router.HandleFunc("GET /_matrix/client/v3/user/{user}/account_data/m.direct", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"@bob:mock.test":["!dm:mock.test"]}`)
	})
	created := false
	ts.Router.HandleFunc("POST /_matrix/client/v3/createRoom", func(w http.ResponseWriter, r *http.Request) {
		created = true
		fmt.Fprint(w, `{"room_id":"!new:mock.test"}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	var mu sync.Mutex
	var got []tea.Msg
	r := newRunner(cli, func(m tea.Msg) {
		mu.Lock()
		got = append(got, m)
		mu.Unlock()
	})
	r.register("!dm:mock.test", "")
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}
	cm, err := model.ParseCmd("/dm @bob:mock.test")
	require.NoError(t, err)
	require.NoError(t, cs.do("#r", cm))
	mu.Lock()
	defer mu.Unlock()
	require.False(t, created)
	var focus string
	for _, m := range got {
		if f, ok := m.(ui.FocusMsg); ok {
			focus = string(f)
		}
	}
	require.Equal(t, "#dm", focus) // the room is already registered under its label
}
