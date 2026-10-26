package main

import (
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"mtx/internal/ui"
)

// onTyping shows who types in a room, minus ourselves.
func (r *runner) onTyping(room id.RoomID, users []id.UserID) {
	name, ok := r.nameOf(room)
	if !ok {
		return
	}
	nicks := []string{}
	for _, u := range users {
		if u != r.mx.UserID {
			nicks = append(nicks, localpart(u.String()))
		}
	}
	r.emit(ui.TypingMsg{Ch: name, Nicks: nicks})
}

// noteLast remembers the newest message of a room (read markers point at it).
func (r *runner) noteLast(evt *event.Event) {
	if evt.Type != event.EventMessage && evt.Type != event.EventEncrypted {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.last[evt.RoomID] = evt.ID
}

// seen sends the read markers of channel ch to its newest message, once per message.
func (r *runner) seen(ch string) error {
	if r.loadHistory != nil {
		if room, err := r.roomOf(ch); err == nil {
			r.loadHistory(room)
		}
	}
	if room, err := r.roomOf(ch); err == nil && r.mx != nil {
		r.threadsOnce(room)
		if _, done := r.fullyAsked.LoadOrStore(room, true); !done {
			go func() {
				ctx, cancel := callCtx()
				defer cancel()
				var fr struct {
					EventID string `json:"event_id"`
				}
				if err := r.mx.GetRoomAccountData(ctx, room, "m.fully_read", &fr); err == nil && fr.EventID != "" {
					r.emit(ui.FullyReadMsg{Ch: ch, ID: fr.EventID})
				}
			}()
		}
	}
	room, err := r.roomOf(ch)
	if err != nil {
		return err
	}
	r.mu.Lock()
	last, ok := r.last[room]
	r.mu.Unlock()
	if !ok {
		return nil
	}
	r.mu.Lock()
	if r.marked[room] == last {
		r.mu.Unlock()
		return nil
	}
	r.marked[room] = last
	r.mu.Unlock()
	ctx, cancel := callCtx()
	defer cancel()
	return r.mx.SetReadMarkers(ctx, room, &mautrix.ReqSetReadMarkers{Read: last, FullyRead: last})
}
