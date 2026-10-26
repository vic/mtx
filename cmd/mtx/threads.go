package main

import (
	"net/http"
	"slices"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"mtx/internal/model"
	"mtx/internal/ui"
)

// threadRoot is what a thread head shows before its root text is known (the root came before
// this session and was not replayed).
const unknownRoot = "(thread)"

// noteThread counts a thread reply and emits the channel's heads. The head's text comes from the
// root message when it was seen (r.texts); a root not seen shows unknownRoot.
func (r *runner) noteThread(evt *event.Event, text string, old bool) {
	root := threadOf(evt)
	if root == "" {
		return
	}
	heads := r.bumpHead(evt.RoomID, root, old, time.UnixMilli(evt.Timestamp))
	r.mu.Lock()
	_, known := r.texts[id.EventID(root)]
	r.mu.Unlock()
	if !known {
		go r.fetchRoot(evt.RoomID, id.EventID(root)) // the head shows the root text when it lands
	}
	name, ok := r.nameOf(evt.RoomID)
	if ok {
		r.emit(ui.ThreadsMsg{Ch: name, Heads: append([]model.Thread(nil), heads...)})
	}
}

// noteText remembers a message's sender and text, so a later thread head can show its root.
func (r *runner) noteText(evt *event.Event, text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.texts) >= maxTexts {
		r.texts = map[id.EventID]text2{}
	}
	r.texts[evt.ID] = text2{nick: localpart(evt.Sender.String()), text: text}
}

const maxTexts = 2000

type text2 struct{ nick, text string }

// threadFallback is the newest message of the room, the spec's fallback for a thread reply.
func (r *runner) threadFallback(room id.RoomID, root id.EventID) id.EventID {
	r.mu.Lock()
	defer r.mu.Unlock()
	if last, ok := r.last[room]; ok {
		return last
	}
	return root
}

// bumpHead counts one reply under root in room and returns the room's heads. The lock is released
// by defer, so a panic in the handler cannot leave r.mu held.
func (r *runner) bumpHead(room id.RoomID, root string, old bool, at time.Time) []model.Thread {
	r.mu.Lock()
	defer r.mu.Unlock()
	heads := r.heads[room]
	found := false
	for i := range heads {
		if heads[i].Root == root {
			heads[i].Count++
			heads[i].At = at
			if !old {
				heads[i].Live++
			}
			found = true
		}
	}
	if !found {
		h := model.Thread{Root: root, Nick: unknownRoot, Count: 1, At: at}
		if !old {
			h.Live = 1
		}
		if t, ok := r.texts[id.EventID(root)]; ok {
			h.Nick, h.Text = t.nick, t.text
		}
		heads = append(heads, h)
	}
	r.heads[room] = heads
	return heads
}

// fetchRoot loads a thread root this session did not see, so its head can show the root's text.
func (r *runner) fetchRoot(room id.RoomID, root id.EventID) {
	ctx, cancel := callCtx()
	defer cancel()
	ev, err := r.mx.GetEvent(ctx, room, root)
	if err != nil {
		return
	}
	text := "(encrypted)"
	if ev.Type != event.EventEncrypted {
		if ev.Content.ParseRaw(ev.Type) == nil {
			if t, _, ok := r.messageText(ev, nil); ok {
				text = t
			}
		}
	}
	nick := localpart(ev.Sender.String())
	r.mu.Lock()
	r.texts[root] = text2{nick: nick, text: text}
	r.mu.Unlock()
	if name, ok := r.nameOf(room); ok {
		r.emit(ui.ThreadRootMsg{Ch: name, Root: string(root), Nick: nick, Text: text})
	}
}

// threadsResp is GET /_matrix/client/v1/rooms/{room}/threads: thread roots, newest activity first.
type threadsResp struct {
	Chunk []*event.Event `json:"chunk"`
}

// fetchThreads loads every thread root of room (up to 100) from the server and merges them into the
// room's heads, so heads show even when no reply was seen this session. Runs in its own goroutine.
func (r *runner) fetchThreads(room id.RoomID) {
	ctx, cancel := callCtx()
	defer cancel()
	var resp threadsResp
	url := r.mx.BuildURLWithQuery(mautrix.ClientURLPath{"v1", "rooms", room, "threads"}, map[string]string{"limit": "100"})
	if _, err := r.mx.MakeRequest(ctx, http.MethodGet, url, nil, &resp); err != nil {
		logf("threads %s: %v", room, err)
		return
	}
	r.mu.Lock()
	heads := r.heads[room]
	for _, ev := range resp.Chunk {
		h := model.Thread{Root: ev.ID.String(), Nick: localpart(ev.Sender.String()), Text: "(encrypted)", At: time.UnixMilli(ev.Timestamp)}
		if ev.Type != event.EventEncrypted && ev.Content.ParseRaw(ev.Type) == nil {
			if t, _, ok := r.messageText(ev, nil); ok {
				h.Text = t
			}
		}
		if rel := ev.Unsigned.Relations; rel != nil && rel.Thread != nil {
			h.Count = rel.Thread.Count
			if l := rel.Thread.LatestEvent; l != nil {
				h.At = time.UnixMilli(l.Timestamp)
			}
		}
		if i := slices.IndexFunc(heads, func(x model.Thread) bool { return x.Root == h.Root }); i >= 0 {
			h.Live = heads[i].Live // live counts belong to this session
			heads[i] = h
		} else {
			heads = append(heads, h)
		}
	}
	slices.SortStableFunc(heads, func(a, b model.Thread) int { return a.At.Compare(b.At) }) // newest last
	r.heads[room] = heads
	out := append([]model.Thread(nil), heads...)
	r.mu.Unlock()
	if name, ok := r.nameOf(room); ok {
		r.emit(ui.ThreadsMsg{Ch: name, Heads: out})
	}
}

// threadsOnce starts fetchThreads for room the first time the room is shown.
func (r *runner) threadsOnce(room id.RoomID) {
	if _, done := r.threadsAsked.LoadOrStore(room, true); !done {
		go r.fetchThreads(room)
	}
}
