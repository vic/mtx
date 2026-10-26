package matrix

import (
	"fmt"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// OnTyping sets the hook called with the typing users of a joined room, once per batch that has
// typing data (an empty list means nobody is typing now).
func (c *Client) OnTyping(fn func(room id.RoomID, users []id.UserID)) { c.onTyping = fn }

// typingOf delivers the m.typing ephemeral events of room to the hook.
func (c *Client) typingOf(room id.RoomID, events []*event.Event) error {
	for _, e := range events {
		if e.Type.Type != event.EphemeralEventTyping.Type {
			continue
		}
		if err := e.Content.ParseRaw(event.EphemeralEventTyping); err != nil {
			return fmt.Errorf("typing %s: %w", room, err)
		}
		if t := e.Content.AsTyping(); t != nil && c.onTyping != nil {
			c.onTyping(room, t.UserIDs)
		}
	}
	return nil
}

// OnReceipt sets the hook called for each read receipt of a joined room: user read up to ev.
func (c *Client) OnReceipt(fn func(room id.RoomID, ev id.EventID, user id.UserID)) { c.onReceipt = fn }

// receiptsOf delivers the m.receipt ephemeral events of room to the hook.
func (c *Client) receiptsOf(room id.RoomID, events []*event.Event) error {
	for _, e := range events {
		if e.Type.Type != event.EphemeralEventReceipt.Type || c.onReceipt == nil {
			continue
		}
		if err := e.Content.ParseRaw(event.EphemeralEventReceipt); err != nil {
			return fmt.Errorf("receipt %s: %w", room, err)
		}
		r := e.Content.AsReceipt()
		if r == nil {
			continue
		}
		for ev, rs := range *r {
			for typ, users := range rs {
				if typ != event.ReceiptTypeRead && typ != event.ReceiptTypeReadPrivate {
					continue
				}
				for u := range users {
					c.onReceipt(room, ev, u)
				}
			}
		}
	}
	return nil
}
