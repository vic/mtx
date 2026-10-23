package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"mtx/internal/model"
)

// TestNextThreadCycles: T steps main, head 1, head 2, main.
func TestNextThreadCycles(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.threads = map[string][]model.Thread{"#a": {{Root: "$a1"}, {Root: "$a2"}}}
	a = a.nextThread()
	require.Equal(t, "$a1", a.thread)
	a = a.nextThread()
	require.Equal(t, "$a2", a.thread)
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
	a.focus, a.fsel = focusThreads, 0
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
