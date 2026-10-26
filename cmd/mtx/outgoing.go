package main

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"
	"maunium.net/go/mautrix/id"

	"mtx/internal/matrix"
	"mtx/internal/model"
)

// content builds the m.room.message for text. A joined member written as @localpart is a mention
// (m.mentions), so other clients highlight it.
func (r *runner) content(ctx context.Context, room id.RoomID, text string) (*event.MessageEventContent, error) {
	md := format.RenderMarkdown(text, true, false) // markdown in, no raw HTML from the user
	c := &md
	c.Body = text // the plain body stays exactly what was typed
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
	if c.Mentions != nil && len(c.Mentions.UserIDs) == 0 { // markdown leaves an empty object: no mention, no field
		c.Mentions = nil
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

// edit replaces my message target (a line with an event ID) with text: an m.replace event.
func (r *runner) edit(ch string, target model.Line, text string) error {
	room, err := r.roomOf(ch)
	if err != nil {
		return err
	}
	ctx, cancel := callCtx()
	defer cancel()
	content := &event.MessageEventContent{
		MsgType:    event.MsgText,
		Body:       "* " + text,
		NewContent: &event.MessageEventContent{MsgType: event.MsgText, Body: text},
		RelatesTo:  (&event.RelatesTo{}).SetReplace(id.EventID(target.ID)),
	}
	return matrix.Retry(ctx, sendAttempts, func() error {
		_, err := r.mx.SendMessageEvent(ctx, room, event.EventMessage, content)
		return err
	})
}

// mediaText is the scrollback text of a media message: its name and, for unencrypted media, the
// homeserver download URL (clickable). Encrypted media has no link yet.
func (r *runner) mediaText(m *event.MessageEventContent) string {
	kind := strings.TrimPrefix(string(m.MsgType), "m.")
	if m.File != nil {
		return fmt.Sprintf("[%s encrypted] %s", kind, m.Body)
	}
	u, err := id.ParseContentURI(string(m.URL))
	if err != nil {
		return fmt.Sprintf("[%s] %s", kind, m.Body)
	}
	link := fmt.Sprintf("%s/_matrix/media/v3/download/%s/%s", strings.TrimSuffix(r.mx.HomeserverURL.String(), "/"), u.Homeserver, u.FileID)
	return fmt.Sprintf("[%s] %s %s", kind, m.Body, link)
}
