package main

import (
	"time"

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
