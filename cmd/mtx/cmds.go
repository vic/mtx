package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"mtx/internal/model"
	"mtx/internal/ui"
)

// cmds runs the parsed / commands against Matrix. me is the user; server is its domain.
type cmds struct {
	r      *runner
	me     id.UserID
	server string
}

// cmd is the ui.App handler: the work runs in the returned tea.Cmd; an error comes back as a dim note.
func (c *cmds) cmd(ch string, cm model.Cmd) tea.Cmd {
	return func() tea.Msg {
		if err := c.do(ch, cm); err != nil {
			return ui.NoteMsg{L: dim(err.Error())}
		}
		return nil
	}
}

func dim(text string) model.Line { return model.Line{At: time.Now(), Text: text, Dim: true} }

func roomArg(args []string, ch string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ch
}

// do runs cm; ch is the focused channel, the default room of /part /close /topic /kick /names /whois.
func (c *cmds) do(ch string, cm model.Cmd) error {
	args := cm.Args()
	switch cm.Name() {
	case model.CmdJoin:
		return c.join(args[0])
	case model.CmdPart:
		return c.part(roomArg(args, ch))
	case model.CmdClose:
		return c.closeRoom(roomArg(args, ch))
	case model.CmdTopic:
		return c.topic(ch, args[0])
	case model.CmdInvite:
		return c.invite(args[0], args[1])
	case model.CmdKick:
		return c.kick(ch, args[0])
	case model.CmdMsg:
		return c.msg(args[0], args[1])
	case model.CmdNames:
		return c.names(roomArg(args, ch))
	case model.CmdWhois:
		return c.whois(args[0])
	}
	return fmt.Errorf("command %q is not handled here", cm.Name())
}

func (c *cmds) ctx() (context.Context, context.CancelFunc) { return callCtx() }

// join: /join &name joins #name:<server> and focuses it.
func (c *cmds) join(s string) error {
	if !strings.HasPrefix(s, "&") || len(s) < 2 {
		return errors.New("usage: /join &room")
	}
	alias := "#" + s[1:] + ":" + c.server
	ctx, cancel := c.ctx()
	defer cancel()
	resp, err := c.r.mx.JoinRoom(ctx, alias, nil)
	if err != nil {
		return err
	}
	name := c.r.register(resp.RoomID, alias)
	c.r.emitRooms()
	if err := c.r.emitMembers(resp.RoomID); err != nil {
		return err
	}
	c.r.emit(ui.FocusMsg(name))
	return nil
}

// part leaves the room; the local channel goes away with it.
func (c *cmds) part(ch string) error {
	room, err := c.r.roomOf(ch)
	if err != nil {
		return err
	}
	ctx, cancel := c.ctx()
	defer cancel()
	if _, err := c.r.mx.LeaveRoom(ctx, room); err != nil {
		return err
	}
	c.r.unregister(room)
	c.r.emitRooms()
	return nil
}

// closeRoom leaves the room and forgets it on the server.
func (c *cmds) closeRoom(ch string) error {
	room, err := c.r.roomOf(ch)
	if err != nil {
		return err
	}
	ctx, cancel := c.ctx()
	defer cancel()
	if _, err := c.r.mx.LeaveRoom(ctx, room); err != nil {
		return err
	}
	if _, err := c.r.mx.ForgetRoom(ctx, room); err != nil {
		return err
	}
	c.r.unregister(room)
	c.r.emitRooms()
	return nil
}

func (c *cmds) topic(ch, text string) error {
	room, err := c.r.roomOf(ch)
	if err != nil {
		return err
	}
	ctx, cancel := c.ctx()
	defer cancel()
	_, err = c.r.mx.SendStateEvent(ctx, room, event.StateTopic, "", &event.TopicEventContent{Topic: text})
	return err
}

func (c *cmds) invite(nick, ch string) error {
	if !strings.HasPrefix(nick, "@") {
		return errors.New("usage: /invite <user-id> &room")
	}
	room, err := c.r.roomOf(ch)
	if err != nil {
		return err
	}
	ctx, cancel := c.ctx()
	defer cancel()
	_, err = c.r.mx.InviteUser(ctx, room, &mautrix.ReqInviteUser{UserID: id.UserID(nick)})
	return err
}

func (c *cmds) kick(ch, nick string) error {
	if !strings.HasPrefix(nick, "@") {
		return errors.New("usage: /kick <user-id>")
	}
	room, err := c.r.roomOf(ch)
	if err != nil {
		return err
	}
	ctx, cancel := c.ctx()
	defer cancel()
	_, err = c.r.mx.KickUser(ctx, room, &mautrix.ReqKickUser{UserID: id.UserID(nick)})
	return err
}

func (c *cmds) names(ch string) error {
	room, err := c.r.roomOf(ch)
	if err != nil {
		return err
	}
	nicks, err := c.r.members(room)
	if err != nil {
		return err
	}
	c.r.emit(ui.NoteMsg{L: dim(fmt.Sprintf("%s members: %s", ch, strings.Join(nicks, ", ")))})
	return c.r.emitMembers(room)
}

// msg: /msg <user-id> <text> sends to the DM with that user, creating it when missing.
func (c *cmds) msg(user, text string) error {
	if !strings.HasPrefix(user, "@") {
		return errors.New("usage: /msg <user-id> <text>")
	}
	room, err := c.dmRoom(id.UserID(user))
	if err != nil {
		return err
	}
	ctx, cancel := c.ctx()
	defer cancel()
	_, err = c.r.mx.SendText(ctx, room, text)
	return err
}

// dmRoom finds the m.direct room with user, or creates an encrypted one and records it.
func (c *cmds) dmRoom(user id.UserID) (id.RoomID, error) {
	ctx, cancel := c.ctx()
	defer cancel()
	direct := map[id.UserID][]id.RoomID{}
	if err := c.r.mx.GetAccountData(ctx, string(event.AccountDataDirectChats.Type), &direct); err != nil {
		return "", fmt.Errorf("m.direct: %w", err)
	}
	for _, room := range direct[user] {
		if _, ok := c.r.nameOf(room); ok {
			return room, nil
		}
	}
	resp, err := c.r.mx.CreateRoom(ctx, &mautrix.ReqCreateRoom{
		Invite:   []id.UserID{user},
		IsDirect: true,
		Preset:   "trusted_private_chat",
		InitialState: []*event.Event{{
			Type:     event.StateEncryption,
			StateKey: ptr(""),
			Content:  event.Content{Parsed: &event.EncryptionEventContent{Algorithm: id.AlgorithmMegolmV1}},
		}},
	})
	if err != nil {
		return "", err
	}
	direct[user] = append(direct[user], resp.RoomID)
	if err := c.r.mx.SetAccountData(ctx, string(event.AccountDataDirectChats.Type), direct); err != nil {
		return "", err
	}
	c.r.register(resp.RoomID, "")
	c.r.emitRooms()
	return resp.RoomID, nil
}

// whois shows the Matrix profile of a user.
func (c *cmds) whois(user string) error {
	if !strings.HasPrefix(user, "@") {
		return errors.New("usage: /whois <user-id>")
	}
	ctx, cancel := c.ctx()
	defer cancel()
	p, err := c.r.mx.GetProfile(ctx, id.UserID(user))
	if err != nil {
		return err
	}
	c.r.emit(ui.NoteMsg{L: dim(fmt.Sprintf("%s %s", localpart(user), p.DisplayName))})
	return nil
}

func ptr[T any](v T) *T { return &v }
