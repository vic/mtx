package ui

import (
	"fmt"

	"mtx/internal/model"
)

// ThreadsMsg sets the thread heads of channel Ch, newest last. Each head shows in the thread pane.
type ThreadsMsg struct {
	Ch    string
	Heads []model.Thread
}

// nextThread moves the thread selection: main chat, then each head in order, then main again.
func (a App) nextThread() App {
	heads := a.threads[a.m.Current().Name]
	if len(heads) == 0 {
		a.thread = ""
		return a
	}
	i := -1
	for k, h := range heads {
		if h.Root == a.thread {
			i = k
		}
	}
	if i+1 >= len(heads) {
		a.thread = ""
	} else {
		a.thread = heads[i+1].Root
	}
	a.hit, a.offs = -1, nil
	return a
}

// inThread reports whether l belongs to the selected thread (the head itself included).
func (a App) inThread(l model.Line) bool {
	return a.thread == "" || l.ID == a.thread || l.Thread == a.thread
}

// threadRowsMax is how many thread heads the top of the chat column shows.
const threadRowsMax = 3

// threadRowCount is how many rows the thread heads take in a column of n rows (0 with no heads).
func threadRowCount(heads []model.Thread, n int) int {
	return min(len(heads), threadRowsMax, max(n-1, 0))
}

// threadRow is one head: "▸" when selected, the root's sender and text, and the reply count.
func threadRow(h model.Thread, selected bool, w int) string {
	mark := "  "
	if selected {
		mark = "▸ "
	}
	return fit(cell, fmt.Sprintf("%s%s: %s (%d)", mark, h.Nick, h.Text, h.Count), w)
}
