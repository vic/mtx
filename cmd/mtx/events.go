package main

import (
	"fmt"
	"time"

	"maunium.net/go/mautrix/event"

	"mtx/internal/model"
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
