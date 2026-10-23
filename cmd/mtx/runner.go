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

const callTimeout = 30 * time.Second

// runner is the Matrix side of the UI: rooms are channels named &local, and
// every UI message is produced here.
type runner struct {
	mx   *mautrix.Client
	emit func(tea.Msg)

	mu    sync.Mutex
	names map[string]id.RoomID // "&name" -> room
	ids   map[id.RoomID]string // room -> "&name"
}

func newRunner(mx *mautrix.Client, emit func(tea.Msg)) *runner {
	return &runner{mx: mx, emit: emit, names: map[string]id.RoomID{}, ids: map[id.RoomID]string{}}
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
func (r *runner) onEvent(_ context.Context, evt *event.Event, decErr error) error {
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
		return r.emitMembers(evt.RoomID)
	}
	if evt.Type != event.EventMessage && evt.Type != event.EventEncrypted {
		return nil
	}
	if evt.Sender.String() == r.mx.UserID.String() {
		return nil // our own lines are echoed locally by the UI when sent
	}
	text := ""
	switch {
	case decErr != nil:
		text = fmt.Sprintf("[undecryptable: %v]", decErr)
	case evt.Content.AsMessage() != nil:
		text = evt.Content.AsMessage().Body
	default:
		return nil
	}
	r.emit(ui.LineMsg{Ch: name, L: model.Line{At: time.UnixMilli(evt.Timestamp), Nick: localpart(evt.Sender.String()), Text: text, Dim: decErr != nil}})
	return nil
}

// send delivers text to the room of channel ch. E2EE is applied by the client.
func (r *runner) send(ch, text string) error {
	room, err := r.roomOf(ch)
	if err != nil {
		return err
	}
	ctx, cancel := callCtx()
	defer cancel()
	_, err = r.mx.SendText(ctx, room, text)
	return err
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
	r.emit(ui.ToastMsg{Text: fmt.Sprintf("invited to %s by %s: joined", what, in.Inviter)})
}
