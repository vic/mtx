package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/pushrules"

	"mtx/internal/model"
	"mtx/internal/ui"
)

// cmds runs the parsed / commands against Matrix. me is the user; server is its domain.
// verifier is the device verification of this client (matrix.Client).
type verifier interface {
	StartVerify(context.Context) (string, error)
	AcceptVerify(context.Context, string) error
	ConfirmVerify(context.Context, string) error
	CancelVerify(context.Context, string) error
}

type cmds struct {
	r      *runner
	me     id.UserID
	server string
	inv    inviteAnswerer
	ver    verifier
	rec    interface {
		Recover(context.Context, string) error
	}
}

// inviteAnswerer accepts or rejects invites (the matrix client; a fake in tests).
type inviteAnswerer interface {
	Accept(ctx context.Context, room id.RoomID) error
	Reject(ctx context.Context, room id.RoomID) error
}

// cmd is the ui.App handler: the work runs in the returned tea.Cmd; an error comes back as a dim note.
func (c *cmds) cmd(ch string, cm model.Cmd) tea.Cmd {
	args := cm.Args()
	if cm.Name() == model.CmdRecover {
		args = []string{"[redacted]"} // the recovery key is a secret
	}
	logf("cmd %s args=%q focus=%q", cm.Name(), args, ch)
	return func() tea.Msg {
		err := c.do(ch, cm)
		logf("cmd %s done err=%v", cm.Name(), err)
		if err != nil {
			return ui.ToastMsg{Text: err.Error(), Err: true}
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
	case model.CmdCreate:
		if len(args) == 0 {
			return errors.New("usage: /create &room [public]")
		}
		return c.createRoom(args[0], len(args) > 1 && args[1] == "public")
	case model.CmdExplore:
		return c.explore(strings.Join(args, " "))
	case model.CmdDevices:
		return c.devices()
	case model.CmdVerify:
		return c.verify(args)
	case model.CmdDM:
		return c.dm(args[0])
	case model.CmdPreview:
		return c.preview(args[0])
	case model.CmdSearch:
		return c.search(ch, args[0])
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
	case model.CmdBan, model.CmdUnban:
		if len(args) == 0 {
			return errors.New("usage: :ban <user> [reason]")
		}
		uid, err := c.fullUser(args[0])
		if err != nil {
			return err
		}
		room, err := c.r.roomOf(ch)
		if err != nil {
			return err
		}
		ctx, cancel := c.ctx()
		defer cancel()
		if cm.Name() == model.CmdUnban {
			_, err = c.r.mx.UnbanUser(ctx, room, &mautrix.ReqUnbanUser{UserID: uid})
			return err
		}
		_, err = c.r.mx.BanUser(ctx, room, &mautrix.ReqBanUser{UserID: uid, Reason: strings.Join(args[1:], " ")})
		return err
	case model.CmdOp:
		uid, err := c.fullUser(args[0])
		if err != nil {
			return err
		}
		level, err := strconv.Atoi(args[1])
		if err != nil {
			return errors.New("usage: :op <user> <level> (100 admin, 50 mod, 0 member)")
		}
		room, err := c.r.roomOf(ch)
		if err != nil {
			return err
		}
		ctx, cancel := c.ctx()
		defer cancel()
		var pl event.PowerLevelsEventContent
		if err := c.r.mx.StateEvent(ctx, room, event.StatePowerLevels, "", &pl); err != nil {
			return err
		}
		if pl.Users == nil {
			pl.Users = map[id.UserID]int{}
		}
		pl.Users[uid] = level
		_, err = c.r.mx.SendStateEvent(ctx, room, event.StatePowerLevels, "", &pl)
		return err
	case model.CmdMsg:
		return c.msg(args[0], args[1])
	case model.CmdNames:
		return c.names(roomArg(args, ch))
	case model.CmdWhois:
		return c.whois(args[0])
	case model.CmdMe:
		return c.emote(ch, args[0])
	case model.CmdOpen:
		return c.r.open(args[0])
	case model.CmdMute, model.CmdUnmute:
		room, err := c.r.roomOf(roomArg(args, ch))
		if err != nil {
			return err
		}
		ctx, cancel := c.ctx()
		defer cancel()
		if cm.Name() == model.CmdUnmute {
			return c.r.mx.DeletePushRule(ctx, "global", pushrules.RoomRule, string(room))
		}
		return c.r.mx.PutPushRule(ctx, "global", pushrules.RoomRule, string(room), &mautrix.ReqPutPushRule{Actions: []*pushrules.PushAction{}})
	case model.CmdRead:
		return c.r.markRead(roomArg(args, ch))
	case model.CmdUnread:
		room, err := c.r.roomOf(roomArg(args, ch))
		if err != nil {
			return err
		}
		ctx, cancel := c.ctx()
		defer cancel()
		return c.r.mx.SetRoomAccountData(ctx, room, "m.marked_unread", map[string]bool{"unread": true})
	case model.CmdJoinRule:
		return c.joinRule(ch, args[0])
	case model.CmdHistory:
		return c.history(ch, args[0])
	case model.CmdAlias:
		return c.alias(ch, args[0])
	case model.CmdRoomAvatar:
		return c.r.setRoomAvatar(ch, args[0])
	case model.CmdAvatar:
		return c.r.setAvatar(args[0])
	case model.CmdUpload:
		return c.r.upload(ch, args[0])
	case model.CmdPin, model.CmdUnpin:
		return c.pin(ch, args[0], cm.Name() == model.CmdPin)
	case model.CmdRecover:
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := c.rec.Recover(ctx, args[0]); err != nil {
			return err
		}
		c.r.emit(ui.ToastMsg{Text: "recovered: keys imported, old messages decrypt on the next sync"})
		return nil
	case model.CmdAccept, model.CmdReject:
		in, err := c.r.pendingInvite(strings.Join(args, " "))
		if err != nil {
			return err
		}
		ctx, cancel := c.ctx()
		defer cancel()
		if cm.Name() == model.CmdAccept {
			err = c.inv.Accept(ctx, in.Room)
		} else {
			err = c.inv.Reject(ctx, in.Room)
		}
		if err == nil {
			c.r.dropInvite(in.Room)
		}
		return err
	case model.CmdInvites:
		c.r.mu.Lock()
		var names []string
		for _, in := range c.r.invites {
			names = append(names, inviteName(in)+" from "+localpart(in.Inviter.String()))
		}
		c.r.mu.Unlock()
		slices.Sort(names)
		if len(names) == 0 {
			names = []string{"none"}
		}
		c.r.emit(ui.NoteMsg{L: dim("invites: " + strings.Join(names, ", "))})
		return nil
	case model.CmdReact:
		return c.react(ch, args[0], args[1])
	case model.CmdRename:
		return c.rename(ch, args[0])
	case model.CmdTag:
		return c.tag(ch, args[0])
	case model.CmdNick:
		ctx, cancel := c.ctx()
		defer cancel()
		return c.r.mx.SetDisplayName(ctx, args[0])
	case model.CmdIgnore, model.CmdUnignore:
		return c.ignore(args[0], cm.Name() == model.CmdIgnore)
	case model.CmdRedact:
		if len(args) == 0 {
			return errors.New("usage: /redact [reason]")
		}
		return c.redact(ch, args[0], strings.Join(args[1:], " "))
	}
	return fmt.Errorf("command %q is not handled here", cm.Name())
}

func (c *cmds) ctx() (context.Context, context.CancelFunc) { return callCtx() }

// markRead sets the read markers of room ch to its newest line and clears a marked-unread flag,
// even when seen already marked it: /read always sends.
func (r *runner) markRead(ch string) error {
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
	ctx, cancel := callCtx()
	defer cancel()
	if err := r.mx.SetReadMarkers(ctx, room, &mautrix.ReqSetReadMarkers{Read: last, FullyRead: last}); err != nil {
		return err
	}
	r.mu.Lock()
	r.marked[room] = last
	r.mu.Unlock()
	return r.mx.SetRoomAccountData(ctx, room, "m.marked_unread", map[string]bool{"unread": false})
}

// join: /join name joins #name:<server> and focuses it.
func (c *cmds) join(s string) error {
	if s == "" {
		return errors.New("usage: /join room")
	}
	alias := joinTarget(s, c.server)
	ctx, cancel := c.ctx()
	defer cancel()
	resp, err := c.r.mx.JoinRoom(ctx, alias, nil)
	if err != nil {
		return err
	}
	label := "#" + localpart(alias) // joined by alias: the channel is named after it
	if strings.HasPrefix(alias, "!") {
		label = roomLabel(c.r.fetchRoom(resp.RoomID), localpart(string(c.me))) // a bare room ID: name it like the room list does
	}
	name := c.r.register(resp.RoomID, label)
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
	uid, err := c.fullUser(nick)
	if err != nil {
		return errors.New("usage: /invite <user-id> room")
	}
	room, err := c.r.roomOf(ch)
	if err != nil {
		return err
	}
	ctx, cancel := c.ctx()
	defer cancel()
	_, err = c.r.mx.InviteUser(ctx, room, &mautrix.ReqInviteUser{UserID: uid})
	return err
}

func (c *cmds) kick(ch, nick string) error {
	uid, err := c.fullUser(nick)
	if err != nil {
		return errors.New("usage: /kick <user-id>")
	}
	room, err := c.r.roomOf(ch)
	if err != nil {
		return err
	}
	ctx, cancel := c.ctx()
	defer cancel()
	_, err = c.r.mx.KickUser(ctx, room, &mautrix.ReqKickUser{UserID: uid})
	return err
}

func (c *cmds) names(ch string) error {
	room, err := c.r.roomOf(ch)
	if err != nil {
		return err
	}
	nicks, _, err := c.r.members(room)
	if err != nil {
		return err
	}
	c.r.emit(ui.NoteMsg{L: dim(fmt.Sprintf("%s members: %s", ch, strings.Join(nicks, ", ")))})
	return c.r.emitMembers(room)
}

// msg: /msg <user-id> <text> sends to the DM with that user, creating it when missing.
func (c *cmds) msg(user, text string) error {
	uid, err := c.fullUser(user)
	if err != nil {
		return errors.New("usage: /msg <user-id> <text>")
	}
	room, err := c.dmRoom(uid)
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
	uid, err := c.fullUser(user)
	if err != nil {
		return errors.New("usage: /whois <user-id>")
	}
	ctx, cancel := c.ctx()
	defer cancel()
	p, err := c.r.mx.GetProfile(ctx, uid)
	if err != nil {
		return err
	}
	c.r.emit(ui.NoteMsg{L: dim(fmt.Sprintf("%s %s", localpart(user), p.DisplayName))})
	return nil
}

func ptr[T any](v T) *T { return &v }

// create: /create &name makes an encrypted private room with alias #name:<server>, invites nobody,
// and focuses it.
func (c *cmds) create(s string) error { return c.createRoom(s, false) }

// createRoom makes the room s: private and encrypted, or public and listed in the directory.
func (c *cmds) createRoom(s string, public bool) error {
	if s == "" {
		return errors.New("usage: /create room")
	}
	ctx, cancel := c.ctx()
	defer cancel()
	req := &mautrix.ReqCreateRoom{
		RoomAliasName: s,
		Preset:        "private_chat",
		InitialState: []*event.Event{{
			Type:     event.StateEncryption,
			StateKey: ptr(""),
			Content:  event.Content{Parsed: &event.EncryptionEventContent{Algorithm: id.AlgorithmMegolmV1}},
		}},
	}
	if public { // listed in the directory, so no encryption
		req = &mautrix.ReqCreateRoom{RoomAliasName: s, Preset: "public_chat", Visibility: "public"}
	}
	resp, err := c.r.mx.CreateRoom(ctx, req)
	if err != nil {
		return err
	}
	alias := "#" + s + ":" + c.server
	logf("create %s room=%s", s, resp.RoomID)
	name := c.r.register(resp.RoomID, "#"+localpart(alias))
	c.r.emitRooms()
	c.r.emit(ui.FocusMsg(name))
	return nil
}

// fullUser is a user ID typed as @local or @local:server. A bare @local gets our server.
func (c *cmds) fullUser(s string) (id.UserID, error) {
	if !strings.HasPrefix(s, "@") || len(s) < 2 {
		return "", fmt.Errorf("expected @user, got %q", s)
	}
	if !strings.Contains(s, ":") {
		s += ":" + c.server
	}
	return id.UserID(s), nil
}

// react toggles my emoji reaction on event ev: sends it, or redacts mine when I already reacted with it.
func (c *cmds) react(ch, emoji, ev string) error {
	room, err := c.r.roomOf(ch)
	if err != nil {
		return err
	}
	ctx, cancel := c.ctx()
	defer cancel()
	if mine := c.r.myReact(id.EventID(ev), emoji); mine != "" {
		_, err = c.r.mx.RedactEvent(ctx, room, mine)
		return err
	}
	_, err = c.r.mx.SendReaction(ctx, room, id.EventID(ev), emoji)
	return err
}

// redact deletes my message ev in channel ch, with an optional reason.
func (c *cmds) redact(ch, ev, reason string) error {
	room, err := c.r.roomOf(ch)
	if err != nil {
		return err
	}
	ctx, cancel := c.ctx()
	defer cancel()
	var req mautrix.ReqRedact
	req.Reason = reason
	_, err = c.r.mx.RedactEvent(ctx, room, id.EventID(ev), req)
	return err
}

// rename sets the room name of channel ch.
func (c *cmds) rename(ch, name string) error {
	room, err := c.r.roomOf(ch)
	if err != nil {
		return err
	}
	ctx, cancel := c.ctx()
	defer cancel()
	_, err = c.r.mx.SendStateEvent(ctx, room, event.StateRoomName, "", &event.RoomNameEventContent{Name: name})
	return err
}

// tag sets my tag group of channel ch: fav, low, or none (both removed). The pane regroups at once.
func (c *cmds) tag(ch, which string) error {
	room, err := c.r.roomOf(ch)
	if err != nil {
		return err
	}
	ctx, cancel := c.ctx()
	defer cancel()
	add, group := event.RoomTag(""), ""
	switch which {
	case "fav":
		add, group = event.RoomTagFavourite, "favourite"
	case "low":
		add, group = event.RoomTagLowPriority, "lowpriority"
	case "none":
	default:
		return errors.New("usage: /tag fav|low|none")
	}
	for _, t := range []event.RoomTag{event.RoomTagFavourite, event.RoomTagLowPriority} {
		if t != add {
			if err := c.r.mx.RemoveTag(ctx, room, t); err != nil {
				return err
			}
		}
	}
	if add != "" {
		if err := c.r.mx.AddTag(ctx, room, add, 0.5); err != nil {
			return err
		}
	}
	c.r.emit(ui.TagsMsg{ch: group})
	return nil
}

// joinTarget is what /join s joins: a room ID or a full alias as given (a matrix.to link is unwrapped),
// else the alias #s on our server.
func joinTarget(s, server string) string {
	if rest, ok := strings.CutPrefix(s, "https://matrix.to/#/"); ok {
		s, _, _ = strings.Cut(rest, "?")
		if u, err := url.PathUnescape(s); err == nil {
			s = u
		}
	}
	s = strings.TrimPrefix(s, "&")
	if strings.HasPrefix(s, "!") || (strings.HasPrefix(s, "#") && strings.Contains(s, ":")) {
		return s
	}
	return "#" + strings.TrimPrefix(s, "#") + ":" + server
}

// ignore adds user to (on) or removes it from my ignored users (m.ignored_user_list account data).
func (c *cmds) ignore(user string, on bool) error {
	uid, err := c.fullUser(user)
	if err != nil {
		return fmt.Errorf("usage: /ignore <user>")
	}
	ctx, cancel := c.ctx()
	defer cancel()
	var list struct {
		Ignored map[id.UserID]struct{} `json:"ignored_users"`
	}
	if err := c.r.mx.GetAccountData(ctx, "m.ignored_user_list", &list); err != nil && !errors.Is(err, mautrix.MNotFound) {
		return err
	}
	if list.Ignored == nil {
		list.Ignored = map[id.UserID]struct{}{}
	}
	if on {
		list.Ignored[uid] = struct{}{}
	} else {
		delete(list.Ignored, uid)
	}
	return c.r.mx.SetAccountData(ctx, "m.ignored_user_list", &list)
}

// emote sends text as an action (m.emote) to channel ch.
func (c *cmds) emote(ch, text string) error {
	room, err := c.r.roomOf(ch)
	if err != nil {
		return err
	}
	ctx, cancel := c.ctx()
	defer cancel()
	_, err = c.r.mx.SendMessageEvent(ctx, room, event.EventMessage, &event.MessageEventContent{MsgType: event.MsgEmote, Body: text})
	return err
}

// pin adds event ev to (on) or removes it from channel ch's pinned messages.
func (c *cmds) pin(ch, ev string, on bool) error {
	room, err := c.r.roomOf(ch)
	if err != nil {
		return err
	}
	ctx, cancel := c.ctx()
	defer cancel()
	var pe event.PinnedEventsEventContent
	if err := c.r.mx.StateEvent(ctx, room, event.StatePinnedEvents, "", &pe); err != nil && !errors.Is(err, mautrix.MNotFound) {
		return err
	}
	pe.Pinned = slices.DeleteFunc(pe.Pinned, func(e id.EventID) bool { return e == id.EventID(ev) })
	if on {
		pe.Pinned = append(pe.Pinned, id.EventID(ev))
	}
	_, err = c.r.mx.SendStateEvent(ctx, room, event.StatePinnedEvents, "", &pe)
	return err
}

// joinRule sets who may join the room: public, invite or knock.
func (c *cmds) joinRule(ch, rule string) error {
	jr := event.JoinRule(rule)
	switch jr {
	case event.JoinRulePublic, event.JoinRuleInvite, event.JoinRuleKnock:
	default:
		return errors.New("usage: :joinrule public|invite|knock")
	}
	room, err := c.r.roomOf(ch)
	if err != nil {
		return err
	}
	ctx, cancel := c.ctx()
	defer cancel()
	_, err = c.r.mx.SendStateEvent(ctx, room, event.StateJoinRules, "", &event.JoinRulesEventContent{JoinRule: jr})
	return err
}

// history sets the history visibility: shared, invited, joined or world (world_readable).
func (c *cmds) history(ch, vis string) error {
	hv := map[string]event.HistoryVisibility{
		"shared": event.HistoryVisibilityShared, "invited": event.HistoryVisibilityInvited,
		"joined": event.HistoryVisibilityJoined, "world": event.HistoryVisibilityWorldReadable,
	}[vis]
	if hv == "" {
		return errors.New("usage: :history shared|invited|joined|world")
	}
	room, err := c.r.roomOf(ch)
	if err != nil {
		return err
	}
	ctx, cancel := c.ctx()
	defer cancel()
	_, err = c.r.mx.SendStateEvent(ctx, room, event.StateHistoryVisibility, "", &event.HistoryVisibilityEventContent{HistoryVisibility: hv})
	return err
}

// alias publishes #alias:server for the room in the directory, then makes it the canonical alias.
func (c *cmds) alias(ch, alias string) error {
	if !strings.HasPrefix(alias, "#") || !strings.Contains(alias, ":") {
		return errors.New("usage: :alias #alias:server")
	}
	room, err := c.r.roomOf(ch)
	if err != nil {
		return err
	}
	ctx, cancel := c.ctx()
	defer cancel()
	if _, err := c.r.mx.CreateAlias(ctx, id.RoomAlias(alias), room); err != nil {
		return err
	}
	_, err = c.r.mx.SendStateEvent(ctx, room, event.StateCanonicalAlias, "", &event.CanonicalAliasEventContent{Alias: id.RoomAlias(alias)})
	return err
}

// explore lists the public rooms of our server in the explore picker; term narrows them by alias, name or topic.
func (c *cmds) explore(term string) error {
	ctx, cancel := c.ctx()
	defer cancel()
	resp, err := c.r.mx.PublicRooms(ctx, &mautrix.ReqPublicRooms{Limit: 50})
	if err != nil {
		return err
	}
	var entries, targets []string
	for _, pr := range resp.Chunk {
		target := string(pr.RoomID)
		if pr.CanonicalAlias != "" {
			target = string(pr.CanonicalAlias)
		}
		if term != "" && !strings.Contains(strings.ToLower(target+" "+pr.Name+" "+pr.Topic), strings.ToLower(term)) {
			continue
		}
		entries = append(entries, target+"  "+pr.Name+" ("+strconv.Itoa(pr.NumJoinedMembers)+")")
		targets = append(targets, target)
	}
	c.r.emit(ui.ExploreMsg{Entries: entries, Targets: targets})
	return nil
}

// search asks the server for messages of channel ch that match term and shows them in a picker.
func (c *cmds) search(ch, term string) error {
	room, err := c.r.roomOf(ch)
	if err != nil {
		return err
	}
	ctx, cancel := c.ctx()
	defer cancel()
	req := map[string]any{"search_categories": map[string]any{"room_events": map[string]any{
		"search_term": term, "order_by": "recent", "filter": map[string]any{"rooms": []string{room.String()}, "limit": 50}}}}
	var resp struct {
		SearchCategories struct {
			RoomEvents struct {
				Results []struct {
					Result *event.Event `json:"result"`
				} `json:"results"`
			} `json:"room_events"`
		} `json:"search_categories"`
	}
	if _, err := c.r.mx.MakeRequest(ctx, http.MethodPost, c.r.mx.BuildClientURL("v3", "search"), req, &resp); err != nil {
		return err
	}
	var entries, ids []string
	for _, r := range resp.SearchCategories.RoomEvents.Results {
		e := r.Result
		if e == nil || e.Content.ParseRaw(e.Type) != nil {
			continue
		}
		if text, _, ok := c.r.messageText(e, nil); ok {
			entries = append(entries, "<"+localpart(e.Sender.String())+"> "+strings.ReplaceAll(text, "\n", " "))
			ids = append(ids, e.ID.String())
		}
	}
	c.r.emit(ui.SearchMsg{Entries: entries, IDs: ids})
	return nil
}

// devices lists my devices, most recently seen first, in the read-only info picker.
func (c *cmds) devices() error {
	ctx, cancel := c.ctx()
	defer cancel()
	resp, err := c.r.mx.GetDevicesInfo(ctx)
	if err != nil {
		return err
	}
	ds := slices.Clone(resp.Devices)
	slices.SortFunc(ds, func(a, b mautrix.RespDeviceInfo) int { return cmp.Compare(b.LastSeenTS, a.LastSeenTS) })
	var entries []string
	for _, d := range ds {
		line := d.DeviceID.String() + "  " + d.DisplayName + "  last seen " + time.UnixMilli(d.LastSeenTS).Format("2006-01-02 15:04")
		if d.DeviceID == c.r.mx.DeviceID {
			line += "  (this device)"
		}
		entries = append(entries, line)
	}
	c.r.emit(ui.ListMsg{Title: "devices", Entries: entries})
	return nil
}

// preview asks the homeserver for the link preview of url and lists its title, site, description in the
// read-only list picker.
func (c *cmds) preview(url string) error {
	ctx, cancel := c.ctx()
	defer cancel()
	var og map[string]any
	u := c.r.mx.BuildURLWithQuery(mautrix.ClientURLPath{"v1", "media", "preview_url"}, map[string]string{"url": url})
	if _, err := c.r.mx.MakeRequest(ctx, http.MethodGet, u, nil, &og); err != nil {
		return err
	}
	var rows []string
	for _, k := range []string{"og:title", "og:site_name", "og:description", "og:url"} {
		if s, ok := og[k].(string); ok && s != "" {
			rows = append(rows, strings.TrimPrefix(k, "og:")+": "+s)
		}
	}
	if len(rows) == 0 {
		rows = []string{"no preview for " + url}
	}
	c.r.emit(ui.ListMsg{Title: "link preview", Entries: rows})
	return nil
}

// dm opens the direct message with user: the room we have with them, else a new one.
func (c *cmds) dm(user string) error {
	uid, err := c.fullUser(user)
	if err != nil {
		return err
	}
	room, err := c.dmRoom(uid)
	if err != nil {
		return err
	}
	name := c.r.register(room, "@"+localpart(uid.String()))
	c.r.emitRooms()
	c.r.emit(ui.FocusMsg(name))
	return nil
}

// verify starts a device verification with my other devices (no argument), or answers the one in progress:
// accept, confirm (the emojis match) or cancel, with its transaction ID.
func (c *cmds) verify(args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if len(args) == 0 {
		if _, err := c.ver.StartVerify(ctx); err != nil {
			return err
		}
		c.r.emit(ui.ToastMsg{Text: "verification requested: accept on your other device"})
		return nil
	}
	if len(args) != 2 {
		return errors.New("usage: :verify [accept|cancel]")
	}
	switch args[0] {
	case "accept":
		return c.ver.AcceptVerify(ctx, args[1])
	case "confirm":
		return c.ver.ConfirmVerify(ctx, args[1])
	case "cancel":
		return c.ver.CancelVerify(ctx, args[1])
	}
	return errors.New("usage: :verify [accept|cancel]")
}
