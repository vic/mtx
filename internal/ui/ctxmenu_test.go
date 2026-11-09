package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// ctxApp is scrollApp with the message menu opened by a right press on the newest line.
func ctxApp(t *testing.T) (App, int, int) {
	t.Helper()
	a := scrollApp(t)
	page, y := a.chatRows()
	x, _ := a.chatText()
	px, py := x+3, y+page-1
	a = mouse(t, a, tea.MouseClickMsg{Button: tea.MouseRight, X: px, Y: py})
	return a, px, py
}

// rowOf is the index of the context menu row labeled label.
func rowOf(t *testing.T, label string) int {
	t.Helper()
	for i, it := range ctxRows("message") {
		if it.label == label {
			return i
		}
	}
	t.Fatalf("no row %q", label)
	return -1
}

func TestCtxMenuOpensAtPointer(t *testing.T) {
	a, _, _ := ctxApp(t)
	if !a.ctxOn() || a.hit < 0 {
		t.Fatalf("right press on a line: ctx %v menu %q hit %d", a.ctx, a.menu, a.hit)
	}
	s := a.screen()
	if !strings.Contains(s, " message ") || !strings.Contains(s, "› ") {
		t.Fatal("the panel shows its title chip and the marker")
	}
	if v := a.View(); v.MouseMode != tea.MouseModeAllMotion {
		t.Fatal("the open menu tracks the pointer")
	}
	if n := strings.Count(strings.Join(func() []string {
		var ls []string
		for _, it := range ctxRows("message") {
			ls = append(ls, it.do)
		}
		return ls
	}(), "\n"), "key:a"); n != 1 {
		t.Fatalf("reply is bound to m and a; the menu lists it %d times", n)
	}
}

// A click on a row does what the row's key does in the open menu.
func TestCtxMenuRowRunsItsKey(t *testing.T) {
	a, _, _ := ctxApp(t)
	_, x, y, start := a.ctxLayout()
	i := rowOf(t, "reply quoted")
	a = mouse(t, a, tea.MouseClickMsg{Button: tea.MouseLeft, X: x + 3, Y: y + ctxBodyY + i - start})
	b, _, _ := ctxApp(t)
	b, _ = press(t, b, "A")
	if a.ctx != nil || a.mode != Insert || a.mode != b.mode || a.in.Value() != b.in.Value() || fmt.Sprint(a.quote) != fmt.Sprint(b.quote) {
		t.Fatalf("row vs key: ctx %v mode %v/%v input %q/%q", a.ctx, a.mode, b.mode, a.in.Value(), b.in.Value())
	}
}

func TestCtxMenuKeysAndHover(t *testing.T) {
	a, _, _ := ctxApp(t)
	a, _ = press(t, a, "down")
	if a.ctx.sel != 1 {
		t.Fatalf("down: sel %d", a.ctx.sel)
	}
	_, x, y, start := a.ctxLayout()
	a = mouse(t, a, tea.MouseMotionMsg{X: x + 2, Y: y + ctxBodyY + 4 - start})
	if a.ctx.sel != 4 {
		t.Fatalf("hover row 4: sel %d", a.ctx.sel)
	}
	a, _ = press(t, a, "up", "enter")
	if a.ctx != nil || a.menu == "message" {
		t.Fatalf("enter runs the row and closes: ctx %v menu %q", a.ctx, a.menu)
	}
}

func TestCtxMenuPressAwayCloses(t *testing.T) {
	a, _, _ := ctxApp(t)
	_, x, _, _ := a.ctxLayout()
	a = mouse(t, a, tea.MouseClickMsg{Button: tea.MouseLeft, X: max(x-2, 0), Y: 0})
	if a.ctx != nil || a.menu != "" || a.quote != nil {
		t.Fatalf("NEG a press away runs nothing: ctx %v menu %q quote %v", a.ctx, a.menu, a.quote)
	}
}
