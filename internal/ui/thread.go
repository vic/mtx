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

// nextThread (T) toggles between the main thread and the last thread opened. From main it goes to
// the last thread; with none opened yet, to the first head.
func (a App) nextThread() App {
	heads := a.threads[a.m.Current().Name]
	switch {
	case a.thread != "":
		return a.selectThread("")
	case a.lastThread != "" && a.hasHead(a.lastThread):
		return a.selectThread(a.lastThread)
	case len(heads) > 0:
		return a.selectThread(heads[0].Root)
	}
	return a
}

// hasHead reports whether root is a head in the current channel.
func (a App) hasHead(root string) bool {
	_, ok := a.headOf(root)
	return ok
}

// inThread reports whether l belongs to the selected thread (the head itself included).
func (a App) inThread(l model.Line) bool {
	return a.thread == "" || l.ID == a.thread || l.Thread == a.thread
}

// threadRowsMax is how many rows the threads box shows (main thread plus heads).
const threadRowsMax = 8

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
	if root != "" {
		a.lastThread = root
	}
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

// mainThreadLabel is the first row of the threads box: the channel's main chat (no thread selected).
const mainThreadLabel = "main thread"

// threadBox is the threads box above the chat in the wide layout: its rows, and the separator
// line after it. Without threads (or narrow) there is no box.
func threadBox(p sidePanes, wide bool) (rows, sep int) {
	if !wide || len(p.heads) == 0 {
		return 0, 0
	}
	return min(len(p.heads)+1, threadRowsMax), 1
}

// midTitle is the title of the middle column's top row: the threads box when it shows, else the channel.
func midTitle(p sidePanes, name string) string {
	if len(p.heads) > 0 {
		return p.title("threads", focusThreads)
	}
	return name
}

// threadsRow is row i of the threads box, width w: row 0 is the main thread, then the heads.
func threadsRow(p sidePanes, i, w int) string {
	focused := p.focus == focusThreads && p.sel == i
	if i == 0 {
		return mainThreadRow(p.thread == "" || focused, p.mainUnread, w)
	}
	if i-1 >= len(p.heads) {
		return fit(cell, "", w)
	}
	u := 0
	if i-1 < len(p.unread) {
		u = p.unread[i-1]
	}
	h := p.heads[i-1]
	return threadRow(h, h.Root == p.thread || focused, u, w)
}

// mainThreadRow is the main thread row: marked when selected.

// nextPane is the pane Tab moves to: chat, channels, nicks, then the threads box when it shows.
func (a App) nextPane() paneFocus {
	order := []paneFocus{focusChat, focusChans, focusNicks}
	if len(a.threads[a.m.Current().Name]) > 0 {
		order = append(order, focusThreads)
	}
	for i, f := range order {
		if f == a.focus {
			return order[(i+1)%len(order)]
		}
	}
	return focusChat
}

// mainThreadRow is the main thread row: marked when selected, with its unread count.
func mainThreadRow(selected bool, unread, w int) string {
	mark := "  "
	if selected {
		mark = "▸ "
	}
	label := mark + mainThreadLabel
	if unread > 0 {
		return fit(cell, label, w-3) + fit(cell, unreadMark.Render(strconv.Itoa(unread)), 3)
	}
	return fit(cell, label, w)
}

// bumpMain counts one more main-thread line in channel ch.
func bumpMain(m map[string]int, ch string) map[string]int {
	if m == nil {
		m = map[string]int{}
	}
	m[ch]++
	return m
}

// threadState is the thread name the modeline shows: "" for main, else the thread's head nick.
func threadState(root string) string {
	if root == "" {
		return ""
	}
	return "open"
}
