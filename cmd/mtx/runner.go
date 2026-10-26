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
	loadHistory  func(id.RoomID)
	hist         map[string][]model.Line                            // history lines waiting for flushHistory, by channel
	memberBusy   map[id.RoomID]bool                                 // a nick-list fetch is running for the room
	memberAgain  map[id.RoomID]bool                                 // the room changed while a fetch ran: fetch once more
	placeholder  map[string]bool                                    // event IDs shown as decrypting and not yet replaced
	reactOrder   map[id.EventID][]string                            // per reacted event: emoji in first-seen order
	reactBy      map[id.EventID]map[string]map[id.UserID]id.EventID // target -> key -> reactor -> reaction event; guarded by mu
	reactOf      map[id.EventID]reactRef                            // reaction event -> what it reacted to; guarded by mu
	invites      map[id.RoomID]matrix.Invite                        // pending invites; guarded by mu
	spaces       map[id.RoomID]bool                                 // known spaces: not channels, their events are ignored; guarded by mu
	media        map[id.EventID]*event.MessageEventContent          // media messages by event, for /open; guarded by mu
	threadsAsked sync.Map                                           // rooms whose thread list was fetched
	cacheLine    func(matrix.CachedLine)                            // saves a shown line to the local cache; nil = off
	cacheRooms   func(map[id.RoomID]string)                         // saves room labels; nil = off
	refreshMu    sync.Mutex                                         // one room refresh at a time; see roomName

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
		label = "#" + localpart(string(room))
	}
	name := label
	if _, taken := r.names[name]; taken {
		name = "#" + localpart(string(room))
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
	r.mu.Lock()
	ids := make(ui.IDsMsg, len(r.ids))
	for room, name := range r.ids {
		ids[name] = room.String()
	}
	r.mu.Unlock()
	r.emit(ids)
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
	tags := ui.TagsMsg{}
	spaces := map[id.RoomID]roomMeta{}
	for m := range metas {
		done++
		r.emit(ui.ProgressMsg{ID: "rooms", Text: "loading rooms", Done: done, Total: total, Finished: done == total})
		if m.err != nil {
			firstErr = cmp.Or(firstErr, m.err)
			continue
		}
		if m.space {
			spaces[m.room] = m
			r.mu.Lock()
			if r.spaces == nil {
				r.spaces = map[id.RoomID]bool{}
			}
			r.spaces[m.room] = true
			r.mu.Unlock()
			continue
		}
		r.register(m.room, roomLabel(m, localpart(r.mx.UserID.String())))
		r.emitRooms()
		name, _ := r.nameOf(m.room)
		tags[name] = m.tag
		r.emit(ui.MembersMsg{Ch: name, Nicks: m.nicks, Levels: m.levels})
		r.emit(ui.PinsMsg{Ch: name, IDs: m.pins})
	}
	for _, s := range spaces {
		label := strings.TrimPrefix(roomLabel(s, ""), "#")
		for _, c := range s.children {
			if name, ok := r.nameOf(c); ok && tags[name] == "" && label != "" {
				tags[name] = "space:" + label
			}
		}
	}
	r.emit(tags)
	if r.cacheRooms != nil {
		r.cacheRooms(r.labels())
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
	tag         string // "favourite", "lowpriority" or ""
	space       bool
	pins        []string
	children    []id.RoomID
}

func (r *runner) fetchRoom(room id.RoomID) roomMeta {
	ctx, cancel := callCtx()
	defer cancel()
	m := roomMeta{room: room, alias: r.canonicalAlias(room)}
	var n event.RoomNameEventContent
	if err := r.mx.StateEvent(ctx, room, event.StateRoomName, "", &n); err == nil {
		m.name = n.Name
	}
	if tags, err := r.mx.GetTags(ctx, room); err == nil {
		m.tag = tagGroup(tags.Tags)
	}
	var pe event.PinnedEventsEventContent
	if err := r.mx.StateEvent(ctx, room, event.StatePinnedEvents, "", &pe); err == nil {
		for _, e := range pe.Pinned {
			m.pins = append(m.pins, e.String())
		}
	}
	var create event.CreateEventContent
	if err := r.mx.StateEvent(ctx, room, event.StateCreate, "", &create); err == nil && create.Type == event.RoomTypeSpace {
		m.space = true
		if state, err := r.mx.State(ctx, room); err == nil {
			for key := range state[event.StateSpaceChild] {
				m.children = append(m.children, id.RoomID(key))
			}
		}
	}
	m.nicks, m.levels, m.err = r.members(room)
	return m
}

// tagGroup is the channel group of a room's tags: favourite wins over low priority.
func tagGroup(tags event.Tags) string {
	switch {
	case tags == nil:
		return ""
	case hasTag(tags, event.RoomTagFavourite):
		return "favourite"
	case hasTag(tags, event.RoomTagLowPriority):
		return "lowpriority"
	}
	return ""
}

func hasTag(tags event.Tags, t event.RoomTag) bool { _, ok := tags[t]; return ok }

// roomLabel is the channel name: "#name" for a named room (name or alias), "@nick" for a room named
// after its other members (a DM), else "" (register then falls back to the room ID).
func roomLabel(m roomMeta, me string) string {
	if n := strings.Join(strings.Fields(strings.ToLower(m.name)), "-"); n != "" {
		return "#" + n
	}
	if a := localpart(m.alias); a != "" {
		return "#" + a
	}
	var others []string
	for _, n := range m.nicks {
		if n != me {
			others = append(others, n)
		}
	}
	if len(others) > 3 { // a long list is not a name
		others = others[:3]
	}
	if len(others) == 0 {
		return ""
	}
	return "@" + strings.Join(others, ",")
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
	name, ok, err := r.roomName(evt.RoomID)
	if err != nil {
		return err
	}
	if !ok {
		return nil // a room we are not in (left or not joined yet)
	}
	if decErr == nil && evt.Type != event.EventMessage {
		r.mu.Lock()
		was := r.placeholder[evt.ID.String()]
		delete(r.placeholder, evt.ID.String())
		r.mu.Unlock()
		if was { // the "decrypting" line was not a message after all
			r.emit(ui.RemoveLineMsg{Ch: name, ID: evt.ID.String()})
		}
	}
	if evt.Type == event.EventReaction {
		if rc := evt.Content.AsReaction(); rc != nil && rc.RelatesTo.Key != "" {
			r.emit(ui.ReactsMsg{Ch: name, ID: rc.RelatesTo.EventID.String(), Reacts: r.addReact(rc.RelatesTo.EventID, rc.RelatesTo.Key, evt.Sender, evt.ID)})
		}
		return nil
	}
	if evt.Type == event.StateMember {
		if text, ok := membershipText(evt); ok {
			r.systemOut(old, name, systemLine(evt, text))
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
			r.systemOut(old, name, systemLine(evt, fmt.Sprintf("%s renamed the room to %s", localpart(evt.Sender.String()), n.Name)))
		}
		return nil
	}
	if evt.Type == event.StatePinnedEvents {
		if err := evt.Content.ParseRaw(evt.Type); err == nil {
			if pe := evt.Content.AsPinnedEvents(); pe != nil {
				ids := make([]string, 0, len(pe.Pinned))
				for _, e := range pe.Pinned {
					ids = append(ids, e.String())
				}
				r.emit(ui.PinsMsg{Ch: name, IDs: ids})
			}
		}
		return nil
	}
	if evt.Type == event.StateTopic {
		if text, ok := topicText(evt); ok {
			r.systemOut(old, name, systemLine(evt, text))
		}
		return nil
	}
	if evt.Type != event.EventMessage && evt.Type != event.EventEncrypted && evt.Type != event.EventRedaction {
		return nil
	}
	if evt.Type == event.EventRedaction {
		target := evt.Redacts // room v11 puts it in the content
		if target == "" {
			if rc := evt.Content.AsRedaction(); rc != nil {
				target = rc.Redacts
			}
		}
		if t, sum, ok := r.dropReact(target); ok {
			r.emit(ui.ReactsMsg{Ch: name, ID: t.String(), Reacts: sum})
			return nil
		}
		if target != "" {
			r.emit(ui.ReplaceLineMsg{Ch: name, ID: target.String(), L: model.Line{Nick: localpart(evt.Sender.String()), Text: "(deleted)", Dim: true, ID: target.String()}})
		}
		r.systemOut(old, name, systemLine(evt, fmt.Sprintf("%s removed a message", localpart(evt.Sender.String()))))
		return nil
	}
	text, dim, ok := r.messageText(evt, decErr)
	if mm := evt.Content.AsMessage(); mm != nil && isMediaType(mm.MsgType) { // after decryption: the file info is set
		r.recordMedia(evt.ID, mm)
	}
	if matrix.IsLate(ctx) {
		text = "(late) " + text
	}
	if !ok {
		return nil
	}
	line := model.Line{At: time.UnixMilli(evt.Timestamp), Nick: localpart(evt.Sender.String()), Text: text, Dim: dim, ID: evt.ID.String(), Thread: threadOf(evt)}
	if m := evt.Content.AsMessage(); m != nil && m.RelatesTo != nil && m.RelatesTo.Type == event.RelReplace && decErr == nil {
		r.mu.Lock()
		wasPlaceholder := r.placeholder[evt.ID.String()]
		delete(r.placeholder, evt.ID.String())
		r.mu.Unlock()
		if wasPlaceholder {
			r.emit(ui.RemoveLineMsg{Ch: name, ID: evt.ID.String()}) // the "decrypting" line was this edit
		}
		line.ID = m.RelatesTo.EventID.String() // the edit replaces the original line in place
		if !old || !r.replaceBuffered(name, line) {
			r.emit(ui.ReplaceLineMsg{Ch: name, ID: line.ID, L: line})
		}
		if r.cacheLine != nil {
			r.cacheLine(matrix.CachedLine{Room: evt.RoomID, EventID: id.EventID(line.ID), TS: evt.Timestamp, Nick: line.Nick, Text: line.Text, Dim: line.Dim, Thread: line.Thread})
		}
		return nil
	}
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
	if !old && decErr == nil && wantsNotify(name, line.Nick, line.Text, localpart(r.mx.UserID.String())) {
		notify(name+" <"+line.Nick+">", line.Text) // after decryption: the real text
	}
	if decErr != matrix.ErrPending {
		if r.cacheLine != nil {
			r.cacheLine(matrix.CachedLine{Room: evt.RoomID, EventID: evt.ID, TS: evt.Timestamp, Nick: line.Nick, Text: line.Text, Dim: line.Dim, Thread: line.Thread})
		}
		r.noteText(evt, text)
		r.noteThread(evt, text, old)
	}
	return nil
}

// roomName is room's channel name, refreshing the room list when the room is unknown. A caller that
// waited for another refresh checks again before starting its own, so a burst of events from one new
// room costs one refresh.
func (r *runner) roomName(room id.RoomID) (string, bool, error) {
	r.mu.Lock()
	space := r.spaces[room]
	r.mu.Unlock()
	if space { // a space is a group, not a channel: nothing to show, nothing to refresh
		return "", false, nil
	}
	if name, ok := r.nameOf(room); ok {
		return name, true, nil
	}
	r.refreshMu.Lock()
	defer r.refreshMu.Unlock()
	if name, ok := r.nameOf(room); ok {
		return name, true, nil
	}
	if err := r.refresh(); err != nil {
		return "", false, err
	}
	name, ok := r.nameOf(room)
	return name, ok, nil
}

// systemOut shows a state-change line: history goes into the room's history batch (merged by time),
// live lines go out at once.
func (r *runner) systemOut(old bool, ch string, l model.Line) {
	if old {
		r.bufferHistory(ch, l)
		return
	}
	r.emit(ui.LineMsg{Ch: ch, L: l})
}

// replaceBuffered swaps a history line still waiting in ch's batch for l (same ID); false if none.
func (r *runner) replaceBuffered(ch string, l model.Line) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, b := range r.hist[ch] {
		if b.ID == l.ID {
			l.At, l.Thread = b.At, b.Thread
			r.hist[ch][i] = l
			return true
		}
	}
	return false
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
	if m.MsgType == event.MsgEmote {
		body = "* " + localpart(evt.Sender.String()) + " " + body
	}
	switch m.MsgType {
	case event.MsgImage, event.MsgVideo, event.MsgAudio, event.MsgFile:
		body = r.mediaText(m)
	}
	if m.NewContent != nil {
		body = m.NewContent.Body + " (edited)"
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

// toastInvite records a pending invite and tells the user how to answer it. The room is named by its
// alias when it has one.
func (r *runner) toastInvite(in matrix.Invite) {
	r.mu.Lock()
	if r.invites == nil {
		r.invites = map[id.RoomID]matrix.Invite{}
	}
	_, seen := r.invites[in.Room]
	r.invites[in.Room] = in
	r.mu.Unlock()
	if seen {
		return // each sync repeats pending invites: toast once
	}
	what := inviteName(in)
	r.emit(ui.ToastMsg{Text: fmt.Sprintf("invited to %s by %s: /accept %s or /reject %s", what, localpart(in.Inviter.String()), what, what)})
}

// inviteName is how commands name an invite: its alias, else its room ID.
func inviteName(in matrix.Invite) string {
	if in.Alias != "" {
		return in.Alias
	}
	return string(in.Room)
}

// pendingInvite finds the invite named s (alias or room ID); with s empty, the only pending one.
func (r *runner) pendingInvite(s string) (matrix.Invite, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s == "" && len(r.invites) == 1 {
		for _, in := range r.invites {
			return in, nil
		}
	}
	for _, in := range r.invites {
		if s == inviteName(in) || s == string(in.Room) {
			return in, nil
		}
	}
	return matrix.Invite{}, fmt.Errorf("no pending invite %q (see /invites)", s)
}

// dropInvite forgets an answered invite.
func (r *runner) dropInvite(room id.RoomID) {
	r.mu.Lock()
	delete(r.invites, room)
	r.mu.Unlock()
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

// reactRef is what a reaction event reacted to.
type reactRef struct {
	target id.EventID
	key    string
	user   id.UserID
}

// addReact records user's reaction key (event rid) on target and returns target's summary.
func (r *runner) addReact(target id.EventID, key string, user id.UserID, rid id.EventID) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.reactBy == nil {
		r.reactBy, r.reactOf, r.reactOrder = map[id.EventID]map[string]map[id.UserID]id.EventID{}, map[id.EventID]reactRef{}, map[id.EventID][]string{}
	}
	if r.reactBy[target] == nil {
		r.reactBy[target] = map[string]map[id.UserID]id.EventID{}
	}
	if r.reactBy[target][key] == nil {
		r.reactBy[target][key] = map[id.UserID]id.EventID{}
		r.reactOrder[target] = append(r.reactOrder[target], key)
	}
	r.reactBy[target][key][user] = rid
	r.reactOf[rid] = reactRef{target, key, user}
	return r.summaryLocked(target)
}

// dropReact forgets reaction event rid (it was redacted); ok is false when rid is not a known reaction.
func (r *runner) dropReact(rid id.EventID) (target id.EventID, summary string, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ref, ok := r.reactOf[rid]
	if !ok {
		return "", "", false
	}
	delete(r.reactOf, rid)
	delete(r.reactBy[ref.target][ref.key], ref.user)
	return ref.target, r.summaryLocked(ref.target), true
}

// summaryLocked is target's reaction summary ("👍2 ❤️1"), keys in first-seen order; r.mu held.
func (r *runner) summaryLocked(target id.EventID) string {
	var parts []string
	for _, k := range r.reactOrder[target] {
		if n := len(r.reactBy[target][k]); n > 0 {
			parts = append(parts, fmt.Sprintf("%s%d", k, n))
		}
	}
	return strings.Join(parts, " ")
}

// myReact is my reaction event with key on target, "" if I have none.
func (r *runner) myReact(target id.EventID, key string) id.EventID {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reactBy[target][key][r.mx.UserID]
}
