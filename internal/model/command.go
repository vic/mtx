package model

import (
	"fmt"
	"slices"
	"strings"
)

// CmdName is the closed set of command names (DESIGN D:318-319).
type CmdName string

const (
	CmdJoin       CmdName = "join"
	CmdCreate     CmdName = "create"
	CmdExplore    CmdName = "explore"
	CmdSearch     CmdName = "search"
	CmdDevices    CmdName = "devices"
	CmdGoto       CmdName = "goto"
	CmdTheme      CmdName = "theme"
	CmdPreview    CmdName = "preview"
	CmdDM         CmdName = "dm"
	CmdOlder      CmdName = "older"
	CmdVerify     CmdName = "verify"
	CmdPart       CmdName = "part"
	CmdClose      CmdName = "close"
	CmdInvite     CmdName = "invite"
	CmdKick       CmdName = "kick"
	CmdBan        CmdName = "ban"
	CmdUnban      CmdName = "unban"
	CmdOp         CmdName = "op"
	CmdTopic      CmdName = "topic"
	CmdRename     CmdName = "name"
	CmdTag        CmdName = "tag"
	CmdWhois      CmdName = "whois"
	CmdNick       CmdName = "nick"
	CmdIgnore     CmdName = "ignore"
	CmdUnignore   CmdName = "unignore"
	CmdNames      CmdName = "names"
	CmdMsg        CmdName = "msg"
	CmdMe         CmdName = "me"
	CmdOpen       CmdName = "open"
	CmdView       CmdName = "view"
	CmdUpload     CmdName = "upload"
	CmdAvatar     CmdName = "avatar"
	CmdJoinRule   CmdName = "joinrule"
	CmdHistory    CmdName = "history"
	CmdAlias      CmdName = "alias"
	CmdRoomAvatar CmdName = "roomavatar"
	CmdMute       CmdName = "mute"
	CmdSponsor    CmdName = "sponsor"
	CmdDocs       CmdName = "docs"
	CmdUnmute     CmdName = "unmute"
	CmdRecover    CmdName = "recover"
	CmdAccept     CmdName = "accept"
	CmdReject     CmdName = "reject"
	CmdInvites    CmdName = "invites"
	CmdReact      CmdName = "react"
	CmdPin        CmdName = "pin"
	CmdUnpin      CmdName = "unpin"
	CmdRead       CmdName = "read"
	CmdUnread     CmdName = "unread"
	CmdForward    CmdName = "forward"
	CmdRedact     CmdName = "redact"
	CmdQuit       CmdName = "quit"
	CmdHelp       CmdName = "help"
)

// CmdWords are the words a / line can start with (completion lists them); rm and q
// are aliases ParseCmd maps to close and quit.
var CmdWords = []string{"join", "create", "part", "rm", "close", "invite", "kick", "topic", "name", "tag", "whois", "nick", "ignore", "unignore", "names", "msg", "me", "open", "upload", "view", "avatar", "mute", "unmute", "sponsor", "docs", "recover", "accept", "reject", "invites", "pin", "unpin", "react", "redact", "read", "unread", "forward", "ban", "unban", "op", "joinrule", "history", "alias", "roomavatar", "explore", "search", "devices", "goto", "theme", "preview", "dm", "older", "verify", "help", "quit"}

// Cmd can only be built by ParseCmd.
type Cmd struct {
	name CmdName
	args []string
}

func (c Cmd) Name() CmdName  { return c.name }
func (c Cmd) Args() []string { return c.args }

// arity gives the arg count per name; raw means the last arg is the rest of
// the line verbatim. ok is false for an unknown name; n < 0 is free-form.
func arity(n CmdName) (count int, raw, ok bool) {
	switch n {
	case CmdCreate, CmdExplore:
		return -1, false, true
	case CmdVerify:
		return -1, false, true
	case CmdOlder:
		return -1, false, true
	case CmdDM:
		return 1, false, true
	case CmdPreview:
		return 1, false, true
	case CmdTheme:
		return -1, false, true
	case CmdGoto:
		return 1, true, true
	case CmdDevices:
		return -1, false, true
	case CmdSearch:
		return 1, true, true
	case CmdJoin, CmdKick:
		return 1, false, true
	case CmdInvite:
		return 2, false, true
	case CmdRename:
		return 1, true, true
	case CmdTag:
		return 1, false, true
	case CmdForward:
		return 1, false, true
	case CmdBan:
		return -1, false, true
	case CmdUnban:
		return 1, false, true
	case CmdOp:
		return 2, false, true
	case CmdTopic:
		return 1, true, true
	case CmdSponsor, CmdDocs:
		return -1, false, true
	case CmdMute, CmdUnmute, CmdRead, CmdUnread:
		return -1, false, true
	case CmdAccept, CmdReject, CmdInvites:
		return -1, false, true
	case CmdRecover:
		return 1, true, true
	case CmdAvatar:
		return 1, true, true
	case CmdJoinRule, CmdHistory, CmdAlias:
		return 1, false, true
	case CmdRoomAvatar:
		return 1, true, true
	case CmdUpload:
		return 1, true, true
	case CmdOpen, CmdView:
		return 1, false, true
	case CmdMe:
		return 1, true, true
	case CmdMsg:
		return 2, true, true
	case CmdNick:
		return 1, true, true
	case CmdIgnore, CmdUnignore:
		return 1, false, true
	case CmdWhois:
		return 1, false, true
	case CmdRedact:
		return -1, false, true
	case CmdPin, CmdUnpin:
		return 1, false, true
	case CmdReact:
		return 2, false, true
	case CmdPart, CmdClose, CmdNames, CmdQuit, CmdHelp:
		return -1, false, true
	}
	return 0, false, false
}

// ParseCmd parses a command typed as `:word` (a `/` start is accepted too). `:q` is `quit`. A raw
// last arg (/msg body, /topic text) keeps its inner whitespace.
func ParseCmd(line string) (Cmd, error) {
	if !strings.HasPrefix(line, "/") && !strings.HasPrefix(line, ":") {
		return Cmd{}, fmt.Errorf("not a command: %q", line)
	}
	word, rest := line[1:], ""
	if i := strings.IndexAny(word, " \t"); i >= 0 {
		word, rest = word[:i], word[i+1:]
	}
	switch word { // aliases
	case "q":
		word = string(CmdQuit)
	case "rm":
		word = string(CmdClose) // remove a & room: endpoint, saved file and log
	}
	name := CmdName(word)
	n, raw, ok := arity(name)
	if !ok {
		return Cmd{}, fmt.Errorf("unknown command %q", word)
	}
	var args []string
	if raw {
		for len(args) < n-1 {
			var a string
			rest = strings.TrimLeft(rest, " \t")
			i := strings.IndexAny(rest, " \t")
			if i < 0 {
				i = len(rest)
			}
			a, rest = rest[:i], rest[i:]
			args = append(args, a)
		}
		if rest = strings.TrimLeft(rest, " \t"); strings.TrimSpace(rest) != "" {
			args = append(args, rest)
		}
	} else {
		args = strings.Fields(rest)
	}
	if n >= 0 && (len(args) != n || slices.Contains(args, "")) {
		return Cmd{}, fmt.Errorf("usage: %s", usage[name])
	}
	return Cmd{name: name, args: args}, nil
}

// usage is shown when a command gets the wrong arguments (vic 2026-10-23).
var usage = map[CmdName]string{
	CmdJoin:       ":join &room",
	CmdCreate:     ":create &room [public]",
	CmdExplore:    ":explore [term]",
	CmdSearch:     ":search <term>",
	CmdDevices:    ":devices",
	CmdGoto:       ":goto <date | date time | HH:MM | yesterday>",
	CmdTheme:      ":theme [scheme id]",
	CmdPreview:    ":preview (the selected line's link)",
	CmdDM:         ":dm <user>",
	CmdOlder:      ":older",
	CmdVerify:     ":verify [accept|cancel]",
	CmdPart:       ":part [&room]",
	CmdClose:      ":close [&room] (or /rm)",
	CmdInvite:     ":invite <nick> &room",
	CmdKick:       ":kick <nick>",
	CmdBan:        ":ban <user> [reason]",
	CmdUnban:      ":unban <user>",
	CmdOp:         ":op <user> <level>",
	CmdTopic:      ":topic <text>",
	CmdRename:     ":name <text>",
	CmdTag:        ":tag fav|low|none",
	CmdWhois:      ":whois <nick>",
	CmdNick:       ":nick <display name>",
	CmdIgnore:     ":ignore <user>",
	CmdUnignore:   ":unignore <user>",
	CmdNames:      ":names [&room]",
	CmdMsg:        ":msg <nick> <text>",
	CmdMe:         ":me <action>",
	CmdOpen:       ":open (the selected line's media)",
	CmdView:       ":view (the selected line's image, in a popup)",
	CmdUpload:     ":upload <path>",
	CmdAvatar:     ":avatar <image path>",
	CmdJoinRule:   ":joinrule public|invite|knock",
	CmdHistory:    ":history shared|invited|joined|world",
	CmdAlias:      ":alias <#alias:server>",
	CmdRoomAvatar: ":roomavatar <image path>",
	CmdMute:       ":mute [room]",
	CmdSponsor:    ":sponsor",
	CmdDocs:       ":docs",
	CmdUnmute:     ":unmute [room]",
	CmdRead:       ":read [room]",
	CmdUnread:     ":unread [room]",
	CmdForward:    ":forward <chat> (the selected line)",
	CmdRecover:    ":recover <recovery key>",
	CmdAccept:     ":accept [room]",
	CmdReject:     ":reject [room]",
	CmdInvites:    ":invites",
	CmdReact:      ":react <emoji> (on the selected line)",
	CmdPin:        ":pin (the selected line)",
	CmdUnpin:      ":unpin (the selected line)",
	CmdRedact:     ":redact [reason] (on my selected line)",
	CmdQuit:       ":quit",
	CmdHelp:       ":help",
}

// Usages lists the usage of every command whose word starts with prefix (no "/"),
// in CmdWords order; aliases show their command's usage once.
func Usages(prefix string) []string {
	var out []string
	for _, w := range CmdWords {
		if !strings.HasPrefix(w, prefix) {
			continue
		}
		u := usage[CmdName(w)]
		if w == "rm" {
			u = usage[CmdClose]
		}
		if u != "" && !slices.Contains(out, u) {
			out = append(out, u)
		}
	}
	return out
}

// Usage is the usage line of command word w (an alias gives its command's usage), "" if unknown.
func Usage(w string) string {
	if w == "rm" {
		w = string(CmdClose)
	}
	return usage[CmdName(w)]
}
