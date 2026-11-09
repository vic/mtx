package ui

// The context menu is ported from tuios, internal/app/contextmenu.go, internal/app/render_context_menu.go
// and internal/input/contextmenu.go (github.com/Gaurav-Gosain/tuios): a surface panel with a title chip,
// a › marker on the highlighted row, key hints right-aligned and dim, the destructive row in the alert
// color; it opens down and right of the pointer and flips at an edge; a row runs exactly what its key
// runs; a press away from it closes it and runs nothing.
// MIT License, Copyright (c) 2025 Gaurav Gosain.

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	ctxMinWidth = 16            // below this a row has no room for a marker and a label
	ctxMaxWidth = 36            // a context menu is a short list of short labels, not a floating panel
	ctxGap      = 2             // the least run of spaces between a label and its hint
	ctxChrome   = panelBody + 1 // rows that are not items: the top pad, the title chip, the blank under it, the bottom pad
	ctxBodyY    = panelBody     // the first item row, under the top pad, the chip and the blank
)

// ctxMenu draws the which-key menu name at the pointer (x, y), and sel is its highlighted row. It places
// only the menu it was opened on: any other menu is drawn as a which-key box.
type ctxMenu struct {
	name string
	x, y int
	sel  int
}

// ctxRows is the rows of menu name: one per action, a repeated action once (the message menu binds reply
// to two keys).
func ctxRows(name string) []menuItem {
	var out []menuItem
	seen := map[string]bool{}
	for _, it := range menus[name] {
		if !seen[it.do] {
			seen[it.do] = true
			out = append(out, it)
		}
	}
	return out
}

// ctxOn reports whether the open menu is drawn as the context menu.
func (a App) ctxOn() bool { return a.ctx != nil && a.ctx.name == a.menu }

// ctxLayout is the context menu as drawn: its lines, the screen cell of its top-left corner, and the
// first row shown (a menu taller than the screen scrolls so the highlighted row stays in view).
func (a App) ctxLayout() (lines []string, x, y, start int) {
	items := ctxRows(a.ctx.name)
	hints := make([]string, len(items))
	width := max(ctxMinWidth, lipgloss.Width(a.ctx.name)+2)
	for i, it := range items {
		hints[i] = menuHint(it)
		w := 2 + lipgloss.Width(it.label)
		if hints[i] != "" {
			w += ctxGap + lipgloss.Width(hints[i])
		}
		width = max(width, w)
	}
	width = max(min(width, ctxMaxWidth, a.w-2*panelPad), 1)
	visible := max(min(len(items), a.h-ctxChrome), 1)
	start = min(max(a.ctx.sel-visible+1, 0), max(len(items)-visible, 0))

	var body []panelRow
	for i := start; i < min(start+visible, len(items)); i++ {
		body = append(body, ctxRow(items[i], hints[i], i == a.ctx.sel, width))
	}
	lines = strings.Split(panel{title: a.ctx.name, width: width, body: body}.render(), "\n")

	bw, bh := width+2*panelPad, len(lines)
	x, y = a.ctx.x, a.ctx.y
	if x+bw > a.w { // flip to the other side of the pointer rather than slide along the edge
		x = a.ctx.x - bw + 1
	}
	if y+bh > a.h {
		y = a.ctx.y - bh + 1
	}
	return lines, max(min(x, a.w-bw), 0), max(min(y, a.h-bh), 0), start
}

// ctxRow is one item row, width wide: the label gives up room before the hint does, and the destructive
// row is in the alert color.
func ctxRow(it menuItem, hint string, on bool, width int) panelRow {
	label := cell.Bold(on)
	if it.do == ":redact" {
		label = alert
	}
	hw := 0
	if hint != "" {
		hw = lipgloss.Width(hint) + ctxGap
	}
	text := ansi.Truncate(it.label, max(width-2-hw, 1), "…")
	gap := max(width-2-lipgloss.Width(text)-lipgloss.Width(hint), 0)
	return listRow(label.Render(text)+strings.Repeat(" ", gap)+dimmed.Render(hint), on)
}

// withCtx draws the context menu over screen.
func (a App) withCtx(screen string) string {
	lines, x, y, _ := a.ctxLayout()
	y += a.popDY()
	rows := strings.Split(screen, "\n")
	for j, l := range lines {
		if r := y + j; r < len(rows) {
			bw := lipgloss.Width(l)
			rows[r] = ansi.Cut(rows[r], 0, x) + l + ansi.Cut(rows[r], x+bw, a.w)
		}
	}
	return strings.Join(rows, "\n")
}

// ctxHit is the row under the screen cell (px, py), -1 for none, and whether the cell is on the menu.
func (a App) ctxHit(px, py int) (row int, on bool) {
	lines, x, y, start := a.ctxLayout()
	bw := lipgloss.Width(lines[0])
	on = px >= x && px < x+bw && py >= y && py < y+len(lines)
	if r := py - y - ctxBodyY; on && r >= 0 && r < len(lines)-ctxChrome {
		return start + r, true
	}
	return -1, on
}

// ctxOpen answers a right press on a chat line: the line becomes current and its message menu opens at
// the pointer.
func (a App) ctxOpen(x, y int) (App, bool) {
	page, y0 := a.chatRows()
	x0, width := a.chatText()
	if !wide(a.w, a.h) || y < y0 || y >= y0+page || x <= x0 || x >= x0+width {
		return a, false
	}
	_, idx, _, ok := a.lineAt(y - y0)
	if !ok {
		return a, false
	}
	a.hit, a.menu, a.sel = idx, "message", nil
	a.ctx = &ctxMenu{name: "message", x: x, y: y}
	return a, true
}

// ctxPress answers a press while the context menu is up: a row runs, the rest of the menu swallows the
// press, and a press away from it closes it.
func (a App) ctxPress(m tea.Mouse) (tea.Model, tea.Cmd) {
	row, on := a.ctxHit(m.X, m.Y)
	switch {
	case !on:
		a.menu, a.ctx = "", nil
	case row >= 0 && m.Button == tea.MouseLeft:
		return a.ctxRun(row)
	}
	return a, nil
}

// ctxRun runs row i as its key would: the which-key menu is still open, so the key picks the item.
func (a App) ctxRun(i int) (tea.Model, tea.Cmd) {
	it := ctxRows(a.ctx.name)[i]
	a.ctx = nil
	return a.key(keyMsg(it.key))
}

// ctxKey answers a key while the context menu is up: the list keys move (listnav), enter runs, any other
// key is the which-key menu's.
func (a App) ctxKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m := listKeys(msg.String()); m != moveNone {
		c := *a.ctx
		c.sel = listStep(c.sel, m, len(ctxRows(c.name)), a.h-ctxChrome)
		a.ctx = &c
		return a, nil
	}
	if msg.String() == "enter" {
		return a.ctxRun(a.ctx.sel)
	}
	a.ctx = nil
	return a.key(msg)
}

// ctxHover highlights the row under the pointer, so the pointer tracks like the arrows do.
func (a App) ctxHover(px, py int) App {
	if row, _ := a.ctxHit(px, py); row >= 0 {
		c := *a.ctx
		c.sel = row
		a.ctx = &c
	}
	return a
}
