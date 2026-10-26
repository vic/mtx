package ui

import (
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"mtx/internal/model"
)

const (
	chanW    = 15 // channel list pane, inner width
	nickW    = 10 // nick pane, inner width
	minWide  = 40 // below this width (or h < minWideH) View shows the lines pane only
	minWideH = 5
	nickCol  = 11 // "<nick>" column in the lines pane
)

// Styles are named by role, one table; ANSI 16 colors only, so the terminal theme
// picks the shades.
var (
	cell        = lipgloss.NewStyle().Inline(true)
	dimmed      = fg("8")
	border      = dimmed
	focusBorder = fg("13").Bold(true) // the pane that takes keys
	faint       = dimmed
	title       = fg("5").Bold(true)
	shortcut    = fg("5")
	prompt      = fg("6").Bold(true)
	unread      = fg("3").Bold(true)
	roomName    = fg("13")
	alert       = fg("9").Bold(true)
	current     = cell.Background(lipgloss.Color("236"))                                           // selected pane row
	cursor      = cell.Background(lipgloss.Color("13"))                                            // the input cursor: the focused pane border color
	selected    = cell.Bold(true).Background(lipgloss.Color("8")).Foreground(lipgloss.Color("15")) // current channel
	helpHead    = fg("2").Bold(true)
	// nickColors leaves out red (bypass) and gray (dim lines).
	nickColors = []string{"2", "3", "4", "5", "6", "10", "11", "12", "13", "14"}
)

func fg(c string) lipgloss.Style { return cell.Foreground(lipgloss.Color(c)) }

// badge is the mode block at the left of the modeline: black on a mode color, red in bypass.
func badge(mode string, bypass bool) string {
	bg := "5"
	if mode == "INSERT" {
		bg = "6" // cyan
	}
	if bypass {
		bg = "9"
	}
	return cell.Bold(true).Background(lipgloss.Color(bg)).Foreground(lipgloss.Color("0")).Render(" " + mode + " ") // Inline drops Padding
}

// keyHint is "key desc" pairs: keys in the shortcut color, descriptions dimmed.
func keyHint(pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, shortcut.Render(pairs[i])+" "+dimmed.Render(pairs[i+1]))
	}
	return strings.Join(parts, "  ")
}

// nickStyle gives each nick a stable color.
func nickStyle(nick string) lipgloss.Style {
	h := fnv.New32a()
	h.Write([]byte(nick))
	return fg(nickColors[h.Sum32()%uint32(len(nickColors))])
}

// fit pads or truncates s to exactly n columns.
func fit(st lipgloss.Style, s string, n int) string {
	return st.Width(n).MaxWidth(n).Render(strings.ReplaceAll(s, "\n", " "))
}

// fitCell is fit(cell, s, n), memoized: side pane cells repeat every frame.
func fitCell(s string, n int) string {
	k := fitKey{s, n}
	fitCells.mu.Lock()
	defer fitCells.mu.Unlock()
	if r, ok := fitCells.m[k]; ok {
		return r
	}
	r := fit(cell, s, n)
	if len(fitCells.m) >= styledMax {
		fitCells.m = map[fitKey]string{}
	}
	fitCells.m[k] = r
	return r
}

type fitKey struct {
	s string
	n int
}

// fitCells holds fitCell results across frames.
var fitCells = struct {
	mu sync.Mutex
	m  map[fitKey]string
}{m: map[fitKey]string{}}

func wide(w, h int) bool { return w >= minWide && h >= minWideH }

// paneRows is how many scrollback rows View shows in a w x h screen with in input rows.
func paneRows(w, h, in int) int {
	if wide(w, h) {
		return max(h-3-in, 1) // top and bottom border, modeline, input
	}
	return max(h-2-in, 1) // title, modeline, input
}

// inputRows is the input row text (with its prompt) wrapped to w columns: the last rows, at most a third of h,
// so the end being typed stays visible.
func inputRows(input string, w, h int) []string {
	rs := strings.Split(lipgloss.NewStyle().Width(max(w, 1)).Render(input), "\n")
	k := max(h/3, 1)
	if len(rs) > k { // the window ends at the cursor's row, so moving up scrolls up
		at := len(rs) - 1
		for i, r := range rs {
			if strings.Contains(r, cursorMark) {
				at = i
				break
			}
		}
		start := min(max(at-k+1, 0), len(rs)-k)
		rs = rs[start : start+k]
	}
	style := strings.NewReplacer(cursorMark, cursorOn, cursorEndMark, cursorOff)
	for i := range rs {
		rs[i] = style.Replace(rs[i])
	}
	return rs
}

// seg is a pane title in a top border: " title ───" filled to n columns.
func seg(name string, n int, line lipgloss.Style) string {
	t := " " + name + " "
	if lipgloss.Width(t) >= n {
		return fit(title, t, n)
	}
	return title.Render(t) + line.Render(strings.Repeat("─", n-lipgloss.Width(t)))
}

// lineRow is lineRowUncached, memoized: a styled row depends only on these inputs, so a frame
// that shows the same rows reuses them.
func lineRow(rows []string, of []int, shown []model.Line, hit, i, n int, lead string, code []bool, q string) string {
	if i >= len(rows) {
		return lineRowUncached(rows, of, shown, hit, i, n, lead, code, q)
	}
	l := shown[of[i]]
	k := styledKey{row: rows[i], nick: l.Nick, dim: l.Dim, kind: l.Kind, hit: of[i] == hit, code: code[i], q: q, n: n, lead: lead}
	styled.mu.Lock()
	s, ok := styled.m[k]
	styled.mu.Unlock()
	if ok {
		return s
	}
	s = lineRowUncached(rows, of, shown, hit, i, n, lead, code, q)
	styled.mu.Lock()
	if len(styled.m) >= styledMax {
		styled.m = map[styledKey]string{}
	}
	styled.m[k] = s
	styled.mu.Unlock()
	return s
}

// styledKey is everything lineRowUncached reads for one row.
type styledKey struct {
	row, nick, q, lead string
	dim, hit, code     bool
	kind               model.LineKind
	n                  int
}

// styledMax bounds the styled row cache; it is cleared when full.
const styledMax = 20000

// styled holds styled rows across frames (View runs on one goroutine; tests may run in parallel).
var styled = struct {
	mu sync.Mutex
	m  map[styledKey]string
}{m: map[styledKey]string{}}

// lineRowUncached is scrollback row i in n columns: the search hit reversed, else the
// nick in its color, dim or tool text gray, chat text with markdown colors.
// code[i]: the row is inside a ``` block of its line.
func lineRowUncached(rows []string, of []int, shown []model.Line, hit, i, n int, lead string, code []bool, q string) string {
	if i >= len(rows) {
		return fit(cell, "", n)
	}
	l, r := shown[of[i]], rows[i]
	plain := l.Dim || l.Kind == model.Tool
	st := cell
	if plain {
		st = faint
	}
	text := func(s string) string {
		if plain {
			return mentions(s, st)
		}
		pad := len(s) - len(strings.TrimLeft(s, " ")) // indentation stays unstyled
		return s[:pad] + markHits(s[pad:], st, code[i], q)
	}
	if s := stampRE.FindString(r); s != "" {
		lead, r = lead+dimmed.Render(s), r[len(s):]
	}
	// body: the text right of the nick column. The current message's bar takes the
	// last leading space, the column just left of the text.
	body := func(s string) string {
		pad := len(s) - len(strings.TrimLeft(s, " "))
		if of[i] != hit || pad == 0 {
			return text(s)
		}
		return text(s[:pad-1]) + gutterBar + text(s[pad:])
	}
	nk := "<" + l.Nick + ">"
	if after, ok := strings.CutPrefix(r, nk); ok {

		if of[i] == hit { // the selected line: nick bold italic, its <> in the cursor color
			br := fg("13").Bold(true).Italic(true)
			return fit(cell, lead+br.Render("<")+nickStyle(l.Nick).Bold(true).Italic(true).Render(l.Nick)+br.Render(">")+body(after), n)
		}
		return fit(cell, lead+nickStyle(l.Nick).Render(nk)+body(after), n)
	}
	return fit(cell, lead+body(r), n)
}

// codeRows marks the rows inside a ``` block; the fence state restarts per line.
func codeRows(rows []string, of []int, shown []model.Line) []bool {
	out := make([]bool, len(rows))
	in := false
	for i, r := range rows {
		if i == 0 || of[i] != of[i-1] {
			in = false
		}
		r = strings.TrimPrefix(r[len(stampRE.FindString(r)):], "<"+shown[of[i]].Nick+">")
		if fenced(r) {
			in = !in
			continue
		}
		out[i] = in
	}
	return out
}

// View renders the box layout of DESIGN D:290-299: channels, lines and nicks
// panes with borders, then the modeline and the input line, in w columns and h
// rows. h < 3 or w < 1 yields "". off scrolls the lines pane back: it shows the
// lines ending off before the newest (clamped, D:304). hit is the index of the
// search hit in the current channel (-1 none); it is drawn reversed.
// Channel markers: ● current, ○ gone, else the unread count.
func View(m model.Model, ml model.Modeline, input string, w, h, off, hit int) string {
	cur := m.Current()
	var names []string
	for _, c := range m.All() {
		names = append(names, c.Name)
	}
	return render(m, ml, "> "+input, w, h, off, hit, sidePanes{chans: names, nicks: memberNicks(cur)}, cur.Lines.All(), wrapLine, nil, "", "")
}

// render is View with the nick pane given as nicks and the shown lines of the current channel.
func render(m model.Model, ml model.Modeline, input string, w, h, off, hit int, p sidePanes, shown []model.Line, wrap wrapFunc, sugg []string, hint, q string) string {
	if w < 1 || h < 3 {
		return ""
	}
	cur := m.Current()
	ir := append(sugg, inputRows(input, w, h)...) // the suggestion row sits right above the input
	n := paneRows(w, h, len(ir))
	details := ml.Left // channel details (members, bypass, unknown, lines/min) are the chat frame title
	ml.Left = ""
	tRows, sep := threadBox(p, wide(w, h)) // the threads box above the chat, and its separator line
	lines, of := tailRows(shown, n-tRows-sep, off, textWidth(w, h), wrap)
	code := codeRows(lines, of, shown)

	var rows []string
	if !wide(w, h) {
		rows = append(rows, fit(title, details, w))
		for i := 0; i < n; i++ {
			rows = append(rows, lineRow(lines, of, shown, hit, i, w, "", code, ""))
		}
	} else {
		byName := map[string]model.Channel{}
		for _, c := range m.All() {
			byName[c.Name] = c
		}
		var chans []model.Channel
		for _, n := range p.chans[min(p.chanTop, len(p.chans)):] {
			if strings.HasPrefix(n, nickLabel) {
				chans = append(chans, model.Channel{Name: n}) // a group label row
				continue
			}
			chans = append(chans, byName[n])
		}
		mid := w - chanW - nickW - 4 // four border columns
		// The focused pane's borders are drawn in focusBorder (vic 2026-10-23). Pane k
		// (0 channels, 1 chat, 2 nicks) owns vertical borders k and k+1 and segment k.
		pane := 1
		switch p.focus {
		case focusChans:
			pane = 0
		case focusNicks:
			pane = 2
		}
		edge := func(k int, s string) string { // vertical border k, or a corner on it
			return borderGlyph(k == pane || k == pane+1, s)
		}
		threadsFocus := p.focus == focusThreads
		midLit := func(inThreads bool) bool { return pane == 1 && inThreads == threadsFocus } // the middle column is lit for the box or the chat, never both
		styleOf := func(lit bool) lipgloss.Style {
			if lit {
				return focusBorder
			}
			return border
		}
		vEdge := func(k int, s string, inThreads bool) string { // vertical border k of a row in the box (inThreads) or the chat
			switch {
			case k == 1 && pane == 0, k == 2 && pane == 2: // shared with a focused side pane
				return borderGlyph(true, s)
			case k == 1 || k == 2:
				return borderGlyph(midLit(inThreads), s)
			}
			return edge(k, s)
		}
		line := func(k int) lipgloss.Style {
			if k == pane {
				return focusBorder
			}
			return border
		}
		rows = append(rows, edge(0, "┌")+seg(p.title("channels", focusChans), chanW, line(0))+vEdge(1, "┬", tRows > 0)+
			seg(midTitle(p, details), mid, styleOf(midLit(tRows > 0)))+vEdge(2, "┬", tRows > 0)+seg(p.title("nicks", focusNicks), nickW, line(2))+edge(3, "┐"))
		for i := 0; i < n; i++ {
			nk := at(p.nicks, i+p.nickTop)
			label := strings.HasPrefix(nk, nickLabel) // a group label: dim, not a nick
			if label {
				nk = dimmed.Render(strings.TrimPrefix(nk, nickLabel))
			} else if nk != "" { // an empty Render still emits escapes
				mark, name := "", nk
				for _, sm := range statusMarks {
					if rest, ok := strings.CutPrefix(nk, sm); ok {
						mark, name = sm, rest
					}
				}
				nk = mark + nickStyle(name).Render(name)
			}
			cc, nc := chanCell(chans, cur.Name, i), fitCell(" "+nk, nickW)
			if p.focus == focusChans && i == p.sel && i < len(chans) {
				cc = fit(current, " "+chans[i].Name, chanW)
			}
			if p.focus == focusNicks && i == p.nickSel {
				nc = fit(current, " "+p.nicks[min(i+p.nickTop, len(p.nicks)-1)], nickW)
			}
			if sep == 1 && i == tRows { // the line between the threads box and the chat box
				rows = append(rows, edge(0, "│")+cc+vEdge(1, "├", false)+styleOf(midLit(false)).Render(frameRule(details, mid))+
					vEdge(2, "┤", false)+nc+edge(3, "│"))
				continue
			}
			midCell := ""
			if i < tRows {
				midCell = threadsRow(p, i, mid)
			} else {
				midCell = lineRow(lines, of, shown, hit, i-tRows-sep, mid, p.lead(of, shown, i-tRows-sep), code, q)
			}
			rows = append(rows, edge(0, "│")+cc+vEdge(1, "│", i < tRows)+midCell+
				vEdge(2, "│", i < tRows)+nc+edge(3, "│"))
		}
		rows = append(rows, edge(0, "└")+line(0).Render(strings.Repeat("─", chanW))+vEdge(1, "┴", false)+styleOf(midLit(false)).Render(strings.Repeat("─", mid))+
			vEdge(2, "┴", false)+line(2).Render(strings.Repeat("─", nickW))+edge(3, "┘"))
	}

	left, lst := " ", cell
	if ml.Mode != "" {
		left = badge(ml.Mode, ml.Red) + " "
	}
	if ml.Red {
		lst = alert
	}
	left += lst.Render(ml.Left)
	if hint != "" { // keys for this mode, between the status and the clock (vic 2026-10-22)
		left += "   " + hint
	}
	right := dimmed.Render(ml.Right + " ")
	rows = append(rows, fit(cell, fit(cell, left, max(w-lipgloss.Width(right), 0))+right, w))
	for _, r := range ir {
		if after, ok := strings.CutPrefix(r, ">"); ok {
			r = prompt.Render(">") + after
		} else if after, ok := strings.CutPrefix(r, "RE:"); ok { // a reply prompt is the same color
			r = prompt.Render("RE:") + after
		} else if after, ok := strings.CutPrefix(r, "! "); ok { // the ! prompt matches the others
			r = prompt.Render("!") + " " + after
		} else if after, ok := strings.CutPrefix(r, "$ "); ok { // the $ and : prompts match the chat prompt
			r = prompt.Render("$") + " " + after
		} else if after, ok := strings.CutPrefix(r, ": "); ok {
			r = prompt.Render(":") + " " + after
		}
		rows = append(rows, fit(cell, r, w))
	}
	return strings.Join(rows, "\n")
}

// chanCell is chanCellUncached, memoized on what the cell shows.
func chanCell(chans []model.Channel, curName string, i int) string {
	if i >= len(chans) {
		return fitCell("", chanW)
	}
	c := chans[i]
	k := chanKey{name: c.Name, kind: c.Kind, gone: c.Gone, unread: c.Unread, cur: c.Name == curName}
	chanCells.mu.Lock()
	defer chanCells.mu.Unlock()
	if s, ok := chanCells.m[k]; ok {
		return s
	}
	s := chanCellUncached(chans, curName, i)
	if len(chanCells.m) >= styledMax {
		chanCells.m = map[chanKey]string{}
	}
	chanCells.m[k] = s
	return s
}

// chanKey is everything chanCellUncached reads for one channel.
type chanKey struct {
	name      string
	kind      model.ChanKind
	gone, cur bool
	unread    int
}

// chanCells holds rendered channel cells across frames.
var chanCells = struct {
	mu sync.Mutex
	m  map[chanKey]string
}{m: map[chanKey]string{}}

// chanCellUncached is channel row i of the channel pane, chanW wide.
func chanCellUncached(chans []model.Channel, curName string, i int) string {
	if i >= len(chans) {
		return fit(cell, "", chanW)
	}
	if l, ok := strings.CutPrefix(chans[i].Name, nickLabel); ok {
		return fit(dimmed, " "+l, chanW)
	}
	c := chans[i]
	marker, markSt, nameSt := "", cell, nickStyle(strings.TrimPrefix(c.Name, "#")) // a session is its main agent nick
	if c.Kind == model.RoomChan {
		nameSt = roomName
	}
	switch {
	case c.Gone:
		marker, markSt, nameSt = "○", faint, faint
	case c.Name == curName:
		marker = "●"
	case c.Unread > 0:
		marker, markSt = strconv.Itoa(c.Unread), unread
	}
	if c.Name == curName { // plain text: inner colors would end the background early
		return fit(selected, " "+fit(cell, c.Name, 11)+fit(cell, marker, 3), chanW)
	}
	return fit(cell, " "+fit(nameSt, c.Name, 11)+fit(markSt, marker, 3), chanW)
}

// textWidth is the width of the scrollback text in a w x h screen.
func textWidth(w, h int) int {
	if wide(w, h) {
		return w - chanW - nickW - 4 - 1 // borders and the leading space
	}
	return w
}

// wrapLine word-wraps l to width columns: "<nick>" on the first row, the text
// aligned under itself on the rows after it.
func wrapLine(l model.Line, width int) []string {
	nk := strings.Repeat(" ", nickCol) // a system line (no nick) has no <> mark
	if l.Nick != "" {
		nk = "<" + l.Nick + ">"
		nk += strings.Repeat(" ", max(nickCol-lipgloss.Width(nk), 1))
	}
	bw := max(width-lipgloss.Width(nk), 1)
	var out []string
	text := l.Text
	if l.Reacts != "" {
		text += "  [" + l.Reacts + "]"
	}
	for i, r := range strings.Split(lipgloss.NewStyle().Width(bw).Render(text), "\n") {
		r = strings.TrimRight(r, " ")
		if i == 0 {
			out = append(out, nk+r)
		} else {
			out = append(out, strings.Repeat(" ", lipgloss.Width(nk))+r)
		}
	}
	return out
}

// rowCount is how many rows c's scrollback takes at width.
func rowCount(ls []model.Line, width int, wrap wrapFunc) int {
	n := 0
	for _, l := range ls {
		n += len(wrap(l, width))
	}
	return n
}

// tailRows returns the n rows ending off rows before the newest and, per row,
// the index of the line it belongs to. It wraps lines newest-first and stops
// once off+n rows are collected, so cost follows the screen, not the ring.
func tailRows(all []model.Line, n, off, width int, wrap func(model.Line, int) []string) ([]string, []int) {
	off = max(off, 0)
	var rev []string // rows, newest first
	var revOf []int
	for i := len(all) - 1; i >= 0 && len(rev) < off+n; i-- {
		rs := wrap(all[i], width)
		for j := len(rs) - 1; j >= 0; j-- {
			rev, revOf = append(rev, rs[j]), append(revOf, i)
		}
	}
	rows := make([]string, 0, n)
	of := make([]int, 0, n)
	for k := min(off+n, len(rev)) - 1; k >= off; k-- {
		rows, of = append(rows, rev[k]), append(of, revOf[k])
	}
	return rows, of
}

func at(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return ""
}

func memberNicks(c model.Channel) []string {
	out := make([]string, len(c.Members))
	for i, m := range c.Members {
		out[i] = m.Nick
	}
	return out
}

// chanNicks is the nick pane of c (D:217): user, then the session's main agent and
// its subagents
// (D:229 nick = channel name without "#") or the room's members.
func chanNicks(c model.Channel, user string) []string {
	if c.Kind == model.SessionChan {
		out := []string{user, strings.TrimPrefix(c.Name, "#")}
		for _, ag := range c.Agents {
			out = append(out, ag.Nick)
		}
		return out
	}
	return append([]string{user}, memberNicks(c)...)
}

// helpBox is the help box in h rows at most: taller than that, a window that scrolls.
func helpBox(h, off int) string {
	box := strings.Split(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8")).Padding(0, 1).Render(helpText()), "\n")
	if len(box) <= h {
		return strings.Join(box, "\n")
	}
	off = helpOff(len(box), h, off)
	return strings.Join(box[off:off+h], "\n")
}

// overlayCentered draws box centered over screen (w x h cells); the cells around it keep their content.
func overlayCentered(screen string, w, h int, box string) string {
	rows := strings.Split(screen, "\n")
	bl := strings.Split(box, "\n")
	bw := min(lipgloss.Width(bl[0]), w)
	top, left := max((h-len(bl))/2, 0), max((w-bw)/2, 0)
	for j, b := range bl {
		if i := top + j; i < len(rows) {
			b = ansi.Truncate(b, bw, "")
			rows[i] = fit(cell, ansi.Cut(rows[i], 0, left)+b+ansi.Cut(rows[i], left+bw, w), w)
		}
	}
	return strings.Join(rows, "\n")
}

// helpOff clamps a help scroll offset for a box of n rows in h rows.
func helpOff(n, h, off int) int { return min(max(off, 0), max(n-h, 0)) }

// helpRows is how many rows the help box has.
func helpRows() int { return strings.Count(helpText(), "\n") + 3 } // text rows + top and bottom border

// helpSections is the ? popup: a heading, then key and description pairs.
var helpSections = []struct {
	head string
	keys [][2]string
}{
	{"NORMAL", [][2]string{
		{"up down", "line cursor"},
		{"ctrl+n ctrl+p", "next / previous channel (also gt gT, {n}gt)"},
		{"tab", "focus a pane: chat, channels, nicks, threads (type to filter, enter selects)"},
		{"ctrl+o", "open chat (fuzzy, enter goes there)"},
		{"ctrl+t", "fuzzy file finder: enter inserts the path"},
		{"ctrl+f", "fuzzy file finder: enter inserts the file as a fenced block"},
		{"ctrl+g", "grep: show only lines fuzzy-matching the text (esc clears)"},
		{"wheel j k", "scroll"},
		{"J K", "next / previous speaker change"},
		{"S", "show only one speaker (fuzzy pick; (all) clears)"},
		{"y", "yank (copy) the cursor line"},
		{"shift+drag", "select text (the terminal does it; mtx takes plain drags)"},
		{"ctrl+d ctrl+u", "half page down / up"},
		{"gg G", "top / bottom"},
		{"/", "search: highlights matches, up/down picks the current hit"},
		{"i", "type a new message to the main chat"},
		{"+ 1-9", "quick reaction on the selected line: 👍 ❤️ 😂 🎉 😮 😢 🙏 🔥 👀"},
		{"space", "menu: r react, t thread, m room (mute, rename, topic, invite, fav...), j join, u upload, o open, g media"},
		{"space o c t u", "open chat, thread or user (fuzzy, one kind each)"},
		{"a", "reply to my last or selected message"},
		{"A", "reply to my last or selected message, quoted in the text"},
		{"C", "edit my last or selected message"},
		{"I", "invite a user to this room (fuzzy pick)"},
		{"o", "reply in the last thread used"},
		{"O", "reply in the last thread used, quoted in the text"},
		{"T", "start a thread on the selected message, else the newest"},
		{"t", "toggle main thread / last thread"},
		{":", "run a command (:join, :react, :help...); :!text sends text as is"},
		{"$", "run a shell command locally (suspends mtx; Enter returns)"},
		{"!", "run a shell command, insert its output as a fenced block"},
		{"M", "mentions only (direct lines) / all lines"},
		{"m", "recent chats and threads (fuzzy): type, enter"},
		{"f", "jump: a letter on each visible line; type it to select that line"},
		{"e", "edit my selected message"},
		{"d y", "delete my selected message"},
		{"gx", "open the selected line's media or link"},
		{".", "repeat the last line action (e.g. the same reaction) on the selected line"},
		{"s", "show / hide times (15:04)"},
	}},
	{"INSERT", [][2]string{
		{"enter", "send, then back to NORMAL"},
		{"esc", "back to NORMAL; drops the reply, edit or quote (up down still switch channel)"},
	}},
	{"COMMANDS", [][2]string{
		{":join &room", "join a room by its alias on our server"},
		{":create &room", "create an encrypted private room, alias &room"},
		{":invite @user &room", "invite a user (a bare @user gets our server)"},
		{":kick @user", "remove a member from this room"},
		{":part :close", "leave the room (:close also forgets it); :rm = :close"},
		{":topic text", "set the room topic"},
		{":names :who", "list the room's members"},
		{":msg @user text", "send to the DM with that user (created if missing)"},
		{":whois @user", "show the user's profile"},
		{":help", "this help (same as ?)"},
		{":quit", "quit (also ctrl+c)"},
		{":name <text>", "set the room name"},
		{":tag fav|low|none", "group this room in the channel list (favourites, low priority)"},
		{":nick <name>", "set my display name"},
		{":ignore <user>", "hide a user's messages (:unignore undoes it)"},
		{":unignore <user>", "stop ignoring a user"},
		{":me <action>", "send an action line (* you waves)"},
		{":react <emoji>", "react to the selected line (again removes it)"},
		{":redact [reason]", "delete my selected message"},
		{":open", "download and open the selected line's media"},
		{":upload <path>", "send a local file to this room (encrypted in encrypted rooms)"},
		{":avatar <image path>", "set my avatar"},
		{":accept [room]", "join a pending invite"},
		{":reject [room]", "decline a pending invite"},
		{":invites", "list pending invites"},
		{":recover <recovery key>", "verify this device and load old room keys from backup"},
		{":sponsor", "open github.com/sponsors/vic"},
		{":docs", "open the documentation (mtx.oeiuwq.com)"},
		{":mute [room]", "no notifications from this room (all devices)"},
		{":unmute [room]", "notifications from this room again"},
	}},
}

// helpText renders helpSections with the key column aligned.
func helpText() string {
	head := title.Render("mtx") + dimmed.Render("  Matrix client: & rooms, threads, end-to-end encryption")
	rows := []string{head, ""}
	for _, s := range helpSections {
		rows = append(rows, helpHead.Render(s.head))
		for _, k := range s.keys {
			rows = append(rows, "  "+shortcut.Width(20).Render(k[0])+k[1])
		}
		rows = append(rows, "")
	}
	width := 0
	for _, r := range rows {
		width = max(width, lipgloss.Width(r))
	}
	btns := renderHelpButtons()
	rows[0] = head + strings.Repeat(" ", max(width-lipgloss.Width(head)-lipgloss.Width(btns), 1)) + btns
	return strings.Join(append(rows, keyHint("ctrl+c", "quit", "any key", "close help")), "\n")
}

// mentionRE is an @nick in text (D:258); teammate nicks carry @session.
var mentionRE = regexp.MustCompile(`@[A-Za-z0-9_.-]+(@[A-Za-z0-9_.-]+)?`)

// mentions renders s in st with each @nick in that nick's color, bold.
func mentions(s string, st lipgloss.Style) string {
	var b strings.Builder
	last := 0
	for _, m := range mentionRE.FindAllStringIndex(s, -1) {
		if m[0] > last {
			b.WriteString(st.Render(s[last:m[0]]))
		}
		b.WriteString(nickStyle(s[m[0]+1 : m[1]]).Bold(true).Render(s[m[0]:m[1]]))
		last = m[1]
	}
	if last < len(s) {
		b.WriteString(st.Render(s[last:]))
	}
	return b.String()
}

// wrapFunc wraps one line to rows of a width: wrapLine, or wrapStamped with s on.
type wrapFunc func(model.Line, int) []string

// stampRE is the "2006-01-02 15:04 " that wrapStamped puts before a nick.
var stampRE = regexp.MustCompile(`^\d{4}-\d\d-\d\d \d\d:\d\d `)

// wrapStamped is wrapLine after a short "2006-01-02 15:04 " time; later rows are indented under it.
func wrapStamped(l model.Line, width int) []string {
	st := l.At.Format("2006-01-02 15:04") + " "
	rs := wrapLine(l, max(width-len(st), 1))
	for i := range rs {
		if i == 0 {
			rs[i] = st + rs[i]
		} else {
			rs[i] = strings.Repeat(" ", len(st)) + rs[i]
		}
	}
	return rs
}

// sidePanes is what the channel and nick panes show: rows in order (filtered when
// focused), which pane has focus, its selected row and the query typed in it.
type sidePanes struct {
	chans, nicks     []string
	focus            paneFocus
	sel              int
	nickSel          int // row of the selected nick (-1 none), labels are not selectable
	q                string
	heads            []model.Thread // thread heads of the current channel (top rows of the chat column)
	thread           string         // selected head root
	unread           []int          // per head: live replies not yet read (same order as heads)
	mainUnread       int            // main-thread lines not on screen (the main thread row)
	mainLast         string         // the main thread's newest line, "nick: text"
	mainLastAt       time.Time      // time of the main thread's newest line
	readAt           time.Time      // own lines at or before it are read by someone
	user             string         // the local nick
	jump             bool           // f: show line labels in the lead column
	threadTop        int            // first shown row of the threads box (its main thread is the last row)
	chanTop, nickTop int            // first shown row of the channel and nick panes
}

// title is a side pane's title: "▸name query" while it has focus.
func (p sidePanes) title(name string, f paneFocus) string {
	if p.focus != f {
		return name
	}
	return strings.TrimSpace("▸" + name + " " + p.q)
}

// toastCards is how many toasts show at once (the newest).
const toastCards = 3

// overlayToasts draws the newest toasts as cards at the right edge of screen, the
// lowest card's bottom on row bottom (vic 2026-10-23, after jjui flash messages).
func overlayToasts(screen string, w, bottom int, ts []toast) string {
	if len(ts) == 0 || w < minWide {
		return screen
	}
	rows := strings.Split(screen, "\n")
	maxW := max(w/2, 20)
	for i := len(ts) - 1; i >= max(len(ts)-toastCards, 0) && bottom >= 0; i-- {
		color := lipgloss.Color("2")
		if ts[i].err {
			color = lipgloss.Color("9")
		}
		card := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(color).Padding(0, 1).
			Width(min(lipgloss.Width(ts[i].text)+4, maxW)).Render(ts[i].text)
		cl := strings.Split(card, "\n")
		cw := lipgloss.Width(cl[0])
		x := max(w-cw-1, 0) // one column from the right edge
		for j := len(cl) - 1; j >= 0 && bottom >= 0; j-- {
			if bottom < len(rows) {
				rows[bottom] = ansi.Cut(rows[bottom], 0, x) + cl[j] + ansi.Cut(rows[bottom], x+cw, w)
			}
			bottom--
		}
	}
	return strings.Join(rows, "\n")
}

// overlaySuggest draws the completion row in a small box over the screen, its
// bottom border on row bottom, from column 1: it floats above the prompt
// (vic 2026-10-23), so no layout row moves.
func overlaySuggest(screen string, w, bottom int, sugg []string) string {
	if len(sugg) == 0 || bottom < 2 {
		return screen
	}
	box := strings.Split(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("5")).
		Padding(0, 1).Render(strings.Join(sugg, "\n")), "\n")
	rows := strings.Split(screen, "\n")
	bw := lipgloss.Width(box[0])
	for j := len(box) - 1; j >= 0 && bottom >= 0; j-- {
		if bottom < len(rows) {
			rows[bottom] = ansi.Cut(rows[bottom], 0, 1) + box[j] + ansi.Cut(rows[bottom], 1+bw, w)
		}
		bottom--
	}
	return strings.Join(rows, "\n")
}

// hitStyle marks the text a ctrl+g grep or / search matched: a background, not inverted.
var hitStyle = lipgloss.NewStyle().Background(lipgloss.Color("3")).Foreground(lipgloss.Color("0"))

// markHits is markdown with the case-insensitive matches of q in hitStyle.
func markHits(s string, st lipgloss.Style, code bool, q string) string {
	ls, lq := strings.ToLower(s), strings.ToLower(q)
	if q == "" || len(ls) != len(s) || len(lq) != len(q) { // lowercasing changed byte offsets: no highlight
		return markdown(s, st, code)
	}
	var b strings.Builder
	last := 0
	for {
		j := strings.Index(ls[last:], lq)
		if j < 0 {
			break
		}
		j += last
		b.WriteString(markdown(s[last:j], st, code))
		b.WriteString(hitStyle.Render(s[j : j+len(q)]))
		last = j + len(q)
	}
	b.WriteString(markdown(s[last:], st, code))
	return b.String()
}

// gutterBar marks the current message in the lines pane's lead column.
var gutterBar = lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Render("▌") // the cursor color

// frameRule is a horizontal frame line of width w with text in it: "─ text ───".
func frameRule(text string, w int) string {
	s := "─ " + text + " "
	return s + strings.Repeat("─", max(w-lipgloss.Width(s), 0))
}

// borderGlyphs holds each border character rendered in border and in focusBorder: a frame draws
// four per row, and the style never changes.
var borderGlyphs = struct {
	mu sync.Mutex
	m  map[[2]string]string
}{m: map[[2]string]string{}}

// borderGlyph is s in focusBorder (focused) or border, rendered once per process.
func borderGlyph(focused bool, s string) string {
	k := [2]string{"", s}
	if focused {
		k[0] = "f"
	}
	borderGlyphs.mu.Lock()
	defer borderGlyphs.mu.Unlock()
	if g, ok := borderGlyphs.m[k]; ok {
		return g
	}
	st := border
	if focused {
		st = focusBorder
	}
	g := st.Render(s)
	borderGlyphs.m[k] = g
	return g
}

// readMark is the lead column of an own line another member has read.
var readMark = dimmed.Render("✓")

// lead is the one-column lead of scrollback row k: readMark on the first row of an own line that was
// read, else a space.
func (p sidePanes) lead(of []int, shown []model.Line, k int) string {
	if p.jump && k >= 0 && k < len(of) && (k == 0 || of[k] != of[k-1]) {
		return jumpLabel(of, k)
	}
	if k < 0 || k >= len(of) || (k > 0 && of[k] == of[k-1]) {
		return " "
	}
	l := shown[of[k]]
	if p.user != "" && l.Nick == p.user && !p.readAt.IsZero() && !l.At.After(p.readAt) {
		return readMark
	}
	return " "
}

// cmdDesc is the help description of command word w, from the COMMANDS help section.
func cmdDesc(w string) string {
	for _, s := range helpSections {
		for _, k := range s.keys {
			for _, f := range strings.Fields(k[0]) {
				if f == ":"+w {
					return k[1]
				}
			}
		}
	}
	return ""
}

// jumpLabel is the label of row k's line: its position among the lines that start on screen.
func jumpLabel(of []int, k int) string {
	n := 0
	for i := 0; i < k; i++ {
		if i == 0 || of[i] != of[i-1] {
			n++
		}
	}
	if n >= len(jumpKeys) {
		return " "
	}
	return alert.Render(string(jumpKeys[n]))
}
