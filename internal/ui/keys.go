// Package ui holds the terminal UI; keys.go is the pure vim key table (DESIGN R6).
package ui

import (
	"strconv"
	"strings"
)

// EditMode is the vim mode.
type EditMode uint8

const (
	Normal EditMode = iota
	Insert
)

// ActKind names what a key press asks the UI to do.
type ActKind uint8

const (
	None ActKind = iota
	Scroll
	HalfPage
	Top
	Bottom
	NextChan
	HistPrev
	HistNext
	PrevChan
	GotoChan
	CmdLine
	ShellLine
	CaptureLine
	Send
	Complete
	Help
	ToggleMentions
	ThreadNext
	FocusPane
	NextTurn
	PrevTurn
	LineUp
	LineDown

	PickChan
	PickFile
	FileBlock
	GrepLines
	PickSpeaker
	Yank
	PickInvite
	NewLine
	ReplyOwn
	ReplyOwnQuoted
	EditOwn
	ReplyThread
	ReplyThreadQuoted
	ThreadStart

	Leader
	PickRecent
	JumpLine
	JumpOpen
	JumpChan
	Delete
	OpenLine
	Repeat
	Permalink
	MsgMenu
	PickUser
	PickThread
	PickPins
	JumpReply
	Bracket
	Zen
	React
	JumpUnread
	GotoDate
	Quit
	PickLines
	ClearLine
)

// maxCountDigits bounds the {n} in {n}gt.
const maxCountDigits = 4

// Action is a key result; N is the signed amount for Scroll/HalfPage or, for GotoChan,
// the 1-based vim {n}gt channel number (the model uses N-1).
type Action struct {
	Kind ActKind
	N    int
	Key  string // the bracket motion's target key (]m -> "m")
}

// bracketDir is the direction of a pending bracket: ] goes to newer (1), [ to older (-1).
func bracketDir(p string) int {
	if p == "]" {
		return 1
	}
	return -1
}

// binding is one NORMAL key sequence and the action Key gives it.
type binding struct {
	keys []string
	act  Action
}

// normalPreKeys are the NORMAL keys App.key takes before Key, with the action each one takes.
var normalPreKeys = map[string]Action{"/": {Kind: PickLines}, "esc": {Kind: ClearLine}}

// normalBindings are the NORMAL sequences of one key, and of a prefix key then one character.
var normalBindings = buildNormalBindings()

func buildNormalBindings() []binding {
	var out []binding
	for _, k := range normalKeys() {
		act, _, pending := Key(Normal, "", k)
		if pa, ok := normalPreKeys[k]; ok {
			act, pending = pa, ""
		}
		if pending == "" && act.Kind != None {
			out = append(out, binding{[]string{k}, act})
		}
		if pending == "" || (pending[0] >= '0' && pending[0] <= '9') { // a count is not a prefix
			continue
		}
		for _, k2 := range normalKeys() {
			if len(k2) != 1 {
				continue
			}
			if act2, _, p2 := Key(Normal, pending, k2); p2 == "" && act2.Kind != None {
				out = append(out, binding{[]string{k, k2}, act2})
			}
		}
	}
	return out
}

// bindingKeys are the NORMAL sequences that match accepts, in key order, as the hint shows them.
func bindingKeys(match func(binding) bool) []string {
	var out []string
	for _, b := range normalBindings {
		if match(b) {
			out = append(out, bindingText(b.keys))
		}
	}
	return out
}

// bindingText is a NORMAL sequence as a hint shows it: one-character keys run together ("gg"), named keys
// are space separated ("ctrl+w w").
func bindingText(keys []string) string {
	for _, k := range keys {
		if len(k) != 1 {
			return strings.Join(keys, " ")
		}
	}
	return strings.Join(keys, "")
}

// replayKeys is the action the dispatcher takes for the NORMAL sequence keys: the zero Action if it binds nothing.
func replayKeys(keys []string) Action {
	var act Action
	pending := ""
	for _, k := range keys {
		if pa, ok := normalPreKeys[k]; ok && pending == "" {
			act = pa
			continue
		}
		act, _, pending = Key(Normal, pending, k)
	}
	if pending != "" {
		return Action{}
	}
	return act
}

// normalKeys are the single keys Key can bind in NORMAL: the named keys, printable ASCII and ctrl+a to ctrl+z.
func normalKeys() []string {
	out := []string{"space", "tab", "up", "down", "esc"}
	for c := '!'; c <= '~'; c++ {
		out = append(out, string(c))
	}
	for c := 'a'; c <= 'z'; c++ {
		out = append(out, "ctrl+"+string(c))
	}
	return out
}

// Key maps key k (bubbletea KeyMsg.String() form) in mode m with the pending
// prefix ("g", "3", "3g") to an action, the next mode and the next pending prefix.
func Key(m EditMode, pending, k string) (Action, EditMode, string) {
	if m == Insert {
		switch k {
		case "esc":
			return Action{}, Normal, ""
		case "enter":
			return Action{Kind: Send}, Insert, ""
		case "ctrl+o":
			return Action{Kind: PickChan}, Insert, ""
		case "ctrl+f":
			return Action{Kind: FileBlock}, Insert, ""
		case "ctrl+t":
			return Action{Kind: PickFile}, Insert, ""
		case "ctrl+g":
			return Action{Kind: GrepLines}, Insert, ""
		case "up":
			return Action{Kind: LineUp}, Insert, ""
		case "down":
			return Action{Kind: LineDown}, Insert, ""
		case "ctrl+p":
			return Action{Kind: HistPrev}, Insert, ""
		case "ctrl+n":
			return Action{Kind: HistNext}, Insert, ""
		case "tab":
			return Action{Kind: Complete}, Insert, ""
		}
		return Action{}, Insert, ""
	}
	if pending == "" {
		switch k {
		case "space":
			return Action{Kind: Leader}, Normal, ""
		case "e", "C":
			return Action{Kind: EditOwn}, Insert, ""
		case "D":
			return Action{Kind: Delete}, Normal, ""
		case ".":
			return Action{Kind: Repeat}, Normal, ""
		case "Y":
			return Action{Kind: Permalink}, Normal, ""
		case "m":
			return Action{Kind: MsgMenu}, Normal, ""
		case "'":
			return Action{Kind: PickRecent}, Normal, ""
		case "#":
			return Action{Kind: PickChan}, Normal, ""
		case "@":
			return Action{Kind: PickUser}, Normal, ""
		case "%":
			return Action{Kind: PickThread}, Normal, ""
		case "^":
			return Action{Kind: PickPins}, Normal, ""
		case "i":
			return Action{Kind: NewLine}, Insert, ""
		case "a":
			return Action{Kind: ReplyOwn}, Insert, ""
		case "A":
			return Action{Kind: ReplyOwnQuoted}, Insert, ""
		case "o":
			return Action{Kind: ReplyThread}, Insert, ""
		case "O":
			return Action{Kind: ReplyThreadQuoted}, Insert, ""
		case "j", "ctrl+e":
			return Action{Scroll, 1, ""}, Normal, ""
		case "k", "ctrl+y":
			return Action{Scroll, -1, ""}, Normal, ""
		case "ctrl+d":
			return Action{HalfPage, 1, ""}, Normal, ""
		case "ctrl+u":
			return Action{HalfPage, -1, ""}, Normal, ""
		case "G":
			return Action{Kind: Bottom}, Normal, ""
		case "ctrl+o":
			return Action{Kind: PickChan}, Normal, ""
		case "v":
			return Action{Kind: OpenLine}, Normal, ""
		case "ctrl+f":
			return Action{Kind: FileBlock}, Normal, ""
		case "ctrl+t":
			return Action{Kind: PickFile}, Normal, ""
		case "ctrl+g":
			return Action{Kind: GrepLines}, Normal, ""
		case "S":
			return Action{Kind: PickSpeaker}, Normal, ""
		case "y":
			return Action{Kind: Yank}, Normal, ""
		case "up":
			return Action{Kind: LineUp}, Normal, ""
		case "down":
			return Action{Kind: LineDown}, Normal, ""
		case "l":
			return Action{Kind: NextChan}, Normal, ""
		case "h":
			return Action{Kind: PrevChan}, Normal, ""
		case "ctrl+n":
			return Action{Kind: Bracket, N: 1, Key: "t"}, Normal, ""
		case "ctrl+p":
			return Action{Kind: Bracket, N: -1, Key: "t"}, Normal, ""
		case "J":
			return Action{Kind: NextTurn}, Normal, ""
		case "K":
			return Action{Kind: PrevTurn}, Normal, ""
		case "tab":
			return Action{Kind: FocusPane}, Normal, ""
		case "I":
			return Action{Kind: PickInvite}, Normal, ""
		case "t":
			return Action{Kind: ThreadNext}, Normal, ""
		case "u":
			return Action{Kind: JumpUnread}, Normal, ""
		case "z":
			return Action{Kind: Zen}, Normal, ""
		case "f":
			return Action{Kind: JumpLine}, Normal, ""
		case "[", "]", "r", "Z", "c", "ctrl+w":
			return Action{}, Normal, k
		case "F":
			return Action{Kind: JumpOpen}, Normal, ""
		case "M":
			return Action{Kind: ToggleMentions}, Normal, ""
		case "T":
			return Action{Kind: ThreadStart}, Insert, ""
		case "?":
			return Action{Kind: Help}, Normal, ""
		case ":":
			return Action{Kind: CmdLine}, Normal, ""
		case "$":
			return Action{Kind: ShellLine}, Normal, ""
		case "!":
			return Action{Kind: CaptureLine}, Normal, ""
		case "g":
			return Action{}, Normal, "g"
		}
	}
	switch pending {
	case "[", "]":
		return Action{Kind: Bracket, N: bracketDir(pending), Key: k}, Normal, ""
	case "r": // r1-9 reacts with quick reaction n, rr with the first (👍)
		if n := int(k[0] - '0'); len(k) == 1 && n >= 1 && n <= 9 {
			return Action{Kind: React, N: n}, Normal, ""
		}
		if k == "r" {
			return Action{Kind: React, N: 1}, Normal, ""
		}
		return Action{}, Normal, ""
	case "Z":
		if k == "Z" {
			return Action{Kind: Quit}, Normal, ""
		}
		return Action{}, Normal, ""
	case "c":
		if k == "c" {
			return Action{Kind: EditOwn}, Insert, ""
		}
		return Action{}, Normal, ""
	case "ctrl+w":
		if k == "w" {
			return Action{Kind: FocusPane}, Normal, ""
		}
		return Action{}, Normal, ""
	}
	digits, g := strings.CutSuffix(pending, "g")
	switch {
	case len(k) == 1 && k[0] >= '0' && k[0] <= '9' && !g:
		if len(digits) >= maxCountDigits {
			return Action{}, Normal, ""
		}
		if digits != "" || k != "0" {
			return Action{}, Normal, digits + k
		}
	case k == "g" && !g && digits != "":
		return Action{}, Normal, digits + "g"
	case g && digits == "" && k == "g":
		return Action{Kind: Top}, Normal, ""
	case g && digits == "" && (k == "x" || k == "f"):
		return Action{Kind: OpenLine}, Normal, ""
	case g && digits == "" && k == "t":
		return Action{Kind: NextChan}, Normal, ""
	case g && digits == "" && k == "T":
		return Action{Kind: PrevChan}, Normal, ""
	case g && digits == "" && k == "c":
		return Action{Kind: JumpChan}, Normal, ""
	case g && digits == "" && k == "e":
		return Action{Kind: Bottom}, Normal, ""
	case g && digits == "" && k == "w":
		return Action{Kind: JumpLine}, Normal, ""
	case g && digits == "" && k == "i":
		return Action{Kind: NewLine}, Insert, ""
	case g && digits == "" && k == "d":
		return Action{Kind: GotoDate}, Normal, ""
	case g && digits == "" && k == "r":
		return Action{Kind: JumpReply}, Normal, ""
	case g && digits != "" && k == "t":
		if n, err := strconv.Atoi(digits); err == nil {
			return Action{GotoChan, n, ""}, Normal, ""
		}
	}
	return Action{}, Normal, ""
}
