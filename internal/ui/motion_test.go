package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// clockApp is scrollApp with motion on and a clock the test moves.
func clockApp(t *testing.T) (App, *time.Time) {
	t.Helper()
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	a := scrollApp(t).WithMotion()
	a.now = func() time.Time { return now }
	return a, &now
}

func TestPopupSlidesIn(t *testing.T) {
	a, now := clockApp(t)
	a, cmd := press(t, a, "?")
	if a.popDY() != popRise || cmd == nil || !a.moving {
		t.Fatalf("opening: dy %d moving %v", a.popDY(), a.moving)
	}
	start := a.screen()
	*now = now.Add(popDur)
	a = mouse(t, a, frameMsg{})
	if a.popDY() != 0 || a.pop != nil || a.moving || a.screen() == start {
		t.Fatalf("landed: dy %d pop %v moving %v", a.popDY(), a.pop, a.moving)
	}
	a = mouse(t, a, frameMsg{})
	if a.moving {
		t.Fatal("NEG nothing moves: no frame tick")
	}
}

func TestChatJumpGlides(t *testing.T) {
	a, now := clockApp(t)
	a = a.setOff("#a", 6)
	a, _ = press(t, a, "G")
	if a.off() != 0 || a.shownOff() != 6 {
		t.Fatalf("G: off %d shown %d", a.off(), a.shownOff())
	}
	*now = now.Add(60 * time.Millisecond)
	if s := a.shownOff(); s <= 0 || s >= 6 {
		t.Fatalf("on the way: shown %d", s)
	}
	*now = now.Add(glideDur)
	a = mouse(t, a, frameMsg{})
	if a.glide != nil || a.shownOff() != 0 {
		t.Fatalf("landed: shown %d", a.shownOff())
	}
}

func TestDirectMovesDoNotGlide(t *testing.T) {
	a, _ := clockApp(t)
	a, _ = press(t, a, "k")
	a = mouse(t, a, tea.MouseWheelMsg{Button: tea.MouseWheelUp, X: 40, Y: 5})
	if a.glide != nil || a.shownOff() != a.off() {
		t.Fatalf("NEG a line step and a wheel notch move at once: glide %v", a.glide)
	}
	c, _ := clockApp(t)
	c = c.setOff("#a", 6)
	_, x, y := c.chatBar()
	c = mouse(t, c, tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y + 5}) // a scrollbar track press jumps
	if c.off() >= 6 || c.glide != nil {
		t.Fatalf("NEG a pointer jump moves at once: off %d glide %v", c.off(), c.glide)
	}
	b := scrollApp(t).setOff("#a", 6)
	b, _ = press(t, b, "G")
	if b.glide != nil || b.pop != nil {
		t.Fatal("NEG without motion nothing springs")
	}
}
