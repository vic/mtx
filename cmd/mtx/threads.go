package main

import (
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
	r.mu.Lock()
	heads := r.heads[evt.RoomID]
	found := false
	for i := range heads {
		if heads[i].Root == root {
			heads[i].Count++
			if !old {
				heads[i].Live++
			}
			found = true
		}
	}
	if !found {
		h := model.Thread{Root: root, Nick: unknownRoot, Count: 1}
		if !old {
			h.Live = 1
		}
		if t, ok := r.texts[id.EventID(root)]; ok {
			h.Nick, h.Text = t.nick, t.text
		}
		heads = append(heads, h)
	}
	r.heads[evt.RoomID] = heads
	r.mu.Unlock()
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
