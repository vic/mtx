package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// barColumn is the text of the chat scrollbar's column, one rune per chat row.
func barColumn(a App) string {
	g, x, y := a.chatBar()
	rows := strings.Split(a.screen(), "\n")
	var b strings.Builder
	for r := y; r < y+g.page; r++ {
		b.WriteString(ansi.Strip(ansi.Cut(rows[r], x, x+1)))
	}
	return b.String()
}

func mouse(t *testing.T, a App, m tea.Msg) App {
	t.Helper()
	next, _ := a.Update(m)
	return next.(App)
}

func TestScrollbarOnlyWhenScrolledBack(t *testing.T) {
	a := scrollApp(t) // 12 rows, page 6
	if got := barColumn(a); got != strings.Repeat("│", 6) {
		t.Fatalf("NEG at the tail the border has no thumb: %q", got)
	}
	a = a.setOff("#a", 6) // the oldest page: the thumb fills the top half
	if got := barColumn(a); got != "███│││" {
		t.Fatalf("oldest page: %q", got)
	}
	a = a.setOff("#a", 3)
	if got := barColumn(a); got != "│▄██▀│" {
		t.Fatalf("half way: %q", got)
	}
}

func TestScrollbarDrag(t *testing.T) {
	a := scrollApp(t).setOff("#a", 3)
	_, x, y := a.chatBar()
	a = mouse(t, a, tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y + 2}) // on the thumb
	if d, ok := a.drag.(barDrag); !ok || d.hold != 1 {
		t.Fatalf("press on the thumb row 1 below its top: drag %v", a.drag)
	}
	a = mouse(t, a, tea.MouseMotionMsg{Button: tea.MouseLeft, X: x, Y: y})
	if a.off() != 6 {
		t.Fatalf("drag to the top: off %d", a.off())
	}
	a = mouse(t, a, tea.MouseMotionMsg{Button: tea.MouseLeft, X: x, Y: y + 9})
	if a.off() != 0 {
		t.Fatalf("drag past the bottom: off %d", a.off())
	}
	a = mouse(t, a, tea.MouseReleaseMsg{Button: tea.MouseLeft, X: x, Y: y + 9})
	a = mouse(t, a, tea.MouseMotionMsg{Button: tea.MouseLeft, X: x, Y: y})
	if a.drag != nil || a.off() != 0 {
		t.Fatalf("NEG motion after release scrolls nothing: drag %v off %d", a.drag, a.off())
	}
}

func TestScrollbarTrackPressJumps(t *testing.T) {
	a := scrollApp(t).setOff("#a", 6) // thumb on rows 0-2
	_, x, y := a.chatBar()
	a = mouse(t, a, tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y + 5})
	if a.off() >= 6 || a.drag == nil {
		t.Fatalf("a press on the bare track jumps the thumb there: off %d drag %v", a.off(), a.drag)
	}
	b := scrollApp(t) // at the tail there is no bar: the press grabs the border under it
	b = mouse(t, b, tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y + 5})
	if _, ok := b.drag.(barDrag); ok {
		t.Fatal("NEG no bar, no bar drag")
	}
}
