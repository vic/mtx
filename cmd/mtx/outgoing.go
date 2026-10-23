package main

import (
	"context"
	"fmt"
	"regexp"
	"slices"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
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
