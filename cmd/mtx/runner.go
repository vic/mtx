package main

import (
	"cmp"
	"context"
	"errors"
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
	// loadHistory starts a room's history when the room is entered; nil in tests
	loadHistory func(id.RoomID)
	hist        map[string][]model.Line // history lines waiting for flushHistory, by channel
	memberBusy  map[id.RoomID]bool      // a nick-list fetch is running for the room
	memberAgain map[id.RoomID]bool      // the room changed while a fetch ran: fetch once more
	placeholder map[string]bool         // event IDs shown as decrypting and not yet replaced

	mu     sync.Mutex
	last   map[id.RoomID]id.EventID     // newest message per room (read markers)
	marked map[id.RoomID]id.EventID     // newest message per room already marked read
	heads  map[id.RoomID][]model.Thread // thread heads per room
	texts  map[id.EventID]text2         // recent message text for thread roots
	names  map[string]id.RoomID         // "&name" -> room
	ids    map[id.RoomID]string         // room -> "&name"
}

func newRunner(mx *mautrix.Client, emit func(tea.Msg)) *runner {
	return &runner{mx: mx, emit: emit, names: map[string]id.RoomID{}, ids: map[id.RoomID]string{}, last: map[id.RoomID]id.EventID{}, marked: map[id.RoomID]id.EventID{}, heads: map[id.RoomID][]model.Thread{}, texts: map[id.EventID]text2{}, placeholder: map[string]bool{}}
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
// register names room by label; an empty label or a taken name falls back to the room ID.
func (r *runner) register(room id.RoomID, label string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if name, ok := r.ids[room]; ok {
		return name
	}
	if label == "" {
		label = localpart(string(room))
	}
	name := label
	if _, taken := r.names[name]; taken {
		name = localpart(string(room))
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

func (r *runner) emitRooms() {
	logf("rooms %q", r.rooms())
	r.emit(ui.RoomsMsg(r.rooms()))
}

// members returns the joined user localparts and their power levels, admins first (level desc),
// then by name.
func (r *runner) members(room id.RoomID) ([]string, []int, error) {
	ctx, cancel := callCtx()
	defer cancel()
	resp, err := r.mx.JoinedMembers(ctx, room)
	if err != nil {
		return nil, nil, err
	}
	var pl event.PowerLevelsEventContent
	if err := r.mx.StateEvent(ctx, room, event.StatePowerLevels, "", &pl); err != nil {
		pl = event.PowerLevelsEventContent{} // no power levels: everyone is a member
	}
	type member struct {
		nick  string
		level int
	}
	ms := make([]member, 0, len(resp.Joined))
	for u := range resp.Joined {
		ms = append(ms, member{localpart(u.String()), pl.GetUserLevel(u)})
	}
	slices.SortFunc(ms, func(a, b member) int {
		if a.level != b.level {
			return b.level - a.level
		}
		return strings.Compare(a.nick, b.nick)
	})
	nicks := make([]string, len(ms))
	levels := make([]int, len(ms))
	for i, m := range ms {
		nicks[i], levels[i] = m.nick, m.level
	}
	return nicks, levels, nil
}

func (r *runner) emitMembers(room id.RoomID) error {
	name, ok := r.nameOf(room)
	if !ok {
		return nil
	}
	nicks, levels, err := r.members(room)
	if err != nil {
		return err
	}
	r.emit(ui.MembersMsg{Ch: name, Nicks: nicks, Levels: levels})
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
	total := len(resp.JoinedRooms)
	r.emit(ui.ProgressMsg{ID: "rooms", Text: "loading rooms", Total: total})
	// fetches run in goroutines; this loop registers and shows each room as its fetch lands
	metas := make(chan roomMeta)
	go func() {
		var wg sync.WaitGroup
		sem := make(chan struct{}, refreshWorkers)
		for _, room := range resp.JoinedRooms {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				metas <- r.fetchRoom(room)
			}()
		}
		wg.Wait()
		close(metas)
	}()
	done := 0
	var firstErr error
	for m := range metas {
		done++
		r.emit(ui.ProgressMsg{ID: "rooms", Text: "loading rooms", Done: done, Total: total, Finished: done == total})
		if m.err != nil {
			firstErr = cmp.Or(firstErr, m.err)
			continue
		}
		r.register(m.room, roomLabel(m))
		r.emitRooms()
		name, _ := r.nameOf(m.room)
		r.emit(ui.MembersMsg{Ch: name, Nicks: m.nicks, Levels: m.levels})
	}
	return firstErr
}

// refreshWorkers caps the room fetches in flight at start.
const refreshWorkers = 16

// roomMeta is what the channel list needs from one room, fetched at start.
type roomMeta struct {
	room        id.RoomID
	name, alias string
	nicks       []string
	levels      []int
	err         error
}

func (r *runner) fetchRoom(room id.RoomID) roomMeta {
	ctx, cancel := callCtx()
	defer cancel()
	m := roomMeta{room: room, alias: r.canonicalAlias(room)}
	var n event.RoomNameEventContent
	if err := r.mx.StateEvent(ctx, room, event.StateRoomName, "", &n); err == nil {
		m.name = n.Name
	}
	m.nicks, m.levels, m.err = r.members(room)
	return m
}

// roomLabel is the channel name: the room name as Element shows it, else the alias.
func roomLabel(m roomMeta) string {
	if n := strings.Join(strings.Fields(strings.ToLower(m.name)), "-"); n != "" {
		return n
	}
	return localpart(m.alias)
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
		r.membersAsync(evt.RoomID) // the nick list is a fetch: off the sync path, result comes as a message
		return nil
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
	line := model.Line{At: time.UnixMilli(evt.Timestamp), Nick: localpart(evt.Sender.String()), Text: text, Dim: dim, ID: evt.ID.String(), Thread: threadOf(evt)}
	r.mu.Lock()
	replacing := r.placeholder[line.ID]
	if decErr == matrix.ErrPending {
		r.placeholder[line.ID] = true
	} else {
		delete(r.placeholder, line.ID)
	}
	r.mu.Unlock()
	switch {
	case replacing && decErr != matrix.ErrPending:
		r.emit(ui.ReplaceLineMsg{Ch: name, ID: line.ID, L: line}) // the decrypted text replaces its placeholder
	case old:
		r.bufferHistory(name, line) // one batch per room, see flushHistory
	default:
		r.emit(ui.LineMsg{Ch: name, L: line})
	}
	if decErr != matrix.ErrPending {
		r.noteText(evt, text)
		r.noteThread(evt, text, old)
	}
	return nil
}

// messageText is the scrollback text of a message: body, an edit (new content), or the decrypt
// failure. A mention of us in m.mentions is written as @user so the mentions filter sees it.
func (r *runner) messageText(evt *event.Event, decErr error) (text string, dim, ok bool) {
	if errors.Is(decErr, matrix.ErrPending) {
		return "<decrypting message…>", true, true
	}
	if decErr != nil {
		return fmt.Sprintf("[undecryptable: %v]", decErr), true, true
	}
	m := evt.Content.AsMessage()
	if m == nil {
		return "", false, false
	}
	body := m.Body
	switch m.MsgType {
	case event.MsgImage, event.MsgVideo, event.MsgAudio, event.MsgFile:
		body = r.mediaText(m)
	}
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

// sendLiteral sends a : line as is. Our own lines come back in sync and are shown from there.
func (r *runner) sendLiteral(ch, text string) error {
	if err := r.send(ch, text); err != nil {
		return err
	}
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

// bufferHistory holds a history line until flushHistory sends the room's batch.
func (r *runner) bufferHistory(ch string, l model.Line) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.hist == nil {
		r.hist = map[string][]model.Line{}
	}
	r.hist[ch] = append(r.hist[ch], l)
}

// flushHistory sends the buffered history of ch as one update.
func (r *runner) flushHistory(ch string) {
	r.mu.Lock()
	ls := r.hist[ch]
	delete(r.hist, ch)
	r.mu.Unlock()
	if len(ls) > 0 {
		r.emit(ui.HistoryLinesMsg{Ch: ch, L: ls})
	}
}

// membersAsync refreshes a room's nick list in a goroutine. One fetch runs per room at a time;
// a change that arrives meanwhile triggers one more fetch after it.
func (r *runner) membersAsync(room id.RoomID) {
	r.mu.Lock()
	if r.memberBusy == nil {
		r.memberBusy, r.memberAgain = map[id.RoomID]bool{}, map[id.RoomID]bool{}
	}
	if r.memberBusy[room] {
		r.memberAgain[room] = true
		r.mu.Unlock()
		return
	}
	r.memberBusy[room] = true
	r.mu.Unlock()
	go func() {
		for {
			_ = r.emitMembers(room) // a failed fetch leaves the old nick list; the next change retries
			r.mu.Lock()
			again := r.memberAgain[room]
			delete(r.memberAgain, room)
			if !again {
				delete(r.memberBusy, room)
				r.mu.Unlock()
				return
			}
			r.mu.Unlock()
		}
	}()
}
