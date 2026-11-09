package ui

// The scrollbar arithmetic is ported from tuios, internal/app/render_scrollbar.go and
// internal/input/mouse.go (github.com/Gaurav-Gosain/tuios).
// MIT License, Copyright (c) 2025 Gaurav Gosain.

// barUnits is how many thumb units make a row: a thumb end can land mid-cell as ▀ or ▄, so the bar
// carries twice the resolution of one column (tuios "track" style, after opentui's Slider).
const barUnits = 2

// barGeom is the chat scrollbar's arithmetic: a page of rows with back rows above it. It holds no
// offset, so a drag keeps using it after the thumb reaches the tail and the bar is hidden.
type barGeom struct{ page, back int }

// scrollBar is a bar that is drawn: it exists only while the chat is scrolled back (off > 0) over more
// rows than one page, and the page has room for a thumb with travel. At the tail the bar is chrome
// that answers nothing (tuios render_scrollbar.go:38-41).
type scrollBar struct {
	barGeom
	off int
}

// newBar is the bar for a page of rows, total rows in all, off rows back from the newest.
func newBar(page, total, off int) (scrollBar, bool) {
	g := barGeom{page, total - page}
	if off <= 0 || g.back <= 0 || page <= 2 {
		return scrollBar{}, false
	}
	return scrollBar{g, min(off, g.back)}, true
}

// travel is the thumb's size and its travel in units: the thumb is the page's share of all rows, at
// least one unit, and leaves at least one unit of travel.
func (g barGeom) travel() (thumb, travel int) {
	track := g.page * barUnits
	total := g.back + g.page
	thumb = max(min((track*g.page+total-1)/total, track-1), 1)
	return thumb, track - thumb
}

// roundDiv divides two non-negative counts, rounding halves up. Truncation biases every position toward
// the tail, so a thumb a hair off the oldest row would draw a whole row low.
func roundDiv(a, b int) int {
	if b <= 0 {
		return 0
	}
	return (2*a + b) / (2 * b)
}

// start is the thumb's leading edge in units for off rows back, 0 at the oldest row.
func (g barGeom) start(off int) int {
	_, travel := g.travel()
	if travel <= 0 || g.back <= 0 {
		return 0
	}
	return max(min(travel-roundDiv(off*travel, g.back), travel), 0)
}

// offAt inverts start for a thumb whose first cell is on page row r: the offset a drag holds, so the
// thumb lands where the pointer left it.
func (g barGeom) offAt(r int) int {
	_, travel := g.travel()
	if travel <= 0 {
		return g.back
	}
	s := min(max(r, 0), g.page-1) * barUnits
	s = min(s, travel)
	return max(min(roundDiv((travel-s)*g.back, travel), g.back), 0)
}

// barCell is what one page row of the bar shows.
type barCell int

const (
	barTrack barCell = iota // the border shows through
	barFull                 // █
	barUpper                // ▀: the thumb ends in this cell's upper half
	barLower                // ▄: the thumb starts in this cell's lower half
)

// cells is the bar's column, one cell per page row, and the page rows the thumb landed on: a press is
// measured against these, not against the arithmetic that drew them.
func (b scrollBar) cells() (cs []barCell, top, n int) {
	thumb, _ := b.travel()
	start := b.start(b.off)
	end := start + thumb
	cs = make([]barCell, b.page)
	top = -1
	for i := range cs {
		covered := min(end, (i+1)*barUnits) - max(start, i*barUnits)
		switch {
		case covered <= 0:
			continue
		case covered >= barUnits:
			cs[i] = barFull
		case start <= i*barUnits:
			cs[i] = barUpper
		default:
			cs[i] = barLower
		}
		if top < 0 {
			top = i
		}
		n++
	}
	return cs, max(top, 0), n
}

// barInk is each bar cell drawn in the thumb color (the focused border color): set by usePalette.
var barInk [barLower + 1]string

// chatBar is the chat scrollbar's geometry and place on screen: its column x and the screen row y of the
// first chat row (under the top border and the threads box).
func (a App) chatBar() (g barGeom, x, y int) {
	page, y := a.chatRows()
	return barGeom{page, rowCount(a.lines(a.m.Current()), textWidth(a.w, a.h), a.wrap()) - page}, a.w - nickW - 2, y
}

// grabBar answers a left press on the drawn scrollbar and starts a drag that holds the rows between the
// pointer and the thumb top. A press on the bare track first jumps the thumb under the pointer and takes
// the hold from where it landed, else the thumb leaps again on the first motion (tuios mouse.go:115-129).
func (a App) grabBar(x, y int) (App, bool) {
	if !wide(a.w, a.h) {
		return a, false
	}
	g, bx, by := a.chatBar()
	b, ok := newBar(g.page, g.page+g.back, a.off())
	if !ok || x != bx || y < by || y >= by+g.page {
		return a, false
	}
	_, top, n := b.cells()
	if r := y - by; r < top || r >= top+n {
		a = a.setOff(a.m.Current().Name, g.offAt(r-n/2))
		_, top, _ = scrollBar{g, a.off()}.cells()
	}
	hold := y - by - top
	a.drag = barDrag{hold}
	return a, true
}
