package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// A drag over the last two chat rows copies their text on release, one line per row.
func TestSelectDragCopies(t *testing.T) {
	a := scrollApp(t) // msgA..msgL, newest at the bottom
	page, y := a.chatRows()
	x, width := a.chatText()
	a = mouse(t, a, tea.MouseClickMsg{Button: tea.MouseLeft, X: x + 1, Y: y + page - 2})
	a = mouse(t, a, tea.MouseMotionMsg{Button: tea.MouseLeft, X: x + width + 5, Y: y + page + 3}) // past the corner: clamped
	if !strings.Contains(a.screen(), "\x1b[7m") {
		t.Fatal("the dragged range is drawn reversed")
	}
	next, cmd := a.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft})
	a = next.(App)
	if cmd == nil {
		t.Fatal("release copies")
	}
	got := strings.Split(fmt.Sprint(cmd()), "\n")
	if len(got) != 2 || !strings.HasPrefix(got[0], "<n>") || !strings.HasSuffix(got[0], "msgK") || !strings.HasSuffix(got[1], "msgL") { // no gutter, no trailing blanks
		t.Fatalf("copied %q", got)
	}
	if a.sel == nil {
		t.Fatal("the highlight stays up after the copy")
	}
	a = mouse(t, a, tea.KeyPressMsg{Code: 'j', Text: "j"})
	if a.sel != nil {
		t.Fatal("a key drops the selection")
	}
}

func TestSelectClickCopiesNothing(t *testing.T) {
	a := scrollApp(t)
	page, y := a.chatRows()
	x, _ := a.chatText()
	a = mouse(t, a, tea.MouseClickMsg{Button: tea.MouseLeft, X: x + 3, Y: y + page - 1})
	next, cmd := a.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft})
	if cmd != nil || next.(App).sel != nil {
		t.Fatalf("NEG a press that never moved is a click: cmd %v", cmd)
	}
}

// clickUp is a left press and its release at (x, y): a click on the chat text acts on the release.
func clickUp(a App, x, y int) (tea.Model, tea.Cmd) {
	next, _ := a.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	return next.(App).Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
}
