package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// pickerApp is newApp (#a #b #c) at 80x24 with the channel picker open.
func pickerApp(t *testing.T) App {
	t.Helper()
	var log []sent
	a := newApp(t, &log, nil)
	a = mouse(t, a, tea.WindowSizeMsg{Width: 80, Height: 24})
	a, _ = press(t, a, "ctrl+o")
	if !a.pick || len(a.matches()) < 3 {
		t.Fatalf("picker: %v %d", a.pick, len(a.matches()))
	}
	return a
}

// pickRowY is the screen row of picker match i (the window starts at the top).
func pickRowY(a App, i int) int {
	_, y, _, _ := centeredAt(a.w, a.h, a.popBox())
	return y + pickFirstRow + i
}

func TestPickerRowClickRunsIt(t *testing.T) {
	a := pickerApp(t)
	want := a.matches()[2].name
	b, _ := press(t, a, "down", "down", "enter")
	x, _, _, _ := centeredAt(a.w, a.h, a.popBox())
	a = mouse(t, a, tea.MouseClickMsg{Button: tea.MouseLeft, X: x + 6, Y: pickRowY(a, 2)})
	if a.pick || cur(a) != want || cur(a) != cur(b) {
		t.Fatalf("click row 2: pick %v cur %s, enter on it: %s", a.pick, cur(a), cur(b))
	}
}

func TestPickerHoverTracksRow(t *testing.T) {
	a := pickerApp(t)
	x, _, _, _ := centeredAt(a.w, a.h, a.popBox())
	a = mouse(t, a, tea.MouseMotionMsg{X: x + 6, Y: pickRowY(a, 1)})
	if a.psel != 1 {
		t.Fatalf("hover row 1: psel %d", a.psel)
	}
	a = mouse(t, a, tea.MouseMotionMsg{X: x + 6, Y: 0})
	if a.psel != 1 {
		t.Fatalf("NEG hover off the rows keeps the row: psel %d", a.psel)
	}
}

func TestPressAwayDismisses(t *testing.T) {
	a := pickerApp(t)
	before := cur(a)
	a = mouse(t, a, tea.MouseClickMsg{Button: tea.MouseLeft, X: 0, Y: 0})
	if a.pick || cur(a) != before {
		t.Fatalf("NEG a press away closes and picks nothing: pick %v cur %s", a.pick, cur(a))
	}
	a = mouse(t, a, tea.WindowSizeMsg{Width: 200, Height: 80}) // room around the help panel
	a, _ = press(t, a, "?")
	a = mouse(t, a, tea.MouseClickMsg{Button: tea.MouseLeft, X: 0, Y: 0})
	if a.help {
		t.Fatal("a press away closes the help")
	}
	a = mouse(t, a, VerifyMsg{Kind: "sas", Txn: "t1", Emojis: "🐶 dog"})
	next, cmd := a.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 0, Y: 0})
	if next.(App).verifySAS == "" || cmd != nil { // esc there would run :verify cancel
		t.Fatalf("NEG a stray press never answers a verification: cmd %v", cmd)
	}
}
