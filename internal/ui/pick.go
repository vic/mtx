package ui

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// pickMatch is one channel the picker shows: its name and the matched rune positions.
type pickMatch struct {
	name  string
	idx   int // position in the list given to pickList
	at    []int
	score int
}

// fuzzy matches q as a case-insensitive subsequence of s. Runs of matched runes and
// matches at a word start ("#", "&", "-", "_", ".", "~") score higher.
func fuzzy(q, s string) (pickMatch, bool) {
	m := pickMatch{name: s}
	rs, qs := []rune(strings.ToLower(s)), []rune(strings.ToLower(q))
	j := 0
	for i := 0; i < len(rs) && j < len(qs); i++ {
		if rs[i] != qs[j] {
			continue
		}
		m.score++
		if n := len(m.at); n > 0 && m.at[n-1] == i-1 {
			m.score += 3
		}
		if i == 0 || strings.ContainsRune("#&-_./~", rs[i-1]) || unicode.IsSpace(rs[i-1]) {
			m.score += 5
		}
		m.at = append(m.at, i)
		j++
	}
	return m, j == len(qs)
}

// pickList is the names matching q, best first; ties keep the given order.
func pickList(names []string, q string) []pickMatch {
	var out []pickMatch
	for i, n := range names {
		if m, ok := fuzzy(strings.TrimSpace(q), n); ok {
			m.idx = i
			out = append(out, m)
		}
	}
	slices.SortStableFunc(out, func(a, b pickMatch) int { return b.score - a.score })
	return out
}

// pickBox is the picker box (channels or lines, titled what), drawn over the screen by overlayCentered.
// pickW is the picker box width: three quarters of the threads pane in the wide layout, else half the screen.
func pickW(w, h int) int {
	if wide(w, h) {
		return min(max((w-chanW-nickW-4)*3/4, 40), w)
	}
	return min(max(w/2, 40), w)
}

func pickBox(w, h int, what, q string, ms []pickMatch, sel int, multi bool, marks []string) string {
	rows := []string{title.Render(what) + " " + prompt.Render(">") + " " + q, ""}
	start, end := pickWindow(len(ms), sel, max(h-6, 1))
	if start > 0 {
		rows = append(rows, dimmed.Render(fmt.Sprintf("↑ %d more", start)))
	}
	for i := start; i < end; i++ {
		m := ms[i]
		mark, st := " ", dimmed
		if i == sel {
			mark, st = "◆", cell.Bold(true)
		}
		var b strings.Builder
		// one row in the box: cut by cells (wide runes count 2), with a column to spare for the mark
		rs := []rune(ansi.Truncate(m.name, max(pickW(w, h)-8, 1), "…"))
		for k, r := range rs {
			if slices.Contains(m.at, k) {
				b.WriteString(shortcut.Bold(true).Render(string(r)))
			} else {
				b.WriteString(st.Render(string(r)))
			}
		}
		check := ""
		if multi { // S picker: a check column, marked nicks get a tick
			check = " "
			if slices.Contains(marks, m.name) {
				check = "✓"
			}
			check += " "
		}
		rows = append(rows, title.Render(mark)+" "+check+b.String())
	}
	if end < len(ms) {
		rows = append(rows, dimmed.Render(fmt.Sprintf("↓ %d more", len(ms)-end)))
	}
	if len(ms) == 0 {
		rows = append(rows, dimmed.Render("no match"))
	}
	keys := keyHint("enter", "go", "up down", "select", "esc", "close")
	if multi { // S: space marks, enter applies
		keys = keyHint("space", "select", "enter", "go", "esc", "close")
	}
	rows = append(rows, "", keys)
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(pal.slots[3]).
		Padding(0, 1).Width(pickW(w, h)).Render(strings.Join(rows, "\n"))
	return box
}

// grepBox is the ctrl+g input box: the filter text, drawn over the screen by overlayCentered.
func grepBox(w int, q string) string {
	rows := []string{title.Render("grep") + " " + prompt.Render(">") + " " + q, "", keyHint("enter", "keep", "esc", "clear")}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(pal.slots[3]).
		Padding(0, 1).Width(min(max(w/2, 40), w)).Render(strings.Join(rows, "\n"))
}

// pickWindow is the matches a picker of rowsMax rows shows: [start, end) with the selection sel in it. A
// "↑ more" row above and a "↓ more" row below take a row each when there are matches past the window.
func pickWindow(n, sel, rowsMax int) (start, end int) {
	for range 3 {
		top := 0
		if start > 0 {
			top = 1
		}
		end = min(start+rowsMax-top, n)
		if end < n {
			end = min(start+rowsMax-top-1, n)
		}
		if sel < end || sel < start {
			return start, end
		}
		start += sel - end + 1
	}
	return start, min(start+rowsMax, n)
}

// exactList keeps the names that contain q (case-insensitive), in their order; at marks the match.
func exactList(names []string, q string) []pickMatch {
	q = strings.ToLower(strings.TrimSpace(q))
	var out []pickMatch
	for i, n := range names {
		j := strings.Index(strings.ToLower(n), q)
		if j < 0 {
			continue
		}
		m := pickMatch{name: n, idx: i}
		for k := range len([]rune(q)) {
			m.at = append(m.at, len([]rune(n[:j]))+k)
		}
		out = append(out, m)
	}
	return out
}
