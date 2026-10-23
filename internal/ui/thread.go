package ui

import (
	"fmt"
	"strconv"

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
		a = a.selectThread("")
		return a
	}
	i := -1
	for k, h := range heads {
		if h.Root == a.thread {
			i = k
		}
	}
	if i+1 >= len(heads) {
		a = a.selectThread("")
	} else {
		a = a.selectThread(heads[i+1].Root)
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

// threadRow is one head: "▸" when selected, the sender and text, the reply count, and the unread
// count when there is one. A head with nothing unread is faded.
func threadRow(h model.Thread, selected bool, unread, w int) string {
	mark := "  "
	if selected {
		mark = "▸ "
	}
	label := fmt.Sprintf("%s%s: %s (%d)", mark, h.Nick, h.Text, h.Count)
	if unread == 0 {
		row := fit(cell, label, w)
		if selected {
			return row
		}
		return faint.Render(row)
	}
	// the channel unread marker: the count, styled as channels style theirs
	return fit(cell, label, w-3) + fit(cell, unreadMark.Render(strconv.Itoa(unread)), 3)
}

// unreadAll is the unread count of each head of the current channel, in head order.
func (a App) unreadAll() []int {
	heads := a.threads[a.m.Current().Name]
	out := make([]int, len(heads))
	for i, h := range heads {
		out[i] = a.unreadOf(h)
	}
	return out
}

// threadLabel is a head as the thread pane and the Ctrl+O picker list it.
func threadLabel(h model.Thread) string {
	return fmt.Sprintf("thread: %s: %s (%d)", h.Nick, h.Text, h.Count)
}

// threadAt is the head that label names in the current channel.
func (a App) threadAt(label string) (model.Thread, bool) {
	for _, h := range a.threads[a.m.Current().Name] {
		if threadLabel(h) == label {
			return h, true
		}
	}
	return model.Thread{}, false
}

// selectThread shows root in the chat pane ("" = main chat) and marks it read.
func (a App) selectThread(root string) App {
	a.thread, a.hit, a.offs = root, -1, nil
	if h, ok := a.headOf(root); ok {
		if a.seenLive == nil {
			a.seenLive = map[string]int{}
		}
		a.seenLive[root] = h.Live
	}
	return a
}

// headOf is the head with root in the current channel.
func (a App) headOf(root string) (model.Thread, bool) {
	for _, h := range a.threads[a.m.Current().Name] {
		if h.Root == root {
			return h, true
		}
	}
	return model.Thread{}, false
}

// unreadOf is the live replies of h not yet read. A head seen for the first time counts as read.
func (a App) unreadOf(h model.Thread) int {
	if h.Root == a.thread {
		return 0
	}
	if s, ok := a.seenLive[h.Root]; ok {
		return max(h.Live-s, 0)
	}
	return 0
}

// unreadMark is the channel unread count style (the number next to a channel with unread lines).
var unreadMark = unread

// lastWithID is the message at index i of the current channel's shown lines, or the newest one
// before it when that line has no event ID (local echoes have none). i < 0 means the newest.
func (a App) lastWithID(i int) (model.Line, bool) {
	ls := a.lines(a.m.Current())
	if i < 0 || i >= len(ls) {
		i = len(ls) - 1
	}
	for ; i >= 0; i-- {
		if ls[i].ID != "" {
			return ls[i], true
		}
	}
	return model.Line{}, false
}
