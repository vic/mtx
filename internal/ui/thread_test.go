package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

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
