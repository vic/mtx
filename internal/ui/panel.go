package ui

// The panel is ported from tuios, internal/overlay/panel.go and rows.go (github.com/Gaurav-Gosain/tuios):
// every popup is one borderless floating panel, a solid surface fill with an inset accent title chip, a
// body, and a muted footer of key hints under a rule. Lists in it follow one row rule: the cursor row takes
// the selection ground across the whole row behind a › marker.
// MIT License, Copyright (c) 2025 Gaurav Gosain.

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// panelPad is the surface cells each side of a panel's content.
const panelPad = 2

// panelBody is the rows above a panel's body: the top pad, the title chip, the blank under it.
const panelBody = 3

// The panel grounds, set by usePalette: the surface every panel row sits on and the cursor row's.
var surfacePre, cursorRowPre string

// panelRow is a body row: its pre-styled text, and whether it is a list's cursor row.
type panelRow struct {
	s      string
	cursor bool
}

// textRows are plain body rows.
func textRows(ss ...string) []panelRow {
	rs := make([]panelRow, len(ss))
	for i, s := range ss {
		rs[i] = panelRow{s: s}
	}
	return rs
}

// listRow is a list row: two marker columns, › on the cursor row.
func listRow(s string, cursor bool) panelRow {
	if cursor {
		return panelRow{sgrOf(focusBorder) + "› " + sgrReset + s, true}
	}
	return panelRow{"  " + s, false}
}

// panel is a popup: width is its inner content width, the block is width+2*panelPad wide.
type panel struct {
	title string
	width int
	body  []panelRow
	hints []string // "key", "label" pairs for the footer; none, no footer
}

// onGround is s drawn on the ground that pre starts, filled to n cells: every reset in s falls back to the
// ground, so the styled spans in a row keep its fill.
func onGround(pre, s string, n int) string {
	s = ansi.Truncate(s, n, "")
	if w := ansi.StringWidth(s); w < n {
		s += strings.Repeat(" ", n-w)
	}
	return pre + strings.ReplaceAll(s, sgrReset, sgrReset+pre) + sgrReset
}

// render is the panel's rows joined.
func (p panel) render() string {
	pad := onGround(surfacePre, "", panelPad)
	line := func(r panelRow) string {
		pre := surfacePre
		if r.cursor {
			pre = cursorRowPre
		}
		return pad + onGround(pre, r.s, p.width) + pad
	}
	blank := onGround(surfacePre, "", p.width+2*panelPad)
	chip := lipgloss.NewStyle().Background(pal.slots[0x0D]).Foreground(pal.slots[0]).Bold(true).Render(" " + ansi.Truncate(p.title, max(p.width-2, 1), "…") + " ")
	rows := []string{blank, line(panelRow{s: chip}), blank}
	for _, r := range p.body {
		rows = append(rows, line(r))
	}
	if len(p.hints) > 0 {
		rows = append(rows, blank, line(panelRow{s: dimmed.Render(strings.Repeat("─", p.width))}), line(panelRow{s: keyHint(p.hints...)}))
	}
	return strings.Join(append(rows, blank), "\n")
}
