package main

import (
	"fmt"
	"time"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"mtx/internal/matrix"
	"mtx/internal/model"
	"mtx/internal/ui"
)

// systemLine is a dim line for a state change (membership, topic). Nick is empty.
func systemLine(evt *event.Event, text string) model.Line {
	return model.Line{At: timeOf(evt), Text: text, Dim: true}
}

// membershipText describes a membership change; ok is false when nothing changed.
func membershipText(evt *event.Event) (string, bool) {
	cur := evt.Content.AsMember()
	if cur == nil {
		return "", false
	}
	prev := event.MemberEventContent{Membership: "unknown"}
	if p := evt.Unsigned.PrevContent; p != nil {
		if err := parseIfRaw(p, event.StateMember); err == nil {
			if pm := p.AsMember(); pm != nil {
				prev = *pm
			}
		}
	}
	if prev.Membership == cur.Membership {
		return "", false
	}
	target := localpart(evt.GetStateKey())
	by := localpart(evt.Sender.String())
	switch cur.Membership {
	case event.MembershipJoin:
		return fmt.Sprintf("%s joined", target), true
	case event.MembershipInvite:
		return fmt.Sprintf("%s invited by %s", target, by), true
	case event.MembershipBan:
		return fmt.Sprintf("%s banned by %s", target, by), true
	case event.MembershipLeave:
		switch {
		case evt.Sender.String() == evt.GetStateKey():
			return fmt.Sprintf("%s left", target), true
		case prev.Membership == event.MembershipBan:
			return fmt.Sprintf("%s unbanned by %s", target, by), true
		default:
			return fmt.Sprintf("%s kicked by %s", target, by), true
		}
	}
	return "", false
}

// topicText is the line for a topic change.
func topicText(evt *event.Event) (string, bool) {
	t := evt.Content.AsTopic()
	if t == nil {
		return "", false
	}
	return fmt.Sprintf("%s set topic: %s", localpart(evt.Sender.String()), t.Topic), true
}

func timeOf(evt *event.Event) time.Time { return time.UnixMilli(evt.Timestamp) }

// parseIfRaw parses c unless it is already parsed (ParseRaw rejects parsed content).
func parseIfRaw(c *event.Content, typ event.Type) error {
	if c.Parsed != nil {
		return nil
	}
	return c.ParseRaw(typ)
}

// leftRoom reports whether a membership event removes the user from the room (leave or ban).
func leftRoom(evt *event.Event) bool {
	cur := evt.Content.AsMember()
	return cur != nil && (cur.Membership == event.MembershipLeave || cur.Membership == event.MembershipBan)
}

// threadOf is the thread root of a message, "" for main-chat messages (and for a thread root itself).
func threadOf(evt *event.Event) string {
	if m := evt.Content.AsMessage(); m != nil && m.RelatesTo != nil {
		return string(m.RelatesTo.GetThreadParent())
	}
	return ""
}

// onReceipt passes other members' read receipts to the UI.
func (r *runner) onReceipt(room id.RoomID, ev id.EventID, user id.UserID) {
	if user == r.mx.UserID {
		return
	}
	if name, ok := r.nameOf(room); ok {
		r.emit(ui.ReadMsg{Ch: name, ID: ev.String(), Nick: localpart(user.String())})
	}
}

// typing tells channel ch's room that I am typing, for 10 s; best effort, off the UI loop.
func (r *runner) typing(ch string) {
	room, err := r.roomOf(ch)
	if err != nil {
		return
	}
	go func() {
		ctx, cancel := callCtx()
		defer cancel()
		_, _ = r.mx.UserTyping(ctx, room, true, 10*time.Second)
	}()
}

// onPresence passes a user's presence (online, unavailable, offline) to the UI.
func (r *runner) onPresence(u id.UserID, p string) {
	r.emit(ui.PresenceMsg{Nick: localpart(u.String()), State: p})
}

// onVerify passes a device verification step to the UI.
func (r *runner) onVerify(e matrix.VerifyEvent) {
	r.emit(ui.VerifyMsg{Kind: e.Kind, Txn: e.Txn, From: e.From, Emojis: e.Emojis, Reason: e.Reason})
}
