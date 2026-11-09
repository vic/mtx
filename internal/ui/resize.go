package ui

// Border resize follows tuios, internal/input/border_resize.go (github.com/Gaurav-Gosain/tuios): a left
// press on a pane border grabs it on the exact cell (a pointer needs no slop), the border then follows the
// pointer, and each pane keeps a minimum width.
// MIT License, Copyright (c) 2025 Gaurav Gosain.

// The side pane inner widths. A border drag resizes them; like zen they hold for the whole session.
var (
	chanW = 15 // channel list pane
	nickW = 10 // nick pane
)

// minPaneW is the narrowest side pane: a space, four name columns and the three marker columns.
const minPaneW = 8

// paneEdge is a border a drag can move.
type paneEdge int

const (
	edgeChans paneEdge = iota // between the channel pane and the chat
	edgeNicks                 // between the chat and the nick pane
)

// edgeDrag moves a pane border with the pointer.
type edgeDrag struct{ edge paneEdge }

func (edgeDrag) dragging() {}

// edgeAt is the border on the screen cell (x, y), if one is there: a column between the frame's top and
// bottom border rows.
func (a App) edgeAt(x, y int) (paneEdge, bool) {
	_, input, _ := a.inputLine()
	if !wide(a.w, a.h) || y < 0 || y > paneRows(a.w, a.h, len(inputRows(input, a.w, a.h)))+1 {
		return 0, false
	}
	switch x {
	case chanW + 1:
		return edgeChans, true
	case a.w - nickW - 2:
		return edgeNicks, true
	}
	return 0, false
}

// pressEdge answers a left press on a pane border: the drag that follows moves it.
func (a App) pressEdge(x, y int) (App, bool) {
	e, ok := a.edgeAt(x, y)
	if ok {
		a.sel, a.drag = nil, edgeDrag{e}
	}
	return a, ok
}

// dragEdge puts border e on column x, clamped so both side panes keep minPaneW and the chat keeps a nick
// column. A width change redraws every cached row and cell, which were cut to the old widths.
func (a App) dragEdge(e paneEdge, x int) App {
	c, n := chanW, nickW
	switch e {
	case edgeChans:
		c = min(max(x-1, minPaneW), a.w-nickW-4-nickCol)
	case edgeNicks:
		n = min(max(a.w-2-x, minPaneW), a.w-chanW-4-nickCol)
	}
	if c != chanW || n != nickW {
		chanW, nickW = c, n
		clearRenderCaches()
	}
	return a
}
