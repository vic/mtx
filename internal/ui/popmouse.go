package ui

// The pointer on a centered popup follows tuios, internal/app/overlay_mouse.go
// (github.com/Gaurav-Gosain/tuios): a press away from the panel dismisses it and the row under the pointer
// is not selected; a press on a list row runs that row; the panel highlights the row under the pointer, so
// the row a click would run is always the row under the cursor, as the keyboard selection would be.
// MIT License, Copyright (c) 2025 Gaurav Gosain.

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

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

// whichKey is the open which-key menu's name, "" for none: the menu, or a prefix waiting for its second key,
// which lists the continuations. It sits at the bottom right, one key per row.
func (a App) whichKey() string {
	if a.menu == "" && slices.Contains(menuPrefixes, a.pending) {
		return a.pending
	}
	return a.menu
}

// menuPanel is the which-key panel of menu name.
func (a App) menuPanel(name string) string { return menuBox(name, menus[name], a.h-4) }

// menuBottom is the screen row the which-key panel ends on: just above the modeline.
func (a App) menuBottom() int {
	_, input, _ := a.inputLine()
	return a.h - len(inputRows(input, a.w, a.h)) - 2
}

// menuPress answers a left press while a which-key menu is up: on an item it runs the item's key, as typing
// it would; on the rest of the panel it does nothing. Away from the panel it closes the menu and the press is
// not taken (ok false), so it still acts on what is under it: the menu is a hint, not a modal panel, and a
// second click on a chat line still replies to it.
func (a App) menuPress(m tea.Mouse) (_ App, _ tea.Cmd, ok bool) {
	name := a.whichKey()
	bl := strings.Split(ansi.Strip(a.menuPanel(name)), "\n")
	bw := lipgloss.Width(bl[0])
	x0, top := a.w-bw-1, a.menuBottom()-len(bl)+1
	if m.X < x0 || m.X >= x0+bw || m.Y < top || m.Y > a.menuBottom() {
		a.menu, a.pending = "", ""
		return a, nil, false
	}
	row := bl[m.Y-top]
	for _, it := range menus[name] {
		if i := strings.Index(row, it.key+"  "+it.label); i >= 0 {
			if c := x0 + lipgloss.Width(row[:i]); m.X >= c && m.X < c+lipgloss.Width(it.key+"  "+it.label) {
				next, cmd := a.key(keyMsg(it.key))
				return next.(App), cmd, true
			}
		}
	}
	return a, nil, true
}
