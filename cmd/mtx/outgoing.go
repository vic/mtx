package main

import (
	"context"
	"fmt"
	"regexp"
	"slices"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"mtx/internal/matrix"
	"mtx/internal/model"
)

// content builds the m.room.message for text. A joined member written as @localpart is a mention
// (m.mentions), so other clients highlight it.
func (r *runner) content(ctx context.Context, room id.RoomID, text string) (*event.MessageEventContent, error) {
	c := &event.MessageEventContent{MsgType: event.MsgText, Body: text}
	resp, err := r.mx.JoinedMembers(ctx, room)
	if err != nil {
		return nil, fmt.Errorf("members for mentions: %w", err)
	}
	var mentioned []id.UserID
	for u := range resp.Joined {
		re := regexp.MustCompile(`@` + regexp.QuoteMeta(localpart(u.String())) + `(\W|$)`)
		if re.MatchString(text) {
			mentioned = append(mentioned, u)
		}
	}
	if len(mentioned) > 0 {
		slices.Sort(mentioned)
		c.Mentions = &event.Mentions{UserIDs: mentioned}
	}
	return c, nil
}

// reply sends text as a reply to to (a line with an event ID). A line without one is a plain send.
func (r *runner) reply(ch string, to model.Line, text string) error {
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
	if to.Thread != "" { // a thread reply: the relation names the root; the fallback is the newest message
		root := id.EventID(to.Thread)
		content.RelatesTo = (&event.RelatesTo{}).SetThread(root, r.threadFallback(room, root))
	} else if to.ID != "" {
		content.RelatesTo = &event.RelatesTo{InReplyTo: &event.InReplyTo{EventID: id.EventID(to.ID)}}
	}
	return matrix.Retry(ctx, sendAttempts, func() error {
		_, err := r.mx.SendMessageEvent(ctx, room, event.EventMessage, content)
		return err
	})
}
