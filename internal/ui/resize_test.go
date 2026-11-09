package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// restoreWidths puts the side pane widths back after a test that drags a border.
func restoreWidths(t *testing.T) {
	c, n := chanW, nickW
	t.Cleanup(func() { chanW, nickW = c, n; clearRenderCaches() })
}

func TestDragChanBorder(t *testing.T) {
	restoreWidths(t)
	a := scrollApp(t)
	tw := textWidth(a.w, a.h)
	_ = a.screen() // fill the render caches at the old widths
	a = mouse(t, a, tea.MouseClickMsg{Button: tea.MouseLeft, X: chanW + 1, Y: 2})
	a = mouse(t, a, tea.MouseMotionMsg{Button: tea.MouseLeft, X: chanW + 4, Y: 2})
	if chanW != 18 || textWidth(a.w, a.h) != tw-3 {
		t.Fatalf("drag 3 right: chanW %d text %d (was %d)", chanW, textWidth(a.w, a.h), tw)
	}
	for i, r := range strings.Split(a.screen(), "\n") {
		if ansi.StringWidth(r) != a.w {
			t.Fatalf("row %d is %d wide, not %d: a cell cut to the old width", i, ansi.StringWidth(r), a.w)
		}
	}
	a = mouse(t, a, tea.MouseMotionMsg{Button: tea.MouseLeft, X: 0, Y: 2})
	if chanW != minPaneW {
		t.Fatalf("drag past the left edge: chanW %d", chanW)
	}
	a = mouse(t, a, tea.MouseMotionMsg{Button: tea.MouseLeft, X: a.w, Y: 2})
	if textWidth(a.w, a.h) < nickCol-1 {
		t.Fatalf("the chat keeps a nick column: text %d", textWidth(a.w, a.h))
	}
	w := chanW
	a = mouse(t, a, tea.MouseReleaseMsg{Button: tea.MouseLeft})
	mouse(t, a, tea.MouseMotionMsg{Button: tea.MouseLeft, X: 20, Y: 2})
	if chanW != w {
		t.Fatalf("NEG motion after release resizes nothing: %d", chanW)
	}
}

func TestDragNickBorder(t *testing.T) {
	restoreWidths(t)
	a := scrollApp(t)
	x := a.w - nickW - 2
	a = mouse(t, a, tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: 3})
	mouse(t, a, tea.MouseMotionMsg{Button: tea.MouseLeft, X: x - 4, Y: 3})
	if nickW != 14 {
		t.Fatalf("drag 4 left: nickW %d", nickW)
	}
}
