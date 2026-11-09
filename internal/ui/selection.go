package ui

// Mouse selection follows tuios, internal/input/mouse_select.go (github.com/Gaurav-Gosain/tuios): copy on
// release (OSC 52, like copy mode's y), a press that never moved copies nothing, and the highlight stays up
// until the next press or key so the user sees what they got.
// MIT License, Copyright (c) 2025 Gaurav Gosain.

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// drag is the mouse gesture in progress; there is one at a time.
type drag interface{ dragging() }

// barDrag holds the scrollbar: hold is the rows between the pointer and the thumb top.
type barDrag struct{ hold int }

// selDrag moves the selection head with the pointer; at is the press, a click if the pointer never moves.
type selDrag struct{ at tea.Mouse }

func (barDrag) dragging() {}
func (selDrag) dragging() {}

// textPos is a chat text cell: back rows above the newest row, col columns into the chat cell. Rows are
// counted from the newest, so the selection stays on its text when the chat scrolls.
type textPos struct{ back, col int }

// selection is a dragged chat text range: anchor where the press was, head where the pointer is.
type selection struct{ anchor, head textPos }

// span is the selection's ends in reading order: the older row (more rows back) first.
func (s selection) span() (from, to textPos) {
	from, to = s.anchor, s.head
	if to.back > from.back || (to.back == from.back && to.col < from.col) {
		from, to = to, from
	}
	return from, to
}

// cols is the column range [c0, c1) of the row back rows up that the selection covers, in a chat cell
// width wide; ok is false when it covers none of it.
func (s selection) cols(back, width int) (c0, c1 int, ok bool) {
	from, to := s.span()
	if back > from.back || back < to.back {
		return 0, 0, false
	}
	c0, c1 = 1, width // column 0 is the gutter (read and pin marks), never text
	if back == from.back {
		c0 = from.col
	}
	if back == to.back {
		c1 = min(to.col+1, width)
	}
	return c0, c1, c0 < c1
}

// selInk draws selected text: reversed, so it reads in any palette.
var selInk = lipgloss.NewStyle().Reverse(true)

// chatRows is the number of chat rows and the screen row of the first one (under the top border and the
// threads box).
func (a App) chatRows() (page, y int) {
	rows, sep := a.threadRows()
	return a.page(), 1 + rows + sep
}

// chatText is the screen column of the chat cell's first column and the chat cell's width.
func (a App) chatText() (x, width int) { return chanW + 2, a.w - chanW - nickW - 4 }

// textAt is the chat text cell under the screen cell (x, y), clamped into the chat pane, so a drag past
// an edge selects to that edge.
func (a App) textAt(x, y int) textPos {
	page, y0 := a.chatRows()
	x0, width := a.chatText()
	r := min(max(y-y0, 0), page-1)
	return textPos{a.off() + page - 1 - r, min(max(x-x0, 1), width-1)}
}

// pressText answers a left press: on the chat text it starts a gesture there (ok), whose release is a
// click or a copy; anywhere it drops the last selection.
func (a App) pressText(x, y int) (App, bool) {
	a.sel = nil
	page, y0 := a.chatRows()
	x0, width := a.chatText()
	if !wide(a.w, a.h) || y < y0 || y >= y0+page || x <= x0 || x >= x0+width {
		return a, false
	}
	p := a.textAt(x, y)
	a.sel, a.drag = &selection{p, p}, selDrag{tea.Mouse{X: x, Y: y, Button: tea.MouseLeft}}
	return a, true
}

// release ends the gesture. A selection that moved is copied; one that never moved is the click it was, and
// copies nothing, so an ordinary click cannot clobber the clipboard.
func (a App) release() (tea.Model, tea.Cmd) {
	d := a.drag
	a.drag = nil
	s, ok := d.(selDrag)
	if !ok || a.sel == nil {
		return a, nil
	}
	if a.sel.anchor == a.sel.head {
		a.sel = nil
		return a.click(s.at)
	}
	if text := a.selText(); text != "" {
		return a, tea.SetClipboard(text)
	}
	return a, nil
}

// selText is the selected text as shown, one line per row, trailing blanks cut.
// shortcut: only rows on screen are read, so a selection scrolled half out copies its visible half;
// read rows through tailRows when that matters.
func (a App) selText() string {
	rows := strings.Split(a.chatBase(), "\n")
	page, y0 := a.chatRows()
	x0, width := a.chatText()
	var out []string
	for r := 0; r < page && y0+r < len(rows); r++ {
		if c0, c1, ok := a.sel.cols(a.off()+page-1-r, width); ok {
			out = append(out, strings.TrimRight(ansi.Strip(ansi.Cut(rows[y0+r], x0+c0, x0+c1)), " "))
		}
	}
	return strings.Join(out, "\n")
}

// withSelection draws the selection reversed over the chat rows of screen.
func (a App) withSelection(screen string) string {
	if a.sel == nil || !wide(a.w, a.h) {
		return screen
	}
	rows := strings.Split(screen, "\n")
	page, y0 := a.chatRows()
	x0, width := a.chatText()
	for r := 0; r < page && y0+r < len(rows); r++ {
		if c0, c1, ok := a.sel.cols(a.off()+page-1-r, width); ok {
			row := rows[y0+r]
			rows[y0+r] = ansi.Cut(row, 0, x0+c0) + selInk.Render(ansi.Strip(ansi.Cut(row, x0+c0, x0+c1))) + ansi.Cut(row, x0+c1, a.w)
		}
	}
	return strings.Join(rows, "\n")
}
