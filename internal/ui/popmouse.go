package ui

// The pointer on a centered popup follows tuios, internal/app/overlay_mouse.go
// (github.com/Gaurav-Gosain/tuios): a press away from the panel dismisses it and the row under the pointer
// is not selected; a press on a list row runs that row; the panel highlights the row under the pointer, so
// the row a click would run is always the row under the cursor, as the keyboard selection would be.
// MIT License, Copyright (c) 2025 Gaurav Gosain.

import tea "charm.land/bubbletea/v2"

// popPress answers a left press while a centered popup is up.
func (a App) popPress(m tea.Mouse) (tea.Model, tea.Cmd) {
	x, y, bw, bh := centeredAt(a.w, a.h, a.popBox())
	switch {
	case m.X < x || m.X >= x+bw || m.Y < y || m.Y >= y+bh:
		if a.verifySAS != "" { // a verification is an answer: a stray press never gives one
			return a, nil
		}
		return a.key(keyMsg("esc"))
	case a.help:
		return a.helpClick(m)
	}
	if i, ok := a.pickRowAt(m.Y - y); ok {
		a.psel = i
		return a.key(keyMsg("enter"))
	}
	return a, nil
}

// popHover highlights the picker row under the pointer.
func (a App) popHover(m tea.Mouse) App {
	_, y, _, _ := centeredAt(a.w, a.h, a.popBox())
	if i, ok := a.pickRowAt(m.Y - y); ok {
		a.psel = i
		return a.pickHit()
	}
	return a
}

// pickRowAt is the match on panel row r of an open picker, if a match is drawn there.
func (a App) pickRowAt(r int) (int, bool) {
	if !a.pick || a.pickWhat == pickGrep {
		return 0, false
	}
	start, end := pickRows(len(a.matches()), a.psel, a.h)
	first := pickFirstRow
	if start > 0 { // the ↑ more row
		first++
	}
	i := start + r - first
	return i, i >= start && i < end
}
