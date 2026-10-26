package ui

import (
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"mtx/internal/model"
)

// TestNextThreadToggles: T toggles main thread and the last thread opened; it never walks the other heads.
func TestNextThreadToggles(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.threads = map[string][]model.Thread{"#a": {{Root: "$a1"}, {Root: "$a2"}}}
	a = a.nextThread()
	require.Equal(t, "$a1", a.thread, "first T with no thread opened: first head")
	a = a.selectThread("$a2")
	a = a.nextThread()
	require.Equal(t, "", a.thread, "T from a thread: main")
	a = a.nextThread()
	require.Equal(t, "$a2", a.thread, "T from main: the last thread opened, not the next head")
	a = a.nextThread()
	require.Equal(t, "", a.thread)
}

// TestThreadPaneOnScreen: heads show at the top of the chat column; the selected one is marked.
func TestThreadPaneOnScreen(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.threads = map[string][]model.Thread{"#a": {{Root: "$a1", Nick: "bob", Text: "start", Count: 2}}}
	a.thread = "$a1"
	s := ansi.Strip(a.screen())
	if !strings.Contains(s, "▸ bob: start (2)") {
		t.Fatalf("thread head not on screen:\n%s", s)
	}
}

// TestThreadsInPickerAndPane: Ctrl+O lists threads and Enter selects one; Tab reaches the thread
// pane and Enter there selects the head under the cursor.
func TestThreadsInPickerAndPane(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.threads = map[string][]model.Thread{"#a": {{Root: "$a1", Nick: "bob", Text: "start", Count: 2}}}
	label := threadLabel(a.threads["#a"][0])
	a = a.openPick(pickThreads)
	if !slices.Contains(a.pickSource(), label) {
		t.Fatalf("picker source has no thread: %v", a.pickSource())
	}
	a.pq.SetValue("start")
	next, _ := a.pickKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	a = next.(App)
	if a.thread != "$a1" {
		t.Fatalf("picker enter: thread %q", a.thread)
	}
	a.thread = ""
	a.focus, a.fsel = focusThreads, 0 // the heads come first; the main thread row is last
	next, _ = a.paneStep(tea.KeyPressMsg{Code: tea.KeyEnter})
	a = next.(App)
	if a.thread != "$a1" || a.focus != focusChat {
		t.Fatalf("pane enter: thread %q focus %v", a.thread, a.focus)
	}
	a.focus = focusNicks
	next, _ = a.paneStep(tea.KeyPressMsg{Code: tea.KeyTab})
	if next.(App).focus != focusThreads {
		t.Fatalf("tab from nicks: focus %v, want threads", next.(App).focus)
	}
}

// TestThreadUnreadCount: live replies not yet read show as [n new]; selecting the thread reads it.
func TestThreadUnreadCount(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.threads = map[string][]model.Thread{"#a": {{Root: "$a1", Nick: "bob", Text: "start", Count: 4, Live: 3}}}
	a.seenLive = map[string]int{"$a1": 1}
	s := ansi.Strip(a.screen())
	if !regexp.MustCompile(`2 .*start \(4\)`).MatchString(s) {
		t.Fatalf("unread not on screen:\n%s", s)
	}
	a = a.selectThread("$a1")
	if s := ansi.Strip(a.screen()); regexp.MustCompile(`2 .*start \(4\)`).MatchString(s) {
		t.Fatalf("selected thread still shows unread:\n%s", s)
	}
}

// TestCommandWithNoChannel: with no rooms, a /command typed in INSERT still runs (it is how the first
// room is made); plain text is dropped.
func TestCommandWithNoChannel(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.m = model.Model{}
	a.in.SetValue(":create &first") // commands run from the : prompt, not sendLine
	next, cmd := a.runCmd(":create &first")
	_ = next
	require.NotNil(t, cmd, "a command with no channel must still be dispatched")
	a.in.SetValue("plain")
	next, cmd = a.sendLine()
	require.Nil(t, cmd, "plain text with no channel is dropped")
	_ = next
}

// TestThreadsBoxAboveChat: with heads, a threads box shows the heads, then .main thread. last, above a
// separator and the chat; without heads there is no box.
func TestThreadsBoxAboveChat(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	s := ansi.Strip(a.screen())
	if strings.Contains(s, "main thread") {
		t.Fatalf("no threads: no box expected:\n%s", s)
	}
	a.threads = map[string][]model.Thread{"#a": {{Root: "$a1", Nick: "bob", Text: "start", Count: 2}}}
	s = ansi.Strip(a.screen())
	mainAt, headAt := strings.Index(s, "main thread"), strings.Index(s, "bob: start")
	if mainAt < 0 || headAt < 0 || headAt > mainAt {
		t.Fatalf("threads box: heads, then main thread expected:\n%s", s)
	}
}

// TestMainThreadUnread: main lines arriving while a thread is open count on the main thread row;
// opening the main thread clears them; a channel pick returns to main.
func TestMainThreadUnread(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.threads = map[string][]model.Thread{"#a": {{Root: "$a1", Nick: "bob", Text: "start", Count: 1, Live: 1}}}
	a = a.selectThread("$a1")
	next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "main msg", ID: "$m1"}})
	a = next.(App)
	require.Equal(t, 1, a.mainUnread["#a"])
	require.Contains(t, ansi.Strip(a.screen()), "1")
	a = a.selectThread("")
	next, _ = a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "again", ID: "$m2"}})
	a = next.(App)
	require.Equal(t, 0, a.mainUnread["#a"], "on main: nothing counted and nothing kept")
}

// TestChannelPickReturnsToMain: picking a channel while in a thread lands on that channel's main thread.
func TestChannelPickReturnsToMain(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.threads = map[string][]model.Thread{"#a": {{Root: "$a1"}}}
	a = a.selectThread("$a1")
	next, _ := a.Update(RoomsMsg{"#b"})
	a = next.(App).focusName("#b")
	require.Equal(t, "#b", a.Channel())
	require.Equal(t, "", a.thread)
}

// TestOwnReplyAndEditKeys: a replies to my newest message (relation only); A quotes it too; C edits
// it; others' messages are not targets.
func TestOwnReplyAndEditKeys(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.user = "vic"
	for _, l := range []model.Line{{Nick: "vic", Text: "mine", ID: "$m1"}, {Nick: "bob", Text: "theirs", ID: "$b1"}} {
		next, _ := a.Update(LineMsg{Ch: "#a", L: l})
		a = next.(App)
	}
	a, _ = press(t, a, "a")
	require.NotNil(t, a.quote)
	require.Equal(t, "$m1", a.quote.ID)
	require.False(t, a.quoteBody)

	a.mode = Normal
	a.in.Reset()
	a, _ = press(t, a, "C")
	require.NotNil(t, a.edit)
	require.Equal(t, "mine", a.in.Value())

	a = a.prepare(ReplyOwnQuoted)
	require.True(t, a.quoteBody)
	require.Equal(t, "$m1", a.quote.ID)
}

// TestReplyLastThreadKey: o replies in the last thread used; with none, nothing changes.
func TestReplyLastThreadKey(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.threads = map[string][]model.Thread{"#a": {{Root: "$a1"}}}
	a = a.prepare(ReplyThread)
	require.Equal(t, "", a.thread)
	a.lastThread = "$a1"
	a = a.prepare(ReplyThread)
	require.Equal(t, "$a1", a.thread)
}

// TestEnterReturnsToNormal: RET sends the line and leaves INSERT.
func TestEnterReturnsToNormal(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a, _ = press(t, a, "i")
	a.in.SetValue("hello")
	next, cmd := a.sendLine()
	require.NotNil(t, cmd)
	require.Equal(t, Normal, next.(App).mode)
}

// TestMainThreadRowShowsLast: the main thread row ends with its newest line.
func TestMainThreadRowShowsLast(t *testing.T) {
	r := ansi.Strip(mainThreadRow(true, 0, time.Time{}, "bob: hi", 60))
	if !strings.Contains(r, "main thread: bob: hi") {
		t.Fatalf("row %q", r)
	}
}

// TestClickThreadHeadOpensIt: a click on a threads box row opens that thread.
func TestClickThreadHeadOpensIt(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.w, a.h = 120, 30
	a.threads = map[string][]model.Thread{"#a": {{Root: "$a1", Nick: "bob", Text: "start", Count: 1}, {Root: "$a2", Nick: "ann", Text: "later", Count: 1}}}
	next, _ := a.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 40, Y: 1 + threadTopOf(3, 0, false, 0)})
	require.Equal(t, "$a1", next.(App).thread)
}

// TestThreadsFocusLightsBoxOnly: with the threads box focused, only its rows carry the lit middle border.
func TestThreadsFocusLightsBoxOnly(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.w, a.h = 120, 30
	a.threads = map[string][]model.Thread{"#a": {{Root: "$a1", Nick: "bob", Text: "start", Count: 2}, {Root: "$a2", Nick: "ann", Text: "later", Count: 1}}}
	a.focus = focusThreads
	rows := strings.Split(a.View().Content, "\n")
	lit, unlit := borderGlyph(true, "│"), borderGlyph(false, "│")
	if !strings.Contains(rows[1], lit) {
		t.Fatalf("box row not lit: %q", rows[1])
	}
	if strings.Contains(rows[6], lit) || !strings.Contains(rows[6], unlit) {
		t.Fatalf("chat row lit: %q", rows[6])
	}
}

// TestCtrlOKeepsChannelsAndNicks: ctrl+o is open chat (channels only); threads and nicks have their own pickers.
func TestCtrlOKeepsChannelsAndNicks(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.threads = map[string][]model.Thread{"#a": {{Root: "$a1", Nick: "bob", Text: "start", Count: 1, Live: 1}}}
	if src := a.pickSource(); !slices.Contains(src, "#a") || slices.ContainsFunc(src, func(s string) bool { return strings.HasPrefix(s, "@") || strings.HasPrefix(s, "thread: ") }) {
		t.Fatalf("open chat source: %v", src)
	}
	a = a.openPick(pickThreads)
	if !slices.Contains(a.pickSource(), threadLabel(a.threads["#a"][0])) {
		t.Fatalf("open thread source lacks the thread: %v", a.pickSource())
	}
	a = a.openPick(pickUsers)
	if !slices.ContainsFunc(a.pickSource(), func(s string) bool { return strings.HasPrefix(s, "@") }) {
		t.Fatalf("open user source lacks @nicks: %v", a.pickSource())
	}
}
