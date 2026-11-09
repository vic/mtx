package ui

import "testing"

// One step off an end wraps; a page or a jump stops at the end (tuios listnav).
func TestListStep(t *testing.T) {
	for _, c := range []struct {
		cur  int
		m    motion
		want int
	}{
		{0, moveUp, 9}, {9, moveDown, 0}, {4, moveDown, 5},
		{8, movePageDown, 9}, {1, movePageUp, 0}, {5, moveHome, 0}, {5, moveEnd, 9}, {5, moveNone, 5},
	} {
		if got := listStep(c.cur, c.m, 10, 3); got != c.want {
			t.Errorf("from %d by %d: %d, want %d", c.cur, c.m, got, c.want)
		}
	}
	if listStep(3, moveDown, 0, 3) != 0 {
		t.Error("an empty list answers 0")
	}
}

func TestListsWrapAndJump(t *testing.T) {
	a := scrollApp(t)
	a.focus = focusChans
	_, names := a.paneItems(focusChans)
	a, _ = press(t, a, "up")
	if a.fsel != len(names)-1 {
		t.Fatalf("up on the first channel wraps to the last: %d of %d", a.fsel, len(names))
	}
	a, _ = press(t, a, "home")
	if a.fsel != 0 {
		t.Fatalf("home: %d", a.fsel)
	}
}
