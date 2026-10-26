package ui

import (
	"fmt"
	"time"

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
	if root != "" && a.threadOpen != nil {
		a.threadOpen(a.m.Current().Name, root)
	}
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

// threadRows is the height of the current channel's threads box and its separator (0, 0 when none).
func (a App) threadRows() (rows, sep int) {
	return threadBox(sidePanes{heads: a.threads[a.m.Current().Name]}, wide(a.w, a.h))
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

// threadStampW is the width of a thread row's left column: time, then unread count.
const threadStampW = 16

// threadStamp is the left column of a thread row: the newest reply's time and its unread count.
func threadStamp(at time.Time, unread int) string {
	t := "           "
	if !at.IsZero() {
		t = at.Format("01-02 15:04")
	}
	u := "   "
	if unread > 0 {
		u = unreadMark.Render(fmt.Sprintf("%3d", unread))
	}
	return fit(cell, t+" "+u, threadStampW)
}

// threadRow is a head of the threads box: stamp, then "nick: text (replies)".
func threadRow(h model.Thread, selected bool, unread, w int) string {
	mark := "  "
	if selected {
		mark = "▸ "
	}
	body := fit(cell, fmt.Sprintf("%s%s: %s (%d)", mark, h.Nick, h.Text, h.Count), w-threadStampW)
	if unread == 0 && !selected {
		body = faint.Render(body)
	}
	return threadStamp(h.At, unread) + body
}

// mainThreadRow is the last row of the threads box: the channel's main chat, then its newest line
// (last, "nick: text") when there is one.
func mainThreadRow(selected bool, unread int, at time.Time, last string, w int) string {
	mark := "  "
	if selected {
		mark = "▸ "
	}
	text := mark + mainThreadLabel
	if last != "" {
		text += ": " + last
	}
	return threadStamp(at, unread) + fit(cell, text, w-threadStampW)
}

// threadsRow is row i of the threads box, width w: the heads, then the main thread last.
func threadsRow(p sidePanes, i, w int) string {
	j := p.threadTop + i
	focused := p.focus == focusThreads && p.sel == j
	if j == len(p.heads) {
		return mainThreadRow(p.thread == "" || focused, p.mainUnread, p.mainLastAt, p.mainLast, w)
	}
	if j > len(p.heads) {
		return fit(cell, "", w)
	}
	u := 0
	if j < len(p.unread) {
		u = p.unread[j]
	}
	h := p.heads[j]
	return threadRow(h, h.Root == p.thread || focused, u, w)
}

// threadTopOf is the first shown row of a threads box of n rows (heads plus the main thread):
// it starts at the bottom, so the main thread shows, and follows the selection sel.
func threadTopOf(n, sel int, focused bool, scroll int) int {
	vis := min(n, threadRowsMax)
	top := n - vis
	top = min(max(top+scroll, 0), n-vis) // the wheel moves the box; it starts at the bottom
	if focused {
		if sel < top {
			top = sel
		} else if sel >= top+vis {
			top = sel - vis + 1
		}
	}
	return top
}
