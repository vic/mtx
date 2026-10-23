package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"mtx/internal/matrix"
	"mtx/internal/model"
	"mtx/internal/ui"
)

const (
	callTimeout  = 30 * time.Second
	sendAttempts = 3
)

// runner is the Matrix side of the UI: rooms are channels named &local, and
// every UI message is produced here.
type runner struct {
	mx   *mautrix.Client
	emit func(tea.Msg)

	mu     sync.Mutex
	last   map[id.RoomID]id.EventID // newest message per room (read markers)
	marked map[id.RoomID]id.EventID // newest message per room already marked read
	names  map[string]id.RoomID     // "&name" -> room
	ids    map[id.RoomID]string     // room -> "&name"
}

func newRunner(mx *mautrix.Client, emit func(tea.Msg)) *runner {
	return &runner{mx: mx, emit: emit, names: map[string]id.RoomID{}, ids: map[id.RoomID]string{}, last: map[id.RoomID]id.EventID{}, marked: map[id.RoomID]id.EventID{}}
}

func callCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), callTimeout)
}

// localpart of an alias (#x:s), room ID (!x:s) or user ID (@x:s).
func localpart(s string) string {
	s = strings.TrimLeft(s, "#!@")
	if i := strings.IndexByte(s, ':'); i >= 0 {
		return s[:i]
	}
	return s
}

// register gives a room its channel name: &<alias localpart>, or &<room localpart> on a clash.
func (r *runner) register(room id.RoomID, alias string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if name, ok := r.ids[room]; ok {
		return name
	}
	name := "&" + localpart(alias)
	if alias == "" || localpart(alias) == "" {
		name = "&" + localpart(string(room))
	}
	if _, taken := r.names[name]; taken {
		name = "&" + localpart(string(room))
	}
	r.names[name] = room
	r.ids[room] = name
	return name
}

func (r *runner) unregister(room id.RoomID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if name, ok := r.ids[room]; ok {
		delete(r.names, name)
		delete(r.ids, room)
	}
}

// roomOf resolves a channel name to its room.
func (r *runner) roomOf(name string) (id.RoomID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.names[name]
	if !ok {
		return "", fmt.Errorf("no room %s", name)
	}
	return room, nil
}

func (r *runner) nameOf(room id.RoomID) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name, ok := r.ids[room]
	return name, ok
}

// rooms lists the channel names, sorted.
func (r *runner) rooms() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.names))
	for n := range r.names {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

func (r *runner) emitRooms() { r.emit(ui.RoomsMsg(r.rooms())) }

// members returns the joined user localparts of room, sorted.
func (r *runner) members(room id.RoomID) ([]string, error) {
	ctx, cancel := callCtx()
	defer cancel()
	resp, err := r.mx.JoinedMembers(ctx, room)
	if err != nil {
		return nil, err
	}
	nicks := make([]string, 0, len(resp.Joined))
	for u := range resp.Joined {
		nicks = append(nicks, localpart(u.String()))
	}
	slices.Sort(nicks)
	return nicks, nil
}

func (r *runner) emitMembers(room id.RoomID) error {
	name, ok := r.nameOf(room)
	if !ok {
		return nil
	}
	nicks, err := r.members(room)
	if err != nil {
		return err
	}
	r.emit(ui.MembersMsg{Ch: name, Nicks: nicks})
	return nil
}

// refresh loads the joined rooms at start and after a join.
func (r *runner) refresh() error {
	ctx, cancel := callCtx()
	defer cancel()
	resp, err := r.mx.JoinedRooms(ctx)
	if err != nil {
		return fmt.Errorf("joined rooms: %w", err)
	}
	for _, room := range resp.JoinedRooms {
		alias := r.canonicalAlias(room)
		r.register(room, alias)
		if err := r.emitMembers(room); err != nil {
			return err
		}
	}
	r.emitRooms()
	return nil
}

func (r *runner) canonicalAlias(room id.RoomID) string {
	ctx, cancel := callCtx()
	defer cancel()
	var c event.CanonicalAliasEventContent
	if err := r.mx.StateEvent(ctx, room, event.StateCanonicalAlias, "", &c); err != nil {
		return ""
	}
	return string(c.Alias)
}

// onEvent is the matrix.Handler: messages become lines, member changes refresh the nick pane.
func (r *runner) onEvent(ctx context.Context, evt *event.Event, decErr error) error {
	old := matrix.IsHistory(ctx) // replayed on the first sync: scrollback, no unread count
	r.noteLast(evt)
	name, ok := r.nameOf(evt.RoomID)
	if !ok {
		if err := r.refresh(); err != nil {
			return err
		}
		if name, ok = r.nameOf(evt.RoomID); !ok {
			return nil // a room we are not in (left or not joined yet)
		}
	}
	if evt.Type == event.StateMember {
		if text, ok := membershipText(evt); ok {
			r.emit(ui.LineMsg{Old: old, Ch: name, L: systemLine(evt, text)})
		}
		if evt.GetStateKey() == r.mx.UserID.String() && leftRoom(evt) {
			r.unregister(evt.RoomID) // kicked, banned, left, or invite declined: the channel goes
			r.emitRooms()
			return nil
		}
		return r.emitMembers(evt.RoomID)
	}
	if evt.Type == event.StateRoomName {
		if n := evt.Content.AsRoomName(); n != nil {
			r.emit(ui.LineMsg{Old: old, Ch: name, L: systemLine(evt, fmt.Sprintf("%s renamed the room to %s", localpart(evt.Sender.String()), n.Name))})
		}
		return nil
	}
	if evt.Type == event.StateTopic {
		if text, ok := topicText(evt); ok {
			r.emit(ui.LineMsg{Old: old, Ch: name, L: systemLine(evt, text)})
		}
		return nil
	}
	if evt.Type != event.EventMessage && evt.Type != event.EventEncrypted && evt.Type != event.EventRedaction {
		return nil
	}
	if evt.Sender.String() == r.mx.UserID.String() {
		return nil // our own lines are echoed locally by the UI when sent
	}
	if evt.Type == event.EventRedaction {
		r.emit(ui.LineMsg{Old: old, Ch: name, L: systemLine(evt, fmt.Sprintf("%s removed a message", localpart(evt.Sender.String())))})
		return nil
	}
	text, dim, ok := r.messageText(evt, decErr)
	if matrix.IsLate(ctx) {
		text = "(late) " + text
	}
	if !ok {
		return nil
	}
	r.emit(ui.LineMsg{Old: old, Ch: name, L: model.Line{At: time.UnixMilli(evt.Timestamp), Nick: localpart(evt.Sender.String()), Text: text, Dim: dim, ID: evt.ID.String(), Thread: threadOf(evt)}})
	return nil
}

// messageText is the scrollback text of a message: body, an edit (new content), or the decrypt
// failure. A mention of us in m.mentions is written as @user so the mentions filter sees it.
func (r *runner) messageText(evt *event.Event, decErr error) (text string, dim, ok bool) {
	if decErr != nil {
		return fmt.Sprintf("[undecryptable: %v]", decErr), true, true
	}
	m := evt.Content.AsMessage()
	if m == nil {
		return "", false, false
	}
	body := m.Body
	if m.NewContent != nil {
		body = "(edited) " + m.NewContent.Body
	}
	me := r.mx.UserID
	if m.Mentions != nil && slices.Contains(m.Mentions.UserIDs, me) && !strings.Contains(body, "@"+localpart(me.String())) {
		body = "@" + localpart(me.String()) + " " + body
	}
	return body, false, true
}

func (r *runner) send(ch, text string) error {
	room, err := r.roomOf(ch)
	if err != nil {
		return err
	}
	ctx, cancel := callCtx()
	defer cancel()
	content, err := r.content(ctx, room, text)
	if err != nil {
		return err
	}
	return matrix.Retry(ctx, sendAttempts, func() error {
		_, err := r.mx.SendMessageEvent(ctx, room, event.EventMessage, content)
		return err
	})
}

// sendLiteral sends a : line as is. There is no local echo for it, so the line is shown here.
func (r *runner) sendLiteral(ch, text string) error {
	if err := r.send(ch, text); err != nil {
		return err
	}
	r.emit(ui.LineMsg{Ch: ch, L: model.Line{At: time.Now(), Nick: localpart(r.mx.UserID.String()), Text: text}})
	return nil
}

// toastInvite tells the user an invite was accepted. The room is named by its alias when it has one.
func (r *runner) toastInvite(in matrix.Invite) {
	what := string(in.Room)
	if in.Alias != "" {
		what = in.Alias
	}
	r.emit(ui.ToastMsg{Text: fmt.Sprintf("invited to %s by %s: joined", what, localpart(in.Inviter.String()))})
}
