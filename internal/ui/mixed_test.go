package ui

import (
	"slices"
	"testing"
	"time"

	"mtx/internal/model"
)

// mixedApp has the thread A in #a, the DM "@c" and the rooms of newApp.
func mixedApp(t *testing.T) (App, model.Thread) {
	t.Helper()
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(RoomsMsg{"@c"})
	a = next.(App)
	old, _, _ := threadsShuffled()
	a.threads = map[string][]model.Thread{"#a": {old}}
	return a, old
}

func rowIndex(rows []string, row string) int { return slices.Index(rows, row) }

// TestRecentMixedOrder (mtx-yrr a): read thread A, then channel B, then DM C: the rows lead C, B, A.
func TestRecentMixedOrder(t *testing.T) {
	a, th := mixedApp(t)
	a = a.selectThread(th.Root).focusName("#b").focusName("@c")
	rows := a.openPick(pickRecent).pickSource()
	want := []string{"@c", "#b", threadLabel(th)}
	if len(rows) < len(want) || !slices.Equal(rows[:len(want)], want) {
		t.Fatalf("rows %q want lead %q", rows, want)
	}
}

// TestRecentOtherChannelThreadListed (mtx-yrr b): a visited thread of another channel appears.
func TestRecentOtherChannelThreadListed(t *testing.T) {
	a, th := mixedApp(t)
	a = a.selectThread(th.Root).focusName("#b")
	if rowIndex(a.openPick(pickRecent).pickSource(), threadLabel(th)) < 0 {
		t.Fatalf("rows %q lack the thread of #a while #b is current", a.pickSource())
	}
}

// TestRecentEnterOnOtherChannelThread (mtx-yrr c): enter on a thread row of another channel focuses that channel and selects the thread.
func TestRecentEnterOnOtherChannelThread(t *testing.T) {
	a, th := mixedApp(t)
	a = a.selectThread(th.Root).focusName("#b")
	a = a.openPick(pickRecent)
	a = typeText(t, a, threadLabel(th))
	a, _ = press(t, a, "enter")
	if cur(a) != "#a" || a.thread != th.Root {
		t.Fatalf("focus %q thread %q want #a %q", cur(a), a.thread, th.Root)
	}
}

// TestRecentNeverVisitedTodaysOrder (mtx-yrr d): with no visit the rows are the chats, then this channel's threads newest reply first.
func TestRecentNeverVisitedTodaysOrder(t *testing.T) {
	a, _ := mixedApp(t)
	old, news, mid := threadsShuffled()
	a.threads = map[string][]model.Thread{"#a": {old, news, mid}}
	a.visits = nil
	want := append(a.chanNames(), threadLabel(news), threadLabel(mid), threadLabel(old))
	if got := a.openPick(pickRecent).pickSource(); !slices.Equal(got, want) {
		t.Fatalf("rows %q want %q", got, want)
	}
}

// TestRecentRevisitMovesToTopOnce (mtx-yrr e): a revisit moves a row to the top without duplicating it.
func TestRecentRevisitMovesToTopOnce(t *testing.T) {
	a, th := mixedApp(t)
	a = a.selectThread(th.Root).focusName("#b").focusName("@c").focusName("#b").selectThread("")
	a = a.focusName("#a").selectThread(th.Root).focusName("#b")
	rows := a.openPick(pickRecent).pickSource()
	if rows[0] != "#b" {
		t.Fatalf("rows %q want #b first", rows)
	}
	seen := map[string]int{}
	for _, r := range rows {
		seen[r]++
		if seen[r] > 1 {
			t.Fatalf("rows %q repeat %q", rows, r)
		}
	}
	if i, j := rowIndex(rows, threadLabel(th)), rowIndex(rows, "@c"); i < 0 || j < 0 || i > j {
		t.Fatalf("rows %q want the thread (read later) above @c", rows)
	}
}

// TestRecentRemovedRoomAndThreadDrop (mtx-yrr f): a removed room or thread drops out.
func TestRecentRemovedRoomAndThreadDrop(t *testing.T) {
	a, th := mixedApp(t)
	a = a.selectThread(th.Root).focusName("@c")
	next, _ := a.Update(RoomsMsg{})
	a = next.(App)
	a.threads = map[string][]model.Thread{"#a": {}}
	rows := a.openPick(pickRecent).pickSource()
	if rowIndex(rows, "@c") >= 0 || rowIndex(rows, threadLabel(th)) >= 0 {
		t.Fatalf("rows %q still list the removed room or thread", rows)
	}
}

// TestRecentEnterOnCurrentChannelThread: enter on a thread row of the current channel still selects it there.
func TestRecentEnterOnCurrentChannelThread(t *testing.T) {
	a, th := mixedApp(t)
	a = a.openPick(pickRecent)
	a = typeText(t, a, threadLabel(th))
	a, _ = press(t, a, "enter")
	if cur(a) != "#a" || a.thread != th.Root {
		t.Fatalf("focus %q thread %q want #a %q", cur(a), a.thread, th.Root)
	}
}

// TestRecentEqualLabelDecodesByPlace: a chat and a thread with the same label are told apart by their row, newest visit first.
func TestRecentEqualLabelDecodesByPlace(t *testing.T) {
	const room = "&x: hi"
	same := model.Thread{Root: "$same", Nick: "&x", Text: "hi", Count: 1, At: time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)}
	if threadLabel(same) != room {
		t.Fatalf("label %q want %q", threadLabel(same), room)
	}
	setup := func() App {
		a, _ := mixedApp(t)
		next, _ := a.Update(RoomsMsg{"@c", room})
		a = next.(App)
		a.threads = map[string][]model.Thread{"#a": {same}}
		return a
	}
	pickTop := func(a App) App {
		a = a.openPick(pickRecent)
		a = typeText(t, a, room)
		a, _ = press(t, a, "enter")
		return a
	}
	a := pickTop(setup().focusName(room).focusName("#a").selectThread("$same"))
	if cur(a) != "#a" || a.thread != "$same" {
		t.Fatalf("thread visited last: focus %q thread %q want #a $same", cur(a), a.thread)
	}
	a = pickTop(setup().selectThread("$same").selectThread("").focusName(room).focusName("#a"))
	if cur(a) != room || a.thread != "" {
		t.Fatalf("room visited after the thread: focus %q thread %q want %q and no thread", cur(a), a.thread, room)
	}
}
