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

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/mockserver"

	tea "charm.land/bubbletea/v2"

	"mtx/internal/matrix"
	"mtx/internal/model"
	"mtx/internal/ui"
)

const testRoom = id.RoomID("!r:mock.test")

// fakeRooms serves join, joined rooms, members and send. sent collects message bodies.
func fakeRooms(ts *mockserver.MockServer, sent *[]string, mu *sync.Mutex) {
	ts.Router.HandleFunc("POST /_matrix/client/v3/join/{alias}", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"room_id":%q}`, testRoom)
	})
	ts.Router.HandleFunc("GET /_matrix/client/v3/joined_rooms", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"joined_rooms":[%q]}`, testRoom)
	})
	ts.Router.HandleFunc("GET /_matrix/client/v3/rooms/{roomID}/joined_members", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"joined":{"@alice:mock.test":{},"@bob:mock.test":{}}}`)
	})
	ts.Router.HandleFunc("PUT /_matrix/client/v3/rooms/{roomID}/send/{type}/{txn}", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m struct {
			Body string `json:"body"`
		}
		_ = json.Unmarshal(b, &m)
		mu.Lock()
		*sent = append(*sent, m.Body)
		mu.Unlock()
		fmt.Fprint(w, `{"event_id":"$sent"}`)
	})
}

// TestRunnerJoinSendReceive: /join r focuses the room, send reaches the server, a received
// message becomes a line for the room, and our own echo is not shown twice.
func TestRunnerJoinSendReceive(t *testing.T) {
	ctx := context.Background()
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var sent []string
	fakeRooms(ts, &sent, &mu)

	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	var out []tea.Msg
	var outMu sync.Mutex
	r := newRunner(cli, func(m tea.Msg) { outMu.Lock(); out = append(out, m); outMu.Unlock() })
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}

	require.NoError(t, cs.join("r"))
	require.NoError(t, r.send("#r", "hi"))
	require.Equal(t, []string{"hi"}, sent)

	require.NoError(t, r.onEvent(ctx, &event.Event{Type: event.EventMessage, RoomID: testRoom, ID: "$b1", Sender: "@bob:mock.test",
		Content: event.Content{Parsed: &event.MessageEventContent{MsgType: event.MsgText, Body: "yo"}}}, nil))
	require.NoError(t, r.onEvent(ctx, &event.Event{Type: event.EventMessage, RoomID: testRoom, ID: "$a1", Sender: "@alice:mock.test",
		Content: event.Content{Parsed: &event.MessageEventContent{MsgType: event.MsgText, Body: "hi"}}}, nil))

	outMu.Lock()
	defer outMu.Unlock()
	var focus ui.FocusMsg
	var lines []model.Line
	for _, m := range out {
		switch v := m.(type) {
		case ui.FocusMsg:
			focus = v
		case ui.LineMsg:
			require.Equal(t, "#r", v.Ch)
			lines = append(lines, v.L)
		}
	}
	require.Equal(t, ui.FocusMsg("#r"), focus)
	require.Len(t, lines, 2, "own message comes back in sync and is shown once")
	require.Equal(t, "bob", lines[0].Nick)
	require.Equal(t, "yo", lines[0].Text)
	require.Equal(t, "alice", lines[1].Nick)
	require.Equal(t, "hi", lines[1].Text)
}

// TestRegisterClashUsesRoomID: two rooms with the same alias localpart get distinct names.
func TestRegisterClashUsesRoomID(t *testing.T) {
	r := newRunner(nil, func(tea.Msg) {})
	a := r.register("!one:s", "review")
	b := r.register("!two:s", "review")
	require.Equal(t, "review", a)
	require.Equal(t, "#two", b)
}

// TestToastInviteNamesAliasAndInviter: the toast names the alias when there is one, and the inviter.
func TestToastInviteNamesAliasAndInviter(t *testing.T) {
	var out []tea.Msg
	r := newRunner(nil, func(m tea.Msg) { out = append(out, m) })
	r.toastInvite(matrix.Invite{Room: "!x:s", Alias: "#review:s", Inviter: "@bob:s"})
	r.toastInvite(matrix.Invite{Room: "!y:s", Inviter: "@carol:s"})
	require.Equal(t, ui.ToastMsg{Text: "invited to #review:s by bob: /accept #review:s or /reject #review:s"}, out[0])
	require.Equal(t, ui.ToastMsg{Text: "invited to !y:s by carol: /accept !y:s or /reject !y:s"}, out[1])
	r.toastInvite(matrix.Invite{Room: "!x:s", Alias: "#review:s", Inviter: "@bob:s"}) // repeated by a sync
	require.Len(t, out, 2)
	in, err := r.pendingInvite("#review:s")
	require.NoError(t, err)
	require.Equal(t, id.RoomID("!x:s"), in.Room)
}

// TestRoomLabelPrefersRoomName: the channel takes the room name as Element shows it, else the alias.
func TestRoomLabelPrefersRoomName(t *testing.T) {
	require.Equal(t, "#mi-sala", roomLabel(roomMeta{name: "  Mi   Sala ", alias: "#x:s"}, ""))
	require.Equal(t, "#x", roomLabel(roomMeta{alias: "#x:s"}, ""))
	require.Equal(t, "@ana", roomLabel(roomMeta{nicks: []string{"me", "ana"}}, "me"))
	require.Equal(t, "@a,b,c", roomLabel(roomMeta{nicks: []string{"a", "me", "b", "c", "d"}}, "me"))
	require.Equal(t, "", roomLabel(roomMeta{nicks: []string{"me"}}, "me"))
}

// TestHistoryLinesAreOld: history is batched per room and sent as scrollback, not unread;
// a live line is still a LineMsg.
func TestHistoryLinesAreOld(t *testing.T) {
	var out []tea.Msg
	cli, err := mautrix.NewClient("https://mock.test", "@alice:mock.test", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(m tea.Msg) { out = append(out, m) })
	r.register(testRoom, "#r")
	msg := &event.Event{Type: event.EventMessage, RoomID: testRoom, ID: "$h1", Sender: "@bob:mock.test",
		Content: event.Content{Parsed: &event.MessageEventContent{MsgType: event.MsgText, Body: "old"}}}
	require.NoError(t, r.onEvent(matrix.WithHistory(context.Background()), msg, nil))
	require.Empty(t, out, "history waits for the batch")
	r.flushHistory("#r")
	require.Len(t, out, 1)
	require.Len(t, out[0].(ui.HistoryLinesMsg).L, 1)
	require.NoError(t, r.onEvent(context.Background(), msg, nil))
	require.False(t, out[1].(ui.LineMsg).Old)
}

// TestTagGroup: favourite wins over low priority; other tags are no group.
func TestTagGroup(t *testing.T) {
	require.Equal(t, "", tagGroup(nil))
	require.Equal(t, "favourite", tagGroup(event.Tags{event.RoomTagFavourite: {}}))
	require.Equal(t, "lowpriority", tagGroup(event.Tags{event.RoomTagLowPriority: {}}))
	require.Equal(t, "favourite", tagGroup(event.Tags{event.RoomTagFavourite: {}, event.RoomTagLowPriority: {}}))
	require.Equal(t, "", tagGroup(event.Tags{"u.work": {}}))
}

// TestReactCounts: each user counts once per key; a redacted reaction leaves the summary without it.
func TestReactCounts(t *testing.T) {
	r := newRunner(nil, func(tea.Msg) {})
	require.Equal(t, "👍1", r.addReact("$e", "👍", "@a", "$r1"))
	require.Equal(t, "👍1 ❤️1", r.addReact("$e", "❤️", "@b", "$r2"))
	require.Equal(t, "👍2 ❤️1", r.addReact("$e", "👍", "@b", "$r3"))
	target, sum, ok := r.dropReact("$r1")
	require.True(t, ok)
	require.Equal(t, id.EventID("$e"), target)
	require.Equal(t, "👍1 ❤️1", sum)
	_, _, ok = r.dropReact("$zz")
	require.False(t, ok)
}

// TestSpaceEventsSkipRefresh: an event from a known space is ignored, with no room refresh (the nil client
// would panic in one).
func TestSpaceEventsSkipRefresh(t *testing.T) {
	r := newRunner(nil, func(tea.Msg) {})
	r.spaces = map[id.RoomID]bool{"!sp:s": true}
	name, ok, err := r.roomName("!sp:s")
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, "", name)
}

func TestRoomTag(t *testing.T) {
	require.Equal(t, "dm", roomTag("", true))
	require.Equal(t, "favourite", roomTag("favourite", true))
	require.Equal(t, "", roomTag("", false))
}

// TestSpaceGroups: a room in a space is "space:<name>", a room in a space inside a space is named by the
// path; a cycle between spaces ends.
func TestSpaceGroups(t *testing.T) {
	spaces := map[id.RoomID]roomMeta{
		"!s1:x": {room: "!s1:x", name: "A", space: true, children: []id.RoomID{"!s2:x", "!r1:x"}},
		"!s2:x": {room: "!s2:x", name: "B", space: true, children: []id.RoomID{"!r2:x", "!s1:x"}},
	}
	got := spaceGroups(spaces)
	require.Equal(t, "space:a", got["!r1:x"])
	require.Equal(t, "space:a › b", got["!r2:x"])
	require.Len(t, got, 2)
}
