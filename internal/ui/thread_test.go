package ui

import (
	"regexp"
	"slices"
	"strings"
	"testing"

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
	if !slices.Contains(a.pickSource(), label) {
		t.Fatalf("picker source has no thread: %v", a.pickSource())
	}
	a = a.openPick(pickChans)
	a.pq.SetValue("start")
	next, _ := a.pickKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	a = next.(App)
	if a.thread != "$a1" {
		t.Fatalf("picker enter: thread %q", a.thread)
	}
	a.thread = ""
	a.focus, a.fsel = focusThreads, 1 // 0 is the main thread row
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
	if !regexp.MustCompile(`start \(4\).*2`).MatchString(s) {
		t.Fatalf("unread not on screen:\n%s", s)
	}
	a = a.selectThread("$a1")
	if s := ansi.Strip(a.screen()); regexp.MustCompile(`start \(4\).*2`).MatchString(s) {
		t.Fatalf("selected thread still shows unread:\n%s", s)
	}
}

// TestCtrlOKeepsChannelsAndNicks: the Ctrl+O list still has channels and @nicks next to threads.
func TestCtrlOKeepsChannelsAndNicks(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.threads = map[string][]model.Thread{"#a": {{Root: "$a1", Nick: "bob", Text: "start", Count: 1, Live: 1}}}
	src := a.pickSource()
	for _, want := range []string{"#a", threadLabel(a.threads["#a"][0])} {
		if !slices.Contains(src, want) {
			t.Fatalf("ctrl+o source lacks %q: %v", want, src)
		}
	}
	if !slices.ContainsFunc(src, func(s string) bool { return strings.HasPrefix(s, "@") }) {
		t.Fatalf("ctrl+o source lacks @nicks: %v", src)
	}
}

// TestCommandWithNoChannel: with no rooms, a /command typed in INSERT still runs (it is how the first
// room is made); plain text is dropped.
func TestCommandWithNoChannel(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.m = model.Model{}
	a.in.SetValue("/create &first")
	next, cmd := a.sendLine()
	_ = next
	require.NotNil(t, cmd, "a command with no channel must still be dispatched")
	a.in.SetValue("plain")
	next, cmd = a.sendLine()
	require.Nil(t, cmd, "plain text with no channel is dropped")
	_ = next
}

// TestReplyLastKey: o quotes the newest message that has an ID and enters INSERT.
func TestReplyLastKey(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: "one", ID: "$1"}})
	a = next.(App)
	a, _ = press(t, a, "o")
	if a.mode != Insert || a.quote == nil || a.quote.ID != "$1" {
		t.Fatalf("o: mode %v quote %+v", a.mode, a.quote)
	}
}

// TestThreadsBoxAboveChat: with heads, a threads box shows 'main thread' then the heads, above a
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
	if mainAt < 0 || headAt < 0 || mainAt > headAt {
		t.Fatalf("threads box: main thread then head expected:\n%s", s)
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
