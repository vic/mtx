package ui

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"time"

	tea "charm.land/bubbletea/v2"

	"mtx/internal/model"
)

var special = map[string]tea.KeyPressMsg{
	"esc":    {Code: tea.KeyEscape},
	"enter":  {Code: tea.KeyEnter},
	"tab":    {Code: tea.KeyTab},
	"ctrl+d": {Code: 'd', Mod: tea.ModCtrl},
	"ctrl+u": {Code: 'u', Mod: tea.ModCtrl},
	"up":     {Code: tea.KeyUp},
	"down":   {Code: tea.KeyDown},
	"ctrl+n": {Code: 'n', Mod: tea.ModCtrl},
	"ctrl+p": {Code: 'p', Mod: tea.ModCtrl},
	"ctrl+o": {Code: 'o', Mod: tea.ModCtrl},
}

type sent struct{ ch, text string }

// press feeds keys to a; each key is a rune string or a name from special.
func press(t *testing.T, a App, keys ...string) (App, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, k := range keys {
		msg, ok := special[k]
		if !ok {
			msg = tea.KeyPressMsg{Code: []rune(k)[0], Text: k}
		}
		var next tea.Model
		next, cmd = a.Update(msg)
		a = next.(App)
	}
	return a, cmd
}

func typeText(t *testing.T, a App, s string) App {
	t.Helper()
	for _, r := range s {
		a, _ = press(t, a, string(r))
	}
	return a
}

func newApp(t *testing.T, log *[]sent, sendErr error) App {
	t.Helper()
	m, err := model.New(20)
	if err != nil {
		t.Fatal(err)
	}
	a := New(m, func(ch, text string) error {
		*log = append(*log, sent{ch, text})
		return sendErr
	})
	a.now = func() time.Time { return time.Date(2026, 10, 22, 12, 4, 0, 0, time.UTC) }
	next, _ := a.Update(SessionsMsg{{PID: 1, Name: "a"}, {PID: 2, Name: "b"}, {PID: 3, Name: "c"}})
	return next.(App)
}

func cur(a App) string { return a.m.Current().Name }

func TestAppChannelKeys(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	for _, c := range []struct {
		name string
		keys []string
		want string
	}{
		{"gt next", []string{"g", "t"}, "#b"},
		{"gT wraps to last", []string{"g", "T"}, "#c"},
		{"3gt", []string{"3", "g", "t"}, "#c"},
		{"gg does not switch", []string{"g", "g"}, "#a"},
		{"NEG g then x", []string{"g", "x"}, "#a"},
		{"NEG 9gt out of range", []string{"9", "g", "t"}, "#a"},
	} {
		got, _ := press(t, a, c.keys...)
		if cur(got) != c.want {
			t.Errorf("%s: current %q want %q", c.name, cur(got), c.want)
		}
	}
}

func TestAppQuit(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a, _ = press(t, a, ":")
	a = typeText(t, a, "quit")
	_, cmd := press(t, a, "enter")
	if cmd == nil || cmd() != tea.Quit() {
		t.Fatalf("want tea.Quit, got %v", cmd)
	}
}

func TestAppCommandMsgAndErrors(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a, _ = press(t, a, ":")
	a = typeText(t, a, "join &x")
	a, cmd := press(t, a, "enter")
	if cmd == nil {
		t.Fatal("no cmd")
	}
	if cm, ok := cmd().(CmdMsg); !ok || cm.Cmd.Name() != model.CmdJoin {
		t.Fatalf("got %#v", cmd())
	}
	// NEG: unknown command -> error line, no cmd, no send
	a, _ = press(t, a, "esc", ":")
	a = typeText(t, a, "bogus")
	a, cmd = press(t, a, "enter")
	if cmd != nil || len(log) != 0 {
		t.Fatalf("cmd %v log %v", cmd, log)
	}
	if ls := notes(a); len(ls) != 1 || !strings.Contains(ls[0].Text, "bogus") {
		t.Fatalf("error line: %+v", ls)
	}
	// esc cancels an open : prompt
	a, _ = press(t, a, "esc")
	a, _ = press(t, a, ":")
	a = typeText(t, a, "q")
	a, cmd = press(t, a, "esc")
	if cmd != nil || a.prompt != 0 {
		t.Fatal("esc did not cancel")
	}
}

// finish runs cmd and feeds its message back into a (sends complete here, off the Update path).
func finish(t *testing.T, a App, cmd tea.Cmd) App {
	t.Helper()
	if cmd == nil {
		t.Fatal("no cmd")
	}
	next, _ := a.Update(cmd())
	return next.(App)
}

func TestAppInsertSend(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a, _ = press(t, a, "i")
	a = typeText(t, a, "hi there")
	a, cmd := press(t, a, "enter")
	if len(log) != 0 { // Update never blocks on send
		t.Fatalf("send ran inside Update: %v", log)
	}
	a = finish(t, a, cmd)
	if len(log) != 1 || log[0] != (sent{"#a", "hi there"}) {
		t.Fatalf("log %v", log)
	}
	if ls := curLines(a); len(ls) != 1 || ls[0].Nick != "me" || ls[0].Text != "hi there" {
		t.Fatalf("echo: %+v", ls)
	}
	if a.in.Value() != "" {
		t.Fatalf("input not cleared: %q", a.in.Value())
	}
	// :quit runs from the : prompt (a /quit typed in INSERT is plain text)
	a, _ = press(t, a, ":") // RET left INSERT; commands run from the : prompt
	a = typeText(t, a, "quit")
	a, cmd = press(t, a, "enter")
	if len(log) != 1 || cmd == nil || cmd() != tea.Quit() {
		t.Fatalf("log %v cmd %v", log, cmd)
	}
	// esc returns to normal: j is a key again, not text
	a, _ = press(t, a, "esc", "j")
	if a.in.Value() != "" {
		t.Fatalf("j typed in normal mode: %q", a.in.Value())
	}
}

func TestAppSendErrorShown(t *testing.T) {
	var log []sent
	a := newApp(t, &log, errors.New("boom"))
	a, _ = press(t, a, "i")
	a = typeText(t, a, "x")
	a, cmd := press(t, a, "enter")
	a = finish(t, a, cmd)
	ls := notes(a)
	if len(ls) != 1 || !strings.Contains(ls[0].Text, "send failed") || !strings.Contains(ls[0].Text, "boom") {
		t.Fatalf("lines %+v", ls) // and no echo of the unsent text
	}
}

func TestAppCtrlCQuits(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	for _, pre := range [][]string{nil, {"i"}, {":"}} {
		b, _ := press(t, a, pre...)
		_, cmd := b.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		if cmd == nil || cmd() != tea.Quit() {
			t.Errorf("ctrl+c after %v did not quit", pre)
		}
	}
}

func TestAppUnknownChannelLineNoted(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(LineMsg{Ch: "#zz", L: model.Line{Text: "x"}})
	ls := notes(next.(App))
	if len(ls) != 1 || !strings.Contains(ls[0].Text, "#zz") {
		t.Fatalf("lines %+v", ls)
	}
}

func TestAppInfoMsgCopied(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	info := map[int]model.SessionInfo{1: {Status: "idle", PermMode: "default"}}
	next, _ := a.Update(InfoMsg(info))
	a = next.(App)
	info[1] = model.SessionInfo{Status: "idle", PermMode: "bypassPermissions"}
	if got := a.info[1].PermMode; got != "default" {
		t.Fatalf("InfoMsg aliased: %q", got)
	}
}

func TestAppSearch(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	for _, s := range []string{"alpha", "needle one", "gamma", "needle two"} {
		next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "n", Text: s}})
		a = next.(App)
	}
	a, _ = press(t, a, "/")
	a = typeText(t, a, "needle")
	a, _ = press(t, a, "enter")
	if a.hit != 1 {
		t.Fatalf("first hit %d", a.hit)
	}
	// NEG: no match leaves hit unchanged
	a, _ = press(t, a, "/")
	a = typeText(t, a, "zzz")
	a, _ = press(t, a, "enter")
	if a.hit != 1 {
		t.Fatalf("no-match moved hit to %d", a.hit)
	}
}

func TestAppMessagesAndView(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(LineMsg{Ch: "#b", L: model.Line{Nick: "bob", Text: "yo"}})
	a = next.(App)
	next, _ = a.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	a = next.(App)
	v := ansi.Strip(a.screen())
	if !strings.Contains(v, "#b"+strings.Repeat(" ", 9)+"1") || !strings.Contains(v, "NORMAL") || !strings.Contains(v, "12:04") {
		t.Fatalf("view:\n%s", v)
	}
	// NEG: line for an unknown channel is dropped without panic
	next, _ = a.Update(LineMsg{Ch: "#zz", L: model.Line{Text: "x"}})
	_ = next.(App).screen()
}

func curLines(a App) []model.Line {
	c := a.m.Current()
	return c.Lines.All()
}

// scrollApp has 12 lines in #a and a 10-row screen: page 6, max offset 6, half page 3.
func scrollApp(t *testing.T) App {
	t.Helper()
	var log []sent
	a := newApp(t, &log, nil)
	for i := 0; i < 12; i++ {
		next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "n", Text: "msg" + string(rune('A'+i))}})
		a = next.(App)
	}
	next, _ := a.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	return next.(App)
}

func TestAppScroll(t *testing.T) {
	for _, c := range []struct {
		name string
		keys []string
		want int
	}{
		{"k scrolls back one", []string{"k"}, 1},
		{"k then j", []string{"k", "k", "j"}, 1},
		{"NEG j at newest stays 0", []string{"j"}, 0},
		{"NEG k clamps at oldest page", []string{"k", "k", "k", "k", "k", "k", "k", "k"}, 6},
		{"ctrl+u half page back", []string{"ctrl+u"}, 3},
		{"ctrl+u twice clamps", []string{"ctrl+u", "ctrl+u"}, 6},
		{"ctrl+d half page forward", []string{"gg", "ctrl+d"}, 3},
		{"G to newest", []string{"gg", "G"}, 0},
	} {
		a := scrollApp(t)
		keys := []string{}
		for _, k := range c.keys {
			if k == "gg" {
				keys = append(keys, "g", "g")
			} else {
				keys = append(keys, k)
			}
		}
		a, _ = press(t, a, keys...)
		if a.off() != c.want {
			t.Errorf("%s: off %d want %d", c.name, a.off(), c.want)
		}
	}
	if a, _ := press(t, scrollApp(t), "g", "g"); a.off() != 6 {
		t.Errorf("gg: off %d want 6", a.off())
	}
}

func TestAppScrollAnchorsAndIsPerChannel(t *testing.T) {
	a := scrollApp(t)
	a, _ = press(t, a, "k", "k")
	next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Text: "new"}})
	a = next.(App)
	if a.off() != 3 { // new line keeps the view anchored
		t.Fatalf("anchored off %d want 3", a.off())
	}
	a, _ = press(t, a, "G")
	next, _ = a.Update(LineMsg{Ch: "#a", L: model.Line{Text: "newer"}})
	if next.(App).off() != 0 { // at the bottom, stay following
		t.Fatalf("followed off %d want 0", next.(App).off())
	}
	a, _ = press(t, a, "k", "g", "t")
	if a.off() != 0 {
		t.Fatalf("offset leaked to #b: %d", a.off())
	}
	a, _ = press(t, a, "g", "T")
	if a.off() != 1 {
		t.Fatalf("#a offset lost: %d", a.off())
	}
}

func TestAppViewScrolled(t *testing.T) {
	a := scrollApp(t)
	if v := a.screen(); !strings.Contains(v, "msgL") || strings.Contains(v, "msgA") {
		t.Fatalf("bottom view:\n%s", v)
	}
	a, _ = press(t, a, "g", "g")
	if v := a.screen(); !strings.Contains(v, "msgA") || strings.Contains(v, "msgL") {
		t.Fatalf("top view:\n%s", v)
	}
}

func TestAppSearchHitDisplayed(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	for _, s := range []string{"alpha", "needle one", "gamma"} {
		next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "n", Text: s}})
		a = next.(App)
	}
	hasRev := func() bool {
		for _, r := range strings.Split(a.screen(), "\n") {
			if strings.Contains(ansi.Strip(r), "needle one") {
				return strings.Contains(r, hitNick("n"))
			}
		}
		t.Fatal("line not on screen")
		return false
	}
	if hasRev() {
		t.Fatal("reversed before any search")
	}
	a, _ = press(t, a, "/")
	a = typeText(t, a, "needle")
	a, _ = press(t, a, "enter")
	if !hasRev() {
		t.Fatal("hit line not highlighted")
	}
}

func TestAppNoteMsgGoesToFocused(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a, _ = press(t, a, "g", "t") // focus #b
	next, _ := a.Update(NoteMsg{L: model.Line{Text: "hello", Dim: true}})
	a = next.(App)
	if ls := notes(a); len(ls) != 1 || ls[0].Text != "hello" {
		t.Fatalf("focused lines %+v", ls)
	}
	for _, c := range a.m.Channels() {
		if c.Name != "#b" && c.Lines.Len() != 0 {
			t.Fatalf("%s got a line", c.Name)
		}
	}
	// NEG: no channels at all -> nothing to land on, no panic
	empty, err := model.New(3)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = New(empty, nil).Update(NoteMsg{L: model.Line{Text: "x"}})
}

func TestAppLineMsgEmptyChIsUnknown(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(LineMsg{L: model.Line{Text: "x"}}) // no sentinel: "" is an unknown channel
	ls := notes(next.(App))
	if len(ls) != 1 || !strings.Contains(ls[0].Text, "unknown channel") || ls[0].Text == "x" {
		t.Fatalf("lines %+v", ls)
	}
}

func TestAppNoteMsgAnchorsWhenScrolled(t *testing.T) {
	a := scrollApp(t)
	a, _ = press(t, a, "k", "k")
	next, _ := a.Update(NoteMsg{L: model.Line{Text: "n"}})
	if got := next.(App).off(); got != 2 { // a note is a toast now: the scrollback and its offset stay
		t.Fatalf("off %d want 2", got)
	}
}

func TestAppScrollCountsWrappedRows(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(tea.WindowSizeMsg{Width: 80, Height: 8}) // 4 pane rows
	a = next.(App)
	for _, s := range []string{"a", strings.Repeat("word ", 40), "z"} { // the middle line wraps to >= 3 rows
		next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "n", Text: s}})
		a = next.(App)
	}
	a, _ = press(t, a, "g", "g")
	if a.off() < 2 { // 3 lines would give max 0; wrapped rows give more
		t.Fatalf("gg off %d: wrapped rows not counted", a.off())
	}
	before := a.off()
	next, _ = a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "n", Text: strings.Repeat("word ", 40)}})
	if got := next.(App).off(); got < before+2 { // anchored by the new line's wrapped rows
		t.Fatalf("anchor %d -> %d", before, got)
	}
}

func TestAppCmdMsgCallsHandlerWithFocusedChannel(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	var gotCh string
	var gotName model.CmdName
	a = a.WithCmd(func(ch string, c model.Cmd) tea.Cmd {
		gotCh, gotName = ch, c.Name()
		return func() tea.Msg { return NoteMsg{L: model.Line{Text: "done"}} }
	})
	a, _ = press(t, a, "g", "t") // focus #b
	c, err := model.ParseCmd("/names")
	if err != nil {
		t.Fatal(err)
	}
	next, cmd := a.Update(CmdMsg{c})
	a = next.(App)
	if gotCh != "#b" || gotName != model.CmdNames || cmd == nil {
		t.Fatalf("handler got ch=%q name=%q cmd=%v", gotCh, gotName, cmd)
	}
	if n, ok := cmd().(NoteMsg); !ok || n.L.Text != "done" {
		t.Fatalf("cmd result %#v", cmd())
	}
	// NEG: no handler installed -> a visible note, no panic, no cmd
	b := newApp(t, &log, nil)
	next, cmd = b.Update(CmdMsg{c})
	b = next.(App)
	if cmd != nil || len(notes(b)) != 1 {
		t.Fatalf("no-handler: cmd=%v lines=%+v", cmd, notes(b))
	}
}

func TestAppRoomsMsgSetsRoomChannels(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(RoomsMsg{"&x", "&y"})
	a = next.(App)
	var got []string
	for _, c := range a.m.Channels() {
		if c.Kind == model.RoomChan {
			got = append(got, c.Name)
		}
	}
	if len(got) != 2 || got[0] != "&x" || got[1] != "&y" {
		t.Fatalf("rooms %v", got)
	}
	next, _ = a.Update(RoomsMsg{})
	for _, c := range next.(App).m.Channels() {
		if c.Kind == model.RoomChan {
			t.Fatalf("room %s survived empty RoomsMsg", c.Name)
		}
	}
}

func TestAppNoLocalEchoForRooms(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(RoomsMsg{"&r"})
	next, _ = next.(App).Update(SendResultMsg{Ch: "&r", Text: "x"})
	next, _ = next.(App).Update(SendResultMsg{Ch: "#a", Text: "y"}) // control: sessions still echo
	for _, c := range next.(App).m.Channels() {
		n := len(c.Lines.All())
		if (c.Name == "&r" && n != 0) || (c.Name == "#a" && n != 1) {
			t.Fatalf("%s has %d lines", c.Name, n)
		}
	}
}

func TestAppPasteIsText(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(tea.PasteMsg{Content: "gt"}) // NORMAL: paste is not keys
	if a = next.(App); cur(a) != "#a" {
		t.Fatalf("paste switched channel: %s", cur(a))
	}
	a, _ = press(t, a, "i")
	next, _ = a.Update(tea.PasteMsg{Content: "gt"})
	if a = next.(App); cur(a) != "#a" || a.in.Value() != "gt" {
		t.Fatalf("insert paste: %s %q", cur(a), a.in.Value())
	}
}

// notes is the toasts as lines: notes and errors show as toasts, not in the scrollback.
func notes(a App) []model.Line {
	var out []model.Line
	for _, t := range a.toasts {
		out = append(out, model.Line{Text: t.text})
	}
	return out
}

// TestChanGroups: the channel pane groups by tag with label rows, in the order favourites, rooms, people.
func TestChanGroups(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(RoomsMsg{"#r", "@ana"})
	a = next.(App)
	next, _ = a.Update(TagsMsg{"#r": "favourite"})
	a = next.(App)
	rows := a.chanRows()
	pos := func(s string) int {
		for i, r := range rows {
			if r == s {
				return i
			}
		}
		return -1
	}
	fav, fr, rooms, people, ana := pos(nickLabel+"favourites"), pos("#r"), pos(nickLabel+"rooms"), pos(nickLabel+"people"), pos("@ana")
	if !(fav >= 0 && fav < fr && fr < rooms && rooms < people && people < ana) {
		t.Fatalf("rows %q", rows)
	}
}

// TestTypingThrottled: a burst of typing sends one typing notice within typingEvery.
func TestTypingThrottled(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	var calls []string
	a = a.WithTyping(func(ch string) { calls = append(calls, ch) })
	a, _ = press(t, a, "i")
	a = typeText(t, a, "abc")
	if len(calls) != 1 {
		t.Fatalf("typing notices %v", calls)
	}
}

// TestRecoverNotInHistory: a /recover line, which carries the recovery key, is not kept as input history.
func TestRecoverNotInHistory(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	var saved []string
	a = a.WithInputSave(func(ch, text string) { saved = append(saved, text) })
	a, _ = press(t, a, "i")
	a = typeText(t, a, "/recover abc")
	a, _ = press(t, a, "enter")
	if len(saved) != 0 || len(a.hist[cur(a)]) != 0 {
		t.Fatalf("saved %q hist %q", saved, a.hist[cur(a)])
	}
}

// TestChanGroupsSpaces: a room tagged with a space is grouped under the space's label, before the rooms.
func TestChanGroupsSpaces(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(RoomsMsg{"#r1", "#r2"})
	a = next.(App)
	next, _ = a.Update(TagsMsg{"#r1": "space:nix"})
	a = next.(App)
	rows := a.chanRows()
	pos := func(s string) int {
		for i, r := range rows {
			if r == s {
				return i
			}
		}
		return -1
	}
	nix, r1, rooms, r2 := pos(nickLabel+"nix"), pos("#r1"), pos(nickLabel+"rooms"), pos("#r2")
	if !(nix >= 0 && nix < r1 && r1 < rooms && rooms < r2) {
		t.Fatalf("rows %q", rows)
	}
}

// TestColonTabCompletes: tab completes the command word at the : prompt, then channels after it.
func TestColonTabCompletes(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a, _ = press(t, a, ":")
	a = typeText(t, a, "jo")
	a, _ = press(t, a, "tab")
	if !strings.HasPrefix(a.in.Value(), "join") {
		t.Fatalf("command %q", a.in.Value())
	}
	a = typeText(t, a, " #")
	a, _ = press(t, a, "tab")
	if !strings.HasPrefix(a.in.Value(), "join #") {
		t.Fatalf("channel %q", a.in.Value())
	}
}

// TestEmojiComplete: INSERT tab turns a :shortcode into its emoji.
func TestEmojiComplete(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a, _ = press(t, a, "i")
	a = typeText(t, a, "hi :thu")
	a, _ = press(t, a, "tab")
	if got := a.in.Value(); got != "hi 👍" {
		t.Fatalf("value %q", got)
	}
}

// TestEmojiMenu: space e 3 inserts the third emoji and opens INSERT.
func TestEmojiMenu(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a, _ = press(t, a, "space", "e", "3")
	if a.mode != Insert || !strings.HasSuffix(a.in.Value(), "😂") {
		t.Fatalf("mode %v value %q", a.mode, a.in.Value())
	}
}

// TestColonSuggests: the : line suggests the commands that match the word typed, with what they do.
func TestColonSuggests(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a, _ = press(t, a, ":")
	a = typeText(t, a, "rea")
	s := a.suggest()
	if len(s) == 0 || !strings.Contains(ansi.Strip(strings.Join(s, "")), ":react") {
		t.Fatalf("suggest %q", s)
	}
}

// TestQuickReact: + opens the quick reaction row; a digit reacts to the cursor line with that emoji; any
// other key just closes the row.
func TestQuickReact(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "hi", ID: "$1"}})
	a = next.(App)
	a, _ = press(t, a, "+")
	if !a.reactRow || !strings.Contains(strings.Join(a.suggest(), ""), "👍") {
		t.Fatalf("row not open: %v %q", a.reactRow, a.suggest())
	}
	a, cmd := press(t, a, "1")
	cm, ok := cmd().(CmdMsg)
	if !ok || cm.Cmd.Name() != model.CmdReact || a.reactRow {
		t.Fatalf("react: %#v open %v", cmd(), a.reactRow)
	}
	if args := cm.Cmd.Args(); len(args) != 2 || args[0] != "👍" || args[1] != "$1" {
		t.Fatalf("args %q", args)
	}
	a, _ = press(t, a, "+")
	a, cmd = press(t, a, "x")
	if cmd != nil || a.reactRow {
		t.Fatalf("x should only close the row: cmd %v open %v", cmd, a.reactRow)
	}
}

// TestLeaderMenu: space opens the leader menu; its keys run an action, open the : prompt for an argument,
// or open a nested menu; any other key closes it.
func TestLeaderMenu(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a, _ = press(t, a, "space")
	rows := strings.Split(ansi.Strip(a.View().Content), "\n")
	ri, ti := -1, -1
	for i, r := range rows {
		if strings.Contains(r, "r  react") {
			ri = i
		}
		if strings.Contains(r, "t  thread") {
			ti = i
		}
	}
	if a.menu != "space" || ri < 0 || ti < 0 || ri == ti {
		t.Fatalf("menu not open: %q %q", a.menu, a.suggest())
	}
	a, _ = press(t, a, "m")
	if a.menu != "room" {
		t.Fatalf("nested menu %q", a.menu)
	}
	a, _ = press(t, a, "n")
	if a.prompt != promptCmd || a.in.Value() != "name " {
		t.Fatalf("argument prompt %q %q", string(a.prompt), a.in.Value())
	}
	a, _ = press(t, a, "esc")
	a, _ = press(t, a, "space", "z")
	if a.menu != "" || a.prompt != 0 {
		t.Fatalf("other key should close: menu %q prompt %q", a.menu, string(a.prompt))
	}
	a, cmd := press(t, a, "space") // opens the menu
	if cmd != nil {
		t.Fatal("opening the menu must not run anything")
	}
	a, cmd = press(t, a, "m") // opens the room menu
	a, cmd = press(t, a, "m") // mute
	cm, ok := cmd().(CmdMsg)
	if !ok || cm.Cmd.Name() != model.CmdMute {
		t.Fatalf("mute: %#v", cmd())
	}
}

// TestSponsorDocsCommands: :sponsor and :docs open their pages (returned as commands, not run here, so no
// browser starts) and send nothing to the command handler.
func TestSponsorDocsCommands(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	var calls []string
	a = a.WithCmd(func(ch string, c model.Cmd) tea.Cmd { calls = append(calls, string(c.Name())); return nil })
	for _, line := range []string{":sponsor", ":docs"} {
		_, cmd := a.runCmd(line)
		if cmd == nil {
			t.Fatalf("%s: no command", line)
		}
	}
	if len(calls) != 0 {
		t.Fatalf("handler got %v", calls)
	}
}

// TestRecentPicker: m opens the recent picker (chats most recent first); typing and enter goes there.
func TestRecentPicker(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a, _ = press(t, a, "m")
	if !a.pick || a.pickWhat != pickRecent {
		t.Fatalf("picker not open: pick %v what %v", a.pick, a.pickWhat)
	}
	a = typeText(t, a, "b")
	a, _ = press(t, a, "enter")
	if cur(a) != "#b" {
		t.Fatalf("current %q", cur(a))
	}
}

// TestJumpLine: f labels the visible lines; a label key selects its line; esc cancels.
// TestJumpUnread: space R j selects the line after my fully-read marker; a marker on the last line says so.
func TestJumpUnread(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.w, a.h = 120, 30
	for i := 1; i <= 3; i++ {
		next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "line " + strconv.Itoa(i), ID: "$" + strconv.Itoa(i)}})
		a = next.(App)
	}
	next, _ := a.Update(FullyReadMsg{Ch: "#a", ID: "$1"})
	a = next.(App).jumpUnread()
	if got := a.lines(a.m.Current())[a.hit].ID; got != "$2" {
		t.Fatalf("hit line %q want $2", got)
	}

	next, _ = a.Update(FullyReadMsg{Ch: "#a", ID: "$3"})
	a = next.(App).jumpUnread()
	if n := len(a.toasts); n == 0 || a.toasts[n-1].text != "no unread messages" {
		t.Fatalf("toasts %+v", a.toasts)
	}
}

// TestJumpOpen: F then a label opens that line's link (the command is returned, not run here).
func TestJumpOpen(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.w, a.h = 120, 30
	next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "see https://x.org/a", ID: "$1"}})
	a = next.(App)
	a, _ = press(t, a, "F")
	if !a.jump || !a.jumpOpen {
		t.Fatal("F did not open the labels")
	}
	if _, cmd := press(t, a, string(jumpKeys[0])); cmd == nil {
		t.Fatal("F and a label returned no command")
	}
}

// TestFirstURL: a markdown link gives its URL, a bare URL is trimmed of punctuation, no URL gives "".
func TestFirstURL(t *testing.T) {
	for in, want := range map[string]string{"a https://x.org b": "https://x.org", "[d](https://y.org)": "https://y.org", "(http://z.org/q),": "http://z.org/q", "none": ""} {
		if got := firstURL(in); got != want {
			t.Errorf("firstURL(%q) = %q want %q", in, got, want)
		}
	}
}

// TestJumpChan: gc then a label selects that channel of the pane (group labels do not take a label).
func TestJumpChan(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.w, a.h = 120, 30
	next, _ := a.Update(RoomsMsg{"#a", "#b", "#c"})
	a = next.(App)
	a, _ = press(t, a, "g", "c")
	if !a.jumpChan {
		t.Fatal("gc did not open the channel labels")
	}
	_, names := a.paneItems(focusChans)
	var rows []string
	for _, n := range names {
		if !strings.HasPrefix(n, nickLabel) {
			rows = append(rows, n)
		}
	}
	a, _ = press(t, a, string(jumpKeys[1]))
	if got := a.m.Current().Name; got != rows[1] {
		t.Fatalf("current %q want %q", got, rows[1])
	}
}

// TestJumpReply: gr on a reply selects the message it answers.
func TestJumpReply(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "first", ID: "$1"}})
	a = next.(App)
	next, _ = a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "me", Text: "answer", ID: "$2", ReplyTo: "$1"}})
	a = next.(App)
	a.hit = 1
	a, _ = press(t, a, "g", "r")
	if a.hit != 0 {
		t.Fatalf("hit %d want 0", a.hit)
	}
}

func TestJumpLine(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.w, a.h = 120, 30
	for i := range 5 {
		next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "line " + strconv.Itoa(i), ID: "$" + strconv.Itoa(i)}})
		a = next.(App)
	}
	a, _ = press(t, a, "f")
	if !a.jump {
		t.Fatal("f did not open the labels")
	}
	ts := a.jumpTargets()
	if len(ts) < 2 {
		t.Fatalf("targets %v", ts)
	}
	a, _ = press(t, a, string(jumpKeys[1]))
	if a.jump || a.hit != ts[1] {
		t.Fatalf("hit %d want %d jump %v", a.hit, ts[1], a.jump)
	}
	a, _ = press(t, a, "f")
	hit := a.hit
	a, _ = press(t, a, "esc")
	if a.jump || a.hit != hit {
		t.Fatalf("esc did not cancel: jump %v hit %d", a.jump, a.hit)
	}
}

// TestLineVerbs: d asks, then y deletes my selected message; . repeats the last line command on the selected line.
func TestLineVerbs(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "me", Text: "mine", ID: "$m"}})
	a = next.(App)
	next, _ = a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "theirs", ID: "$o"}})
	a = next.(App)
	a.hit = 0
	a, _ = press(t, a, "d")
	if !a.confirmDel {
		t.Fatal("d did not ask")
	}
	a, cmd := press(t, a, "y")
	cm, ok := cmd().(CmdMsg)
	if !ok || cm.Cmd.Name() != model.CmdRedact || cm.Cmd.Args()[0] != "$m" {
		t.Fatalf("redact: %#v", cmd())
	}
	a.hit = 0
	a, _ = press(t, a, "+")
	a, _ = press(t, a, "1")
	a.hit = 1
	a, cmd = press(t, a, ".")
	cm, ok = cmd().(CmdMsg)
	if !ok || cm.Cmd.Name() != model.CmdReact || cm.Cmd.Args()[0] != "👍" || cm.Cmd.Args()[1] != "$o" {
		t.Fatalf("repeat: %#v", cmd())
	}
	a.hit = 0
	a, _ = press(t, a, "d")
	a, cmd = press(t, a, "n")
	if cmd != nil || a.confirmDel {
		t.Fatalf("n should cancel: cmd %v confirm %v", cmd, a.confirmDel)
	}
}

// TestOpenPickers: space o t lists only thread labels, space o u lists only @nicks, and ctrl+o lists no threads.
func TestOpenPickers(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.threads = map[string][]model.Thread{"#a": {{Root: "$a1", Nick: "bob", Text: "start", Count: 1}}}
	a, _ = press(t, a, "ctrl+o")
	for _, n := range a.pickSource() {
		if strings.HasPrefix(n, "thread: ") {
			t.Fatalf("ctrl+o lists a thread: %q", n)
		}
	}
	a, _ = press(t, a, "esc")
	a, _ = press(t, a, "space")
	a, _ = press(t, a, "o")
	a, _ = press(t, a, "t")
	if !a.pick || a.pickWhat != pickThreads {
		t.Fatalf("open thread: pick %v what %v", a.pick, a.pickWhat)
	}
	for _, n := range a.pickSource() {
		if !strings.HasPrefix(n, "thread: ") {
			t.Fatalf("thread picker lists %q", n)
		}
	}
	a, _ = press(t, a, "esc")
	a, _ = press(t, a, "space")
	a, _ = press(t, a, "o")
	a, _ = press(t, a, "u")
	if !a.pick || a.pickWhat != pickUsers {
		t.Fatalf("open user: pick %v what %v", a.pick, a.pickWhat)
	}
	for _, n := range a.pickSource() {
		if !strings.HasPrefix(n, "@") {
			t.Fatalf("user picker lists %q", n)
		}
	}
}

// TestPermalink: Y copies a matrix.to link to the selected line and toasts it.
func TestPermalink(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(IDsMsg{"#a": "!r:s"})
	a = next.(App)
	next, _ = a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "hi", ID: "$e"}})
	a = next.(App)
	a, cmd := press(t, a, "Y")
	want := "https://matrix.to/#/" + url.PathEscape("!r:s") + "/" + url.PathEscape("$e")
	if cmd == nil || fmt.Sprint(cmd()) != fmt.Sprint(tea.SetClipboard(want)()) {
		t.Fatalf("clipboard: %v", cmd)
	}
	if !strings.Contains(fmt.Sprint(a.toasts), want) {
		t.Fatalf("toast %v", a.toasts)
	}
}

// TestEveryCommandInMenus: every command word has a which-key path, and every submenu a menu item opens exists.
func TestEveryCommandInMenus(t *testing.T) {
	have := map[string]bool{}
	for _, items := range menus {
		for _, it := range items {
			if w, ok := strings.CutPrefix(it.do, ":"); ok {
				have[strings.Fields(w)[0]] = true
			}
			if it.do == "react" {
				have["react"] = true
			}
			if m, ok := strings.CutPrefix(it.do, "menu:"); ok {
				if _, ok := menus[m]; !ok {
					t.Errorf("menu %q is not defined", m)
				}
			}
		}
	}
	for _, w := range model.CmdWords {
		if w == "rm" || w == "q" {
			continue
		}
		if !have[w] {
			t.Errorf("command %q has no which-key path", w)
		}
	}
}

// TestMsgMenu: enter on a selected line opens the message menu; its keys replay the line verbs.
func TestMsgMenu(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "me", Text: "mine", ID: "$m"}})
	a = next.(App)
	a, _ = press(t, a, "enter")
	if a.menu != "" {
		t.Fatalf("enter opened a menu with no selection: %q", a.menu)
	}
	a.hit = 0
	a, _ = press(t, a, "enter")
	if a.menu != "message" {
		t.Fatalf("message menu %q", a.menu)
	}
	a, _ = press(t, a, "d")
	if !a.confirmDel {
		t.Fatal("d in the message menu did not ask to delete")
	}
}

// TestPinnedPicker: space o p lists the room's pinned messages; enter goes to the chosen one.
func TestPinnedPicker(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "first", ID: "$1"}})
	a = next.(App)
	next, _ = a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "ann", Text: "second", ID: "$2"}})
	a = next.(App)
	next, _ = a.Update(PinsMsg{Ch: "#a", IDs: []string{"$2"}})
	a = next.(App)
	a, _ = press(t, a, "space")
	a, _ = press(t, a, "o")
	a, _ = press(t, a, "p")
	if !a.pick || a.pickWhat != pickPins {
		t.Fatalf("pinned picker: pick %v what %v", a.pick, a.pickWhat)
	}
	src := a.pickSource()
	if len(src) != 1 || !strings.Contains(src[0], "second") {
		t.Fatalf("pinned source %q", src)
	}
	a, _ = press(t, a, "enter")
	if ls := a.lines(a.m.Current()); a.hit < 0 || ls[a.hit].ID != "$2" {
		t.Fatalf("hit %d", a.hit)
	}
}

// TestOpenKeys: # opens the chat picker, @ the user picker, and % the thread picker.
func TestOpenKeys(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	for key, want := range map[string]pickKind{"#": pickChans, "@": pickUsers, "%": pickThreads} {
		b, _ := press(t, a, key)
		if !b.pick || b.pickWhat != want {
			t.Fatalf("%s: pick %v what %v", key, b.pick, b.pickWhat)
		}
	}
}

// TestSigilPickers: # @ % ^ open the room, user, thread and pinned pickers.
func TestSigilPickers(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	for key, want := range map[string]pickKind{"#": pickChans, "@": pickUsers, "%": pickThreads, "^": pickPins} {
		b, _ := press(t, a, key)
		if !b.pick || b.pickWhat != want {
			t.Fatalf("%s: pick %v what %v", key, b.pick, b.pickWhat)
		}
	}
}
