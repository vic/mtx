package ui

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
	a, _ = press(t, a, "space", "i", "3")
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

// TestQuickReact: r waits for a digit and lists the quick reactions; the digit reacts to the cursor line with that emoji; any
// other key just closes the row.
func TestQuickReact(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "hi", ID: "$1"}})
	a = next.(App)
	a, _ = press(t, a, "r")
	if a.pending != "r" || !strings.Contains(ansi.Strip(a.View().Content), "👍") {
		t.Fatalf("r not pending: %q", a.pending)
	}
	a, cmd := press(t, a, "1")
	cm, ok := cmd().(CmdMsg)
	if !ok || cm.Cmd.Name() != model.CmdReact || a.pending != "" {
		t.Fatalf("react: %#v open %v", cmd(), a.pending)
	}
	if args := cm.Cmd.Args(); len(args) != 2 || args[0] != "👍" || args[1] != "$1" {
		t.Fatalf("args %q", args)
	}
	a, _ = press(t, a, "r")
	a, cmd = press(t, a, "x")
	if cmd != nil || a.pending != "" {
		t.Fatalf("x should only close the row: cmd %v open %v", cmd, a.pending)
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
		if strings.Contains(r, "c  channel") {
			ri = i
		}
		if strings.Contains(r, "t  threads") {
			ti = i
		}
	}
	if a.menu != "space" || ri < 0 || ti < 0 || ri == ti {
		t.Fatalf("menu not open: %q %q", a.menu, a.suggest())
	}
	a, _ = press(t, a, "C")
	if a.menu != "admin" {
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
	a, cmd = press(t, a, "c") // opens the channel menu
	a, cmd = press(t, a, "M") // mute
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

// TestRecentPicker: ' opens the recent picker (chats most recent first); typing and enter goes there.
func TestRecentPicker(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a, _ = press(t, a, "'")
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

// TestLineVerbs: D deletes my selected message at once (the shift confirms); r1 reacts; . repeats it on another line.
func TestLineVerbs(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "me", Text: "mine", ID: "$m"}})
	a = next.(App)
	next, _ = a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "theirs", ID: "$o"}})
	a = next.(App)
	a.hit = 0
	a, cmd := press(t, a, "d")
	if cmd != nil {
		t.Fatal("d alone must do nothing")
	}
	a, cmd = press(t, a, "D")
	cm, ok := cmd().(CmdMsg)
	if !ok || cm.Cmd.Name() != model.CmdRedact || cm.Cmd.Args()[0] != "$m" {
		t.Fatalf("redact: %#v", cmd())
	}
	a.hit = 0
	a, _ = press(t, a, "r")
	a, _ = press(t, a, "1")
	a.hit = 1
	a, cmd = press(t, a, ".")
	cm, ok = cmd().(CmdMsg)
	if !ok || cm.Cmd.Name() != model.CmdReact || cm.Cmd.Args()[0] != "👍" || cm.Cmd.Args()[1] != "$o" {
		t.Fatalf("repeat: %#v", cmd())
	}
	a, cmd = press(t, a, "r")
	a, cmd = press(t, a, "r")
	if cm, ok := cmd().(CmdMsg); !ok || cm.Cmd.Name() != model.CmdReact || cm.Cmd.Args()[0] != "👍" {
		t.Fatalf("rr: %#v", cmd())
	}
}

// TestOpenPickers: space t p lists only thread labels, space p p lists only @nicks, and ctrl+o lists no threads.
func TestOpenPickers(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.threads = map[string][]model.Thread{"#a": {{Root: "$a1", Nick: "bob", Text: "start", Count: 1}}}
	a, _ = press(t, a, "ctrl+o")
	for _, n := range a.pickSource() {
		if n == threadLabel(a.threads["#a"][0]) {
			t.Fatalf("ctrl+o lists a thread: %q", n)
		}
	}
	a, _ = press(t, a, "esc")
	a, _ = press(t, a, "space")
	a, _ = press(t, a, "t")
	a, _ = press(t, a, "p")
	if !a.pick || a.pickWhat != pickThreads {
		t.Fatalf("open thread: pick %v what %v", a.pick, a.pickWhat)
	}
	for _, n := range a.pickSource() {
		if !strings.HasSuffix(n, threadLabel(a.threads["#a"][0])) { // the row is the summary columns and the label
			t.Fatalf("thread picker lists %q", n)
		}
	}
	a, _ = press(t, a, "esc")
	a, _ = press(t, a, "space")
	a, _ = press(t, a, "p")
	a, _ = press(t, a, "p")
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
			if it.do == "forward" {
				have["forward"] = true
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

// TestMsgMenu: m on a selected line opens the message menu; its keys replay the line verbs; D deletes.
func TestMsgMenu(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "me", Text: "mine", ID: "$m"}})
	a = next.(App)
	a, _ = press(t, a, "m")
	if a.menu != "" {
		t.Fatalf("m opened a menu with no selection: %q", a.menu)
	}
	a.hit = 0
	a, _ = press(t, a, "m")
	if a.menu != "message" {
		t.Fatalf("message menu %q", a.menu)
	}
	a, cmd := press(t, a, "D")
	if cm, ok := cmd().(CmdMsg); !ok || cm.Cmd.Name() != model.CmdRedact {
		t.Fatalf("D in the message menu: %#v", cmd())
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
	a, _ = press(t, a, "c")
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

// TestForward: :forward sends the selected line quoted with its sender; an unknown chat sends nothing.
func TestForward(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "hi", ID: "$1"}})
	a = next.(App)
	a.hit = 0
	next, cmd := a.runCmd(":forward #b")
	a = next.(App)
	if cmd == nil {
		t.Fatal("forward returned no command")
	}
	cmd()
	if len(log) != 1 || log[0] != (sent{"#b", "↪ bob: hi"}) {
		t.Fatalf("sent %+v", log)
	}

	next, _ = a.runCmd(":forward #zz")
	a = next.(App)
	if n := len(a.toasts); n == 0 || a.toasts[n-1].text != "forward: no chat #zz" || len(log) != 1 {
		t.Fatalf("toasts %+v sent %+v", a.toasts, log)
	}
}

// TestBrackets: ]m and [m step through my messages; ]p steps to a pinned line.
func TestBrackets(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	for _, l := range []model.Line{{Nick: "me", Text: "one", ID: "$m1"}, {Nick: "bob", Text: "two", ID: "$b"}, {Nick: "me", Text: "three", ID: "$m2"}} {
		next, _ := a.Update(LineMsg{Ch: "#a", L: l})
		a = next.(App)
	}
	a, _ = press(t, a, "]", "m")
	if a.hit != 0 {
		t.Fatalf("first ]m hit %d want 0", a.hit)
	}
	a, _ = press(t, a, "]", "m")
	if a.hit != 2 {
		t.Fatalf("second ]m hit %d want 2", a.hit)
	}
	a, _ = press(t, a, "[", "m")
	if a.hit != 0 {
		t.Fatalf("[m hit %d want 0", a.hit)
	}

	b := newApp(t, &log, nil)
	for _, l := range []model.Line{{Nick: "me", Text: "one", ID: "$m1"}, {Nick: "bob", Text: "two", ID: "$b"}} {
		next, _ := b.Update(LineMsg{Ch: "#a", L: l})
		b = next.(App)
	}
	next, _ := b.Update(PinsMsg{Ch: "#a", IDs: []string{"$b"}})
	b = next.(App)
	b, _ = press(t, b, "]", "p")
	if b.hit != 1 {
		t.Fatalf("]p hit %d want 1", b.hit)
	}
}

// TestEditHistory: an edit keeps the text it replaces; enter then h lists the versions, read only.
func TestEditHistory(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "first", ID: "$1"}})
	a = next.(App)
	next, _ = a.Update(ReplaceLineMsg{Ch: "#a", ID: "$1", Edit: true, L: model.Line{ID: "$1", Text: "second (edited)"}})
	a = next.(App)
	if got := a.edits["$1"]; len(got) != 1 || got[0] != "first" {
		t.Fatalf("edits %v", got)
	}
	a.hit = 0
	a, _ = press(t, a, "m", "h")
	if a.pickWhat != pickEdits {
		t.Fatalf("picker %v", a.pickWhat)
	}
	if got := a.pickSource(); len(got) != 2 || got[0] != "first" || got[1] != "second (edited)  (now)" {
		t.Fatalf("history %q", got)
	}
}

// TestExplorePicker: the explore list opens as a picker; enter joins the picked room.
func TestExplorePicker(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(ExploreMsg{Entries: []string{"#a:s A (3)"}, Targets: []string{"#a:s"}})
	a = next.(App)
	if !a.pick || a.pickWhat != pickExplore {
		t.Fatalf("picker open %v kind %v", a.pick, a.pickWhat)
	}
	_, cmd := press(t, a, "enter")
	cm, ok := cmd().(CmdMsg)
	if !ok || cm.Cmd.Name() != model.CmdJoin || cm.Cmd.Args()[0] != "#a:s" {
		t.Fatalf("join: %#v", cmd())
	}
}

// TestFoundPicker: a search result that is loaded is selected by enter.
func TestFoundPicker(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	for _, l := range []model.Line{{Nick: "bob", Text: "zero", ID: "$0"}, {Nick: "bob", Text: "one", ID: "$1"}} {
		next, _ := a.Update(LineMsg{Ch: "#a", L: l})
		a = next.(App)
	}
	next, _ := a.Update(SearchMsg{Entries: []string{"<bob> one"}, IDs: []string{"$1"}})
	a = next.(App)
	if !a.pick || a.pickWhat != pickFound {
		t.Fatalf("picker open %v kind %v", a.pick, a.pickWhat)
	}
	a, _ = press(t, a, "enter")
	if a.hit != 1 {
		t.Fatalf("hit %d want 1", a.hit)
	}
}

// TestInfoPicker: a ListMsg opens a read-only picker with its rows; enter closes it.
func TestInfoPicker(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(ListMsg{Title: "devices", Entries: []string{"THIS  mtx", "OLD  old phone"}})
	a = next.(App)
	if !a.pick || a.pickWhat != pickInfo {
		t.Fatalf("picker open %v kind %v", a.pick, a.pickWhat)
	}
	if got := a.pickSource(); len(got) != 2 || got[0] != "THIS  mtx" {
		t.Fatalf("rows %q", got)
	}
	a, _ = press(t, a, "enter")
	if a.pick {
		t.Fatal("enter did not close the list")
	}
}

// TestEveryPickerRenders: every picker kind and every which-key menu renders without a panic.
func TestEveryPickerRenders(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.w, a.h = 120, 40
	next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "hi", ID: "$1"}})
	a = next.(App)
	render := func(name string, f func()) {
		t.Helper()
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("%s panicked: %v", name, r)
			}
		}()
		f()
	}
	for k := pickKind(0); k <= pickLast; k++ {
		b := a
		b.pick, b.pickWhat = true, k
		render(fmt.Sprintf("picker kind %d", k), func() { _ = b.View() })
	}
	for name := range menus {
		b := a
		b.menu = name
		render("menu "+name, func() { _ = b.View() })
	}
}

// TestDMGroup: a direct chat with a room name lists under people, after the people label.
func TestDMGroup(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(RoomsMsg{"#a", "#chat-with-ana"})
	a = next.(App)
	next, _ = a.Update(TagsMsg{"#chat-with-ana": "dm"})
	a = next.(App)
	rows := a.chanRows()
	people, dm := -1, -1
	for i, r := range rows {
		if r == nickLabel+"people" {
			people = i
		}
		if r == "#chat-with-ana" {
			dm = i
		}
	}
	if people < 0 || dm < 0 || dm < people {
		t.Fatalf("rows %q", rows)
	}
}

// TestGotoDate: :goto selects the first loaded line at or after the given date or time.
func TestGotoDate(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	for _, l := range []model.Line{
		{Nick: "bob", Text: "old", ID: "$1", At: time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)},
		{Nick: "bob", Text: "mid", ID: "$2", At: time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)},
		{Nick: "bob", Text: "new", ID: "$3", At: time.Date(2026, 10, 22, 8, 0, 0, 0, time.UTC)},
	} {
		next, _ := a.Update(LineMsg{Ch: "#a", L: l})
		a = next.(App)
	}
	for _, tc := range []struct {
		cmd  string
		want int
	}{
		{":goto 2026-10-21", 1},
		{":goto yesterday", 1},
		{":goto 07:00", 2},
	} {
		b, _ := a.runCmd(tc.cmd)
		if got := b.(App).hit; got != tc.want {
			t.Errorf("%s: hit %d want %d", tc.cmd, got, tc.want)
		}
	}
	b, _ := a.runCmd(":goto nonsense")
	if n := len(b.(App).toasts); n == 0 || !b.(App).toasts[n-1].err {
		t.Errorf("nonsense: no error toast")
	}
}

// TestSeenBy: each reader's latest read sits at their message; the message shows them, and seen by lists them.
func TestSeenBy(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	for i := 1; i <= 3; i++ {
		next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "line", ID: "$" + strconv.Itoa(i)}})
		a = next.(App)
	}
	seen := func(id string) string {
		c, _ := a.chanByName("#a")
		for i := range c.Lines.Len() {
			if l := c.Lines.At(i); l.ID == id {
				return l.Seen
			}
		}
		t.Fatalf("no line %s", id)
		return ""
	}
	next, _ := a.Update(ReadMsg{Ch: "#a", ID: "$2", Nick: "ana"})
	a = next.(App)
	if seen("$2") != "ana" {
		t.Fatalf("$2 seen %q", seen("$2"))
	}
	next, _ = a.Update(ReadMsg{Ch: "#a", ID: "$3", Nick: "ana"})
	a = next.(App)
	if seen("$2") != "" || seen("$3") != "ana" {
		t.Fatalf("after ana moves: $2 %q $3 %q", seen("$2"), seen("$3"))
	}
	next, _ = a.Update(ReadMsg{Ch: "#a", ID: "$3", Nick: "bob"})
	a = next.(App)
	if seen("$3") != "ana, bob" {
		t.Fatalf("$3 seen %q", seen("$3"))
	}
	a.hit = 0
	a, _ = press(t, a, "m", "s") // the message menu: seen by is s
	if a.pickWhat != pickInfo {
		t.Fatalf("picker %v", a.pickWhat)
	}
	if got := a.pickSource(); len(got) != 2 || got[0] != "ana" || got[1] != "bob" {
		t.Fatalf("seen by %q", got)
	}
}

// TestReactsBeforeLine: a reaction summary that arrives before its message is kept and applied when the line loads.
func TestReactsBeforeLine(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(ReactsMsg{Ch: "#a", ID: "$9", Reacts: "👍1"})
	a = next.(App)
	next, _ = a.Update(HistoryLinesMsg{Ch: "#a", L: []model.Line{{Nick: "bob", Text: "hi", ID: "$9", At: time.Date(2026, 10, 22, 8, 0, 0, 0, time.UTC)}}})
	a = next.(App)
	c, _ := a.chanByName("#a")
	for i := range c.Lines.Len() {
		if l := c.Lines.At(i); l.ID == "$9" {
			if l.Reacts != "👍1" {
				t.Fatalf("reacts %q", l.Reacts)
			}
			return
		}
	}
	t.Fatal("line $9 not loaded")
}

// TestThemePicker: the theme picker previews each scheme as its row is selected; esc restores the theme
// it opened with, enter keeps and saves the selected one.
func TestThemePicker(t *testing.T) {
	t.Cleanup(func() { usePalette(ansiPalette) })
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.runCmd(":theme")
	a = next.(App)
	if !a.pick || a.pickWhat != pickThemes {
		t.Fatalf("picker open %v kind %v", a.pick, a.pickWhat)
	}
	if got := a.pickSource(); len(got) < 2 || got[0] != "terminal (default)" {
		t.Fatalf("rows %q", got[:min(len(got), 2)])
	}
	a, _ = press(t, a, "down")
	if a.themeNow == "" || a.themeNow != a.themeIDs[1] {
		t.Fatalf("preview %q want %q", a.themeNow, a.themeIDs[1])
	}
	a, _ = press(t, a, "esc")
	if a.themeNow != "" || a.pick {
		t.Fatalf("esc: theme %q picker %v", a.themeNow, a.pick)
	}

	var saved string
	a = a.WithThemeSave(func(id string) { saved = id })
	next, _ = a.runCmd(":theme")
	a = next.(App)
	a, _ = press(t, a, "down", "enter")
	if saved != a.themeIDs[1] || a.themeNow != saved {
		t.Fatalf("saved %q now %q", saved, a.themeNow)
	}
}

// normalProbes are the keys a NORMAL user can press: every printable ASCII rune, plus the named keys.
func normalProbes() []string {
	var out []string
	for c := 0x21; c <= 0x7e; c++ {
		out = append(out, string(rune(c)))
	}
	return append(out, "space", "up", "down", "enter", "tab", "ctrl+d", "ctrl+u", "ctrl+n", "ctrl+p", "ctrl+o", "ctrl+t", "ctrl+f", "ctrl+g")
}

// TestEveryNormalKeyInMenus: every NORMAL key that does something is in a which-key menu (as its key:
// item), and every continuation of a prefix key (g [ ]) is in that prefix's menu. Digits are count
// prefixes, not actions, so they are not listed. The bracket prefixes take any key (a no-op for most), so
// only their motion keys (m l p t @ c) are checked; [[ and ]] are the speaker keys listed as their own rows.
func TestEveryNormalKeyInMenus(t *testing.T) {
	hasKey := func(items []menuItem, k string) bool {
		for _, it := range items {
			if it.key == k {
				return true
			}
		}
		return false
	}
	have := map[string]bool{}
	for _, items := range menus {
		for _, it := range items {
			have[it.do] = true
		}
	}
	for _, k := range normalProbes() {
		if k == "space" || (k >= "0" && k <= "9") { // digits are count prefixes
			continue
		}
		act, _, pend := Key(Normal, "", k)
		if act.Kind == None && pend == "" {
			continue
		}
		if pend != "" {
			if _, ok := menus[pend]; !ok {
				t.Errorf("prefix %q has no menu", pend)
			}
			continue
		}
		if !have["key:"+k] {
			t.Errorf("NORMAL key %q is in no menu", k)
		}
	}
	for _, p := range menuPrefixes {
		for _, c := range normalProbes() {
			act, _, _ := Key(Normal, p, c)
			if act.Kind == None {
				continue
			}
			if p != "g" && !strings.Contains("mplt@u", c) {
				continue // a bracket takes any key; only its motions are listed
			}
			if !hasKey(menus[p], c) {
				t.Errorf("%s%s is not in the %q menu", p, c, p)
			}
		}
	}
}

// TestPreviewCommand: :preview with no argument uses the selected line's first link.
func TestPreviewCommand(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "see https://x.org", ID: "$1"}})
	a = next.(App)
	a.hit = 0
	_, cmd := a.runCmd(":preview")
	if cmd == nil {
		t.Fatal("no command")
	}
	cm, ok := cmd().(CmdMsg)
	if !ok || cm.Cmd.Name() != model.CmdPreview || cm.Cmd.Args()[0] != "https://x.org" {
		t.Fatalf("preview: %#v", cmd())
	}
}

// TestPresenceMark: a member whose presence is known gets a dot before the nick in the nick pane.
func TestPresenceMark(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(RoomsMsg{"&r"})
	next, _ = next.(App).Update(MembersMsg{Ch: "&r", Nicks: []string{"ana", "bob"}})
	a = next.(App).focusName("&r")
	next, _ = a.Update(PresenceMsg{Nick: "ana", State: "online"})
	a = next.(App)
	found := false
	rows, _ := a.nickRows()
	for _, row := range rows {
		if strings.Contains(row, "ana") && strings.Contains(row, "●") {
			found = true
		}
		if strings.Contains(row, "bob") && strings.Contains(row, "●") {
			t.Fatalf("bob has no presence but shows a dot: %q", row)
		}
	}
	if !found {
		t.Fatalf("no dot on ana: %q", rows)
	}
}

// TestMenuBackspace: backspace in a submenu goes back to its parent; in the space menu it opens the normal
// menu, where a key runs as it does in NORMAL.
func TestMenuBackspace(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a, _ = press(t, a, "space", "C")
	if a.menu != "admin" {
		t.Fatalf("menu %q", a.menu)
	}
	a, _ = press(t, a, "backspace")
	if a.menu != "space" {
		t.Fatalf("after one backspace %q", a.menu)
	}
	a, _ = press(t, a, "backspace")
	if a.menu != "normal" {
		t.Fatalf("after two backspaces %q", a.menu)
	}
	has := func(do string) bool {
		for _, it := range menus["normal"] {
			if it.do == do {
				return true
			}
		}
		return false
	}
	if !has("key:f") || !has("menu:g") {
		t.Fatal("normal menu misses f or g")
	}
	a, _ = press(t, a, "f")
	if !a.jump {
		t.Fatal("f in the normal menu did not run as NORMAL f")
	}
}

// TestMenuBoxColumns: a menu taller than maxRows goes into columns; every label is still there.
func TestMenuBoxColumns(t *testing.T) {
	var items []menuItem
	for i := range 40 {
		items = append(items, menuItem{strconv.Itoa(i % 10), "label" + strconv.Itoa(i), "key:x"})
	}
	box := menuBox("big", items, 12)
	if n := strings.Count(box, "\n") + 1; n > 12 {
		t.Fatalf("box is %d rows tall, want at most 12", n)
	}
	for i := range 40 {
		if !strings.Contains(box, "label"+strconv.Itoa(i)) {
			t.Fatalf("label%d missing", i)
		}
	}
}

// TestPickerFollowsSelection: a picker taller than its box shows the selection, not only the first rows.
func TestPickerFollowsSelection(t *testing.T) {
	ms := make([]pickMatch, 100)
	for i := range ms {
		ms[i] = pickMatch{name: fmt.Sprintf("e%03d", i), idx: i}
	}
	box := ansi.Strip(pickBox(120, 20, "pick", "", ms, 50, false, nil))
	if !strings.Contains(box, "e050") || strings.Contains(box, "e000") {
		t.Fatalf("box does not follow the selection: %q", box)
	}
}

// TestPaneFollowsSelection: moving the channel selection down scrolls the channel pane to it.
func TestPaneFollowsSelection(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.w, a.h = 120, 20
	var names []string
	for i := range 40 {
		names = append(names, fmt.Sprintf("&c%d", i))
	}
	next, _ := a.Update(RoomsMsg(names))
	a = next.(App)
	a.focus = focusChans
	a, _ = press(t, a, "down", "down", "down", "down", "down", "down", "down", "down", "down", "down",
		"down", "down", "down", "down", "down", "down", "down", "down", "down", "down",
		"down", "down", "down", "down", "down", "down", "down", "down", "down", "down")
	if a.ptop[focusChans] <= 0 {
		t.Fatalf("channel pane did not scroll: top %d", a.ptop[focusChans])
	}
	_, rows := a.paneItems(focusChans)
	if sel := rows[a.fsel]; !strings.Contains(ansi.Strip(a.screen()), sel) {
		t.Fatalf("selected %q is not on screen", sel)
	}
}

// TestSideWheelOneRow: one wheel notch over the channel pane scrolls it one row.
func TestSideWheelOneRow(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.w, a.h = 120, 20
	var names []string
	for i := range 40 {
		names = append(names, fmt.Sprintf("&c%d", i))
	}
	next, _ := a.Update(RoomsMsg(names))
	a = next.(App)
	a = a.wheel(tea.MouseWheelMsg{X: 2, Y: 5, Button: tea.MouseWheelDown}, 1)
	if a.ptop[focusChans] != 1 {
		t.Fatalf("top %d after one notch, want 1", a.ptop[focusChans])
	}
}

// TestWheelMovesPicker: a wheel notch in the theme picker moves its selection and previews that theme.
func TestWheelMovesPicker(t *testing.T) {
	t.Cleanup(func() { usePalette(ansiPalette) })
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.runCmd(":theme")
	a = next.(App)
	next = a.wheel(tea.MouseWheelMsg{X: 60, Y: 10, Button: tea.MouseWheelDown}, 1)
	a = next.(App)
	if a.psel != 1 || a.themeNow == "" || a.themeNow != a.themeIDs[1] {
		t.Fatalf("psel %d theme %q", a.psel, a.themeNow)
	}
}

// TestZen: z hides the side panes (the narrow layout at any size); z again restores them.
func TestZen(t *testing.T) {
	t.Cleanup(func() { zen = false })
	var log []sent
	a := newApp(t, &log, nil)
	a.w, a.h = 120, 30
	a, _ = press(t, a, "z")
	if wide(a.w, a.h) || strings.Contains(ansi.Strip(a.screen()), "channels") {
		t.Fatal("z did not hide the panes")
	}
	a, _ = press(t, a, "z")
	if !wide(a.w, a.h) {
		t.Fatal("z again did not restore the panes")
	}
}

// TestUserPickerOpensDM: enter on a user in the user picker runs /dm for them.
func TestUserPickerOpensDM(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(RoomsMsg{"&r"})
	next, _ = next.(App).Update(MembersMsg{Ch: "&r", Nicks: []string{"bob"}})
	a = next.(App).focusName("&r")
	a = a.openPick(pickUsers)
	a.pq.SetValue("bob")
	_, cmd := press(t, a, "enter")
	if cmd == nil {
		t.Fatal("no command")
	}
	cm, ok := cmd().(CmdMsg)
	if !ok || cm.Cmd.Name() != model.CmdDM || cm.Cmd.Args()[0] != "@bob" {
		t.Fatalf("user picker: %#v", cmd())
	}
}

// TestThreadPickerTimes: the thread picker shows each head's last-activity time; enter selects its root.
func TestThreadPickerTimes(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	at := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	a.threads = map[string][]model.Thread{"#a": {{Root: "$a1", Nick: "bob", Text: "start", Count: 1, At: at}}}
	a = a.openPick(pickThreads)
	if src := a.pickSource(); len(src) != 1 || !strings.HasPrefix(src[0], "10-21 09:00") || !strings.Contains(src[0], "💬") || !strings.Contains(src[0], "bob: start") || strings.Contains(src[0], "\x1b") {
		t.Fatalf("rows %q", src)
	}
	a, _ = press(t, a, "enter")
	if a.thread != "$a1" {
		t.Fatalf("thread %q want $a1", a.thread)
	}
}

// TestPinLoadsContext: enter on a pin that is not loaded asks for it with its neighbours, and selects it
// when its history arrives.
func TestPinLoadsContext(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "hi", ID: "$1", At: time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)}})
	a = next.(App)
	next, _ = a.Update(PinsMsg{Ch: "#a", IDs: []string{"$p"}})
	a = next.(App)
	var gotCh, gotID string
	a = a.WithContextLoad(func(ch, id string) { gotCh, gotID = ch, id })
	a = a.openPick(pickPins)
	a, _ = press(t, a, "enter")
	if gotCh != "#a" || gotID != "$p" || a.wantHit != "$p" {
		t.Fatalf("load %q %q want #a $p, wantHit %q", gotCh, gotID, a.wantHit)
	}
	next, _ = a.Update(HistoryLinesMsg{Ch: "#a", L: []model.Line{{Nick: "bob", Text: "pinned", ID: "$p", At: time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)}}})
	a = next.(App)
	if got := a.lines(a.m.Current()); a.hit < 0 || a.hit >= len(got) || got[a.hit].ID != "$p" {
		t.Fatalf("hit %d, the pinned line is not selected", a.hit)
	}
}

// TestBackwardMotionLoadsOlder: a backward motion with nothing loaded asks for an older page, and tries
// again when the page arrives.
func TestBackwardMotionLoadsOlder(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "hi", ID: "$b1", At: time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)}})
	a = next.(App)
	var asked string
	a = a.WithOlderLoad(func(ch string) { asked = ch })
	a, _ = press(t, a, "[", "m")
	if asked != "#a" || a.retryMotion != "m" {
		t.Fatalf("asked %q retry %q", asked, a.retryMotion)
	}
	next, _ = a.Update(HistoryLinesMsg{Ch: "#a", L: []model.Line{{Nick: "me", Text: "mine", ID: "$me0", At: time.Date(2026, 10, 22, 8, 0, 0, 0, time.UTC)}}})
	a = next.(App)
	if got := a.lines(a.m.Current()); a.hit < 0 || a.hit >= len(got) || got[a.hit].ID != "$me0" {
		t.Fatalf("hit %d: the older own line is not selected", a.hit)
	}
}

// TestVerifyFlow: a SAS step opens the emoji box; y answers it with confirm; done closes the box.
func TestVerifyFlow(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(VerifyMsg{Kind: "sas", Txn: "t1", Emojis: "🐶 dog"})
	a = next.(App)
	view := ansi.Strip(a.screen())
	if !strings.Contains(view, "🐶 dog") || !strings.Contains(view, "they match") {
		t.Fatalf("emoji box missing:\n%s", view)
	}
	_, cmd := press(t, a, "y")
	cm, ok := cmd().(CmdMsg)
	if !ok || cm.Cmd.Name() != model.CmdVerify || !slices.Equal(cm.Cmd.Args(), []string{"confirm", "t1"}) {
		t.Fatalf("y: %#v", cmd())
	}
	next, _ = a.Update(VerifyMsg{Kind: "done"})
	a = next.(App)
	if a.verifySAS != "" || len(a.toasts) == 0 {
		t.Fatalf("done: sas %q toasts %d", a.verifySAS, len(a.toasts))
	}
}

// TestOlderMotions: a motion at the oldest loaded line asks for one older page, retries it when the page
// arrives, and stops at the start of the room.
func TestOlderMotions(t *testing.T) {
	keys := map[string][]string{"K": {"K"}, "up": {"up"}, "k": {"k"}, "[[": {"[", "["}}
	for name, seq := range keys {
		t.Run(name, func(t *testing.T) {
			var log []sent
			a := newApp(t, &log, nil)
			a.w, a.h = 120, 30
			for i, id := range []string{"$1", "$2"} {
				next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "line " + id, ID: id, At: time.Date(2026, 10, 22, 9, i, 0, 0, time.UTC)}})
				a = next.(App)
			}
			asked := 0
			a = a.WithOlderLoad(func(string) { asked++ })
			a.hit = 0
			press1 := func(a App) App {
				if name == "k" { // scroll up until the top of the loaded lines is reached
					for range 50 {
						a, _ = press(t, a, "k")
					}
					return a
				}
				a, _ = press(t, a, seq...)
				return a
			}
			a = press1(a)
			if asked != 1 || a.retryMotion == "" {
				t.Fatalf("first press: asked %d retry %q", asked, a.retryMotion)
			}
			a = press1(a)
			if asked != 1 {
				t.Fatalf("second press asked again: %d", asked)
			}
			next, _ := a.Update(HistoryLinesMsg{Ch: "#a", L: []model.Line{
				{Nick: "ana", Text: "old1", ID: "$o1", At: time.Date(2026, 10, 22, 8, 0, 0, 0, time.UTC)},
				{Nick: "ana", Text: "old2", ID: "$o2", At: time.Date(2026, 10, 22, 8, 1, 0, 0, time.UTC)},
			}})
			a = next.(App)
			if a.retryMotion != "" && name != "k" { // a scroll retry may ask for the next page
				t.Fatalf("retry still pending after the page: %q", a.retryMotion)
			}
			if name != "k" {
				if got := a.lines(a.m.Current()); a.hit < 0 || a.hit >= len(got) || !strings.HasPrefix(got[a.hit].ID, "$o") {
					t.Fatalf("selection %d not moved onto the older page", a.hit)
				}
			}
		})
	}

	t.Run("empty page stops", func(t *testing.T) {
		var log []sent
		a := newApp(t, &log, nil)
		next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "only", ID: "$1", At: time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)}})
		a = next.(App)
		asked := 0
		a = a.WithOlderLoad(func(string) { asked++ })
		a.hit = 0
		a, _ = press(t, a, "K")
		next, _ = a.Update(HistoryLinesMsg{Ch: "#a"})
		a = next.(App)
		if a.retryMotion != "" || asked != 1 {
			t.Fatalf("empty page: retry %q asked %d", a.retryMotion, asked)
		}
		if n := len(a.toasts); n == 0 || a.toasts[n-1].text != "no more" {
			t.Fatalf("no 'no more' toast: %+v", a.toasts)
		}
	})

	t.Run("no loader toasts no more", func(t *testing.T) {
		var log []sent
		a := newApp(t, &log, nil)
		next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "only", ID: "$1", At: time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)}})
		a = next.(App)
		a.hit = 0
		a, _ = press(t, a, "K")
		if n := len(a.toasts); n == 0 || a.toasts[n-1].text != "no more" {
			t.Fatalf("no loader: %+v", a.toasts)
		}
	})
}

// TestSeenByKeys: seen by is s in the message menu (m s, also from a fresh selection); with no line selected it says so.
func TestSeenByKeys(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "hi", ID: "$1"}})
	a = next.(App)
	a, _ = press(t, a, "up")
	a, _ = press(t, a, "m", "s")
	if !a.pick || a.pickWhat != pickInfo || a.listTitle != "seen by" {
		t.Fatalf("m s: pick %v kind %v title %q", a.pick, a.pickWhat, a.listTitle)
	}
	a, _ = press(t, a, "esc")
	a, _ = press(t, a, "up", "m", "s")
	if !a.pick || a.pickWhat != pickInfo || a.listTitle != "seen by" {
		t.Fatalf("message menu s: pick %v kind %v title %q", a.pick, a.pickWhat, a.listTitle)
	}
	a, _ = press(t, a, "esc")
	a.hit = -1
	a, _ = press(t, a, "m")
	if a.menu != "" { // the message menu needs a selected line
		t.Fatalf("m with no line opened %q", a.menu)
	}
}

// TestHelpFuzzy: ? opens the help box; a typed key starts the fuzzy help picker with it; esc closes the box.
func TestHelpFuzzy(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.w, a.h = 120, 40
	a, _ = press(t, a, "?")
	if !a.help || a.pick {
		t.Fatalf("? gave help %v pick %v", a.help, a.pick)
	}
	a, _ = press(t, a, "s")
	if a.help || !a.pick || a.pickWhat != pickHelp || a.pq.Value() != "s" {
		t.Fatalf("s: help %v pick %v kind %v query %q", a.help, a.pick, a.pickWhat, a.pq.Value())
	}
	a = typeText(t, a, "een")
	if !strings.Contains(ansi.Strip(a.screen()), "seen") {
		t.Fatalf("help filter shows no seen row:\n%s", ansi.Strip(a.screen()))
	}
	a, _ = press(t, a, "esc")
	if a.pick {
		t.Fatal("esc did not close the help picker")
	}
	a, _ = press(t, a, "?")
	a, _ = press(t, a, "esc")
	if a.help || a.pick {
		t.Fatalf("esc: help %v pick %v", a.help, a.pick)
	}
	a, _ = press(t, a, "?")
	a, _ = press(t, a, "down")
	if !a.help {
		t.Fatal("down closed the help box")
	}
}

// TestImagePopup: an image shows in the popup; any key closes it. :view with no line says so.
func TestImagePopup(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.w, a.h = 80, 24
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for x := range 4 {
		for y := range 4 {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	next, _ := a.Update(ImageMsg{Img: img})
	a = next.(App)
	if a.img == "" || !strings.Contains(ansi.Strip(a.View().Content), "image") {
		t.Fatal("image popup not shown")
	}
	a, _ = press(t, a, "x")
	if a.img != "" {
		t.Fatal("a key did not close the image popup")
	}
	next, _ = a.runCmd(":view")
	a = next.(App)
	if n := len(a.toasts); n == 0 || a.toasts[n-1].text != "view: no message with an ID here" {
		t.Fatalf("no view note: %+v", a.toasts)
	}
}

// clickRow is the screen row and column of the first line whose text holds want, in the lines pane.
func clickRow(t *testing.T, a App, want string) (int, int) {
	t.Helper()
	for y, row := range strings.Split(ansi.Strip(a.screen()), "\n") {
		if x := strings.Index(row, want); x >= 0 {
			return y, lipgloss.Width(row[:x])
		}
	}
	t.Fatalf("no row with %q:\n%s", want, ansi.Strip(a.screen()))
	return 0, 0
}

// TestClickImageViews: a click on an image line runs /view for it; a click on plain text still mentions.
func TestClickImageViews(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.w, a.h = 120, 30
	next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "[image] cat.png", ID: "$img"}})
	a = next.(App)
	y, x := clickRow(t, a, "cat.png")
	_, cmd := a.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	cm, ok := cmd().(CmdMsg)
	if !ok || cm.Cmd.Name() != model.CmdView || !slices.Equal(cm.Cmd.Args(), []string{"$img"}) {
		t.Fatalf("click on the image line: %#v", cmd())
	}
}

// TestClickPlainMentions: a click on a plain line still starts a mention of its sender.
func TestClickPlainMentions(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.w, a.h = 120, 30
	next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "just words", ID: "$p"}})
	a = next.(App)
	y, x := clickRow(t, a, "just words")
	next, _ = a.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}) // first click selects
	a = next.(App)
	next, _ = a.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}) // second click replies
	a = next.(App)
	if a.mode != Insert || !strings.Contains(a.in.Value(), "@bob") {
		t.Fatalf("mode %v input %q", a.mode, a.in.Value())
	}
}

// TestHelpExact: help matches the query as a substring; "sen" is not in "seen", "seen" is.
func TestHelpExact(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.w, a.h = 120, 40
	a, _ = press(t, a, "?", "s")
	if len(a.matches()) == 0 {
		t.Fatal("setup: no rows for s")
	}
	a.pq.SetValue("eens") // "seen" has "een" but not "eens"
	if n := len(a.matches()); n != 0 {
		t.Fatalf("eens matched %d rows", n)
	}
	a.pq.SetValue("seen")
	found := false
	for _, m := range a.matches() {
		if strings.Contains(m.name, "seen") {
			found = true
		}
	}
	if !found {
		t.Fatal("seen matched no help row")
	}
}

// TestClickSelectsThenReplies: the first click makes the line current; a second click on it replies.
func TestClickSelectsThenReplies(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.w, a.h = 120, 30
	for _, l := range []model.Line{{Nick: "bob", Text: "first words", ID: "$1"}, {Nick: "ana", Text: "second words", ID: "$2"}} {
		next, _ := a.Update(LineMsg{Ch: "#a", L: l})
		a = next.(App)
	}
	y, x := clickRow(t, a, "first words")
	next, _ := a.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	a = next.(App)
	if a.mode != Normal || a.quote != nil || a.hit < 0 || a.lines(a.m.Current())[a.hit].ID != "$1" {
		t.Fatalf("first click: mode %v quote %v hit %d", a.mode, a.quote, a.hit)
	}
	next, _ = a.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	a = next.(App)
	if a.mode != Insert || a.quote == nil || !strings.Contains(a.in.Value(), "@bob") {
		t.Fatalf("second click: mode %v quote %v input %q", a.mode, a.quote, a.in.Value())
	}
}

// TestClickOtherLineMoves: a click on another line moves the selection and does not reply.
func TestClickOtherLineMoves(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.w, a.h = 120, 30
	for _, l := range []model.Line{{Nick: "bob", Text: "first words", ID: "$1"}, {Nick: "ana", Text: "second words", ID: "$2"}} {
		next, _ := a.Update(LineMsg{Ch: "#a", L: l})
		a = next.(App)
	}
	a.hit = 0
	y, x := clickRow(t, a, "second words")
	next, _ := a.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	a = next.(App)
	if a.hit != 1 || a.mode != Normal || a.quote != nil {
		t.Fatalf("other line: hit %d mode %v quote %v", a.hit, a.mode, a.quote)
	}
}

func TestAppLineStepScrollsThroughTallLine(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(tea.WindowSizeMsg{Width: 80, Height: 8}) // 4 pane rows
	a = next.(App)
	for _, s := range []string{"a", strings.Repeat("word ", 80), "z"} { // the middle line is taller than the pane
		next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "n", Text: s}})
		a = next.(App)
	}
	a, _ = press(t, a, "up", "k") // mark z, then the tall line: its bottom shows first
	if a.hit != 1 {
		t.Fatalf("hit %d, want 1", a.hit)
	}
	off := a.off()
	a, _ = press(t, a, "up") // scrolls inside the line, the mark stays
	if a.hit != 1 || a.off() != off+1 {
		t.Fatalf("hit %d off %d -> %d, want hit 1 off +1", a.hit, off, a.off())
	}
	for i := 0; i < 20 && a.hit == 1; i++ {
		a, _ = press(t, a, "up")
	}
	if a.hit != 0 {
		t.Fatalf("k never left the tall line: hit %d", a.hit)
	}
}

func TestAppOpenLineOpensTextURL(t *testing.T) {
	t.Setenv("BROWSER", "true") // openURL runs `true <url>`: no browser starts
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{ID: "$1", Nick: "n", Text: "see https://example.org/x ok"}})
	a = next.(App)
	a, _ = press(t, a, "up")
	_, cmd := press(t, a, "v")
	if cmd == nil {
		t.Fatal("v on a line with a URL returned no command")
	}
	if _, isCmd := cmd().(CmdMsg); isCmd { // the media path: /open <id>
		t.Fatal("v sent /open for a line without media")
	}
}

func TestAppClickPinTitleGoesToPin(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	a = next.(App)
	for _, l := range []model.Line{{Nick: "bob", Text: "first", ID: "$1"}, {Nick: "ann", Text: "second", ID: "$2"}} {
		next, _ = a.Update(LineMsg{Ch: "#a", L: l})
		a = next.(App)
	}
	next, _ = a.Update(PinsMsg{Ch: "#a", IDs: []string{"$1"}})
	a = next.(App)
	next, _ = a.Update(tea.MouseClickMsg{X: chanW + 5, Y: 0, Button: tea.MouseLeft})
	if got := next.(App).hit; got != 0 {
		t.Fatalf("hit %d, want 0 (the pinned line)", got)
	}
}

func TestFrameRuleTruncates(t *testing.T) {
	if got := lipgloss.Width(frameRule(strings.Repeat("x", 100), 30)); got != 30 {
		t.Fatalf("rule width %d, want 30", got)
	}
}
