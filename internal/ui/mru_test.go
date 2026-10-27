package ui

import (
	"slices"
	"strings"
	"testing"

	"mtx/internal/model"
)

// TestRecentPickerChatsMRU: the recent picker lists chats most recently focused first.
func TestRecentPickerChatsMRU(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a, _ = press(t, a, "l") // #b
	a, _ = press(t, a, "l") // #c
	a, _ = press(t, a, "h") // #b
	if cur(a) != "#b" {
		t.Fatalf("focus %q want #b", cur(a))
	}
	a = a.openPick(pickRecent)
	got := a.pickSource()
	want := []string{"#b", "#c", "#a"}
	if !slices.Equal(got, want) {
		t.Fatalf("recent %q want %q", got, want)
	}
}

// TestRecentPickerNoSwitchKeepsOrder: no switches keeps list order, nothing dropped or doubled.
func TestRecentPickerNoSwitchKeepsOrder(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	got := a.openPick(pickRecent).pickSource()
	all := a.chanNames()
	if len(got) != len(all) {
		t.Fatalf("recent %q want %d rows", got, len(all))
	}
	seen := map[string]bool{}
	for _, n := range got {
		if seen[n] || !slices.Contains(all, n) {
			t.Fatalf("recent %q has dup or unknown %q", got, n)
		}
		seen[n] = true
	}
	if !slices.Equal(got, all) {
		t.Fatalf("recent %q want list order %q", got, all)
	}
}

// TestRecentPickerBackToFirst: walking #a, #b, #c, #b, #a lists #a, #b, #c: the latest visit of each wins.
func TestRecentPickerBackToFirst(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a, _ = press(t, a, "l")
	a, _ = press(t, a, "l")
	a, _ = press(t, a, "h")
	a, _ = press(t, a, "h")
	if cur(a) != "#a" {
		t.Fatalf("focus %q want #a", cur(a))
	}
	got := a.openPick(pickRecent).pickSource()
	if want := []string{"#a", "#b", "#c"}; !slices.Equal(got, want) {
		t.Fatalf("recent %q want %q", got, want)
	}
}

// TestRecentChansNeverVisitedKeepListOrder: visited chats come first; never-visited ones follow in list order.
func TestRecentChansNeverVisitedKeepListOrder(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.mru = []string{"#c"}
	if got, want := a.recentChans(), []string{"#c", "#a", "#b"}; !slices.Equal(got, want) {
		t.Fatalf("recentChans %q want %q", got, want)
	}
	a.mru = nil
	if got, want := a.recentChans(), a.chanNames(); !slices.Equal(got, want) {
		t.Fatalf("recentChans %q want list order %q", got, want)
	}
}

// TestRecentChansDropsRemovedRoom: a visited room that a rooms update later removes is no longer listed.
func TestRecentChansDropsRemovedRoom(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(RoomsMsg{"&x", "&y"})
	a = next.(App).focusName("&x")
	if !slices.Contains(a.recentChans(), "&x") || a.recentChans()[0] != "&x" {
		t.Fatalf("recent %q want &x first", a.recentChans())
	}
	next, _ = a.Update(RoomsMsg{"&y"})
	a = next.(App)
	got := a.recentChans()
	if slices.Contains(got, "&x") {
		t.Fatalf("recent %q still lists removed &x", got)
	}
	if !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(a.chanNames()))) {
		t.Fatalf("recent %q want the same chats as %q", got, a.chanNames())
	}
}

// recentThreadRoots is the thread rows of the recent picker as roots, in order.
func recentThreadRoots(a App) []string {
	var out []string
	for _, h := range a.recentThreads(a.threads[a.m.Current().Name]) {
		out = append(out, h.Root)
	}
	return out
}

// TestRecentThreadsMRU: opened threads come first, latest opened first; never-opened follow newest reply first.
func TestRecentThreadsMRU(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	old, news, mid := threadsShuffled()
	a.threads = map[string][]model.Thread{"#a": {old, news, mid}}
	a = a.selectThread("$old")
	a = a.selectThread("$mid")
	a = a.selectThread("$old").selectThread("$mid")
	if got, want := recentThreadRoots(a), []string{"$mid", "$old", "$new"}; !slices.Equal(got, want) {
		t.Fatalf("threads %q want %q", got, want)
	}
	rows := a.openPick(pickRecent).pickSource()
	chats := len(a.chanNames())
	if len(rows) != chats+3 || rows[chats] != threadLabel(mid) || rows[chats+1] != threadLabel(old) || rows[chats+2] != threadLabel(news) {
		t.Fatalf("picker rows %q", rows)
	}
}

// TestRecentThreadsNeverVisitedKeepOrder: with no visit the rows are newest reply first, none dropped or doubled.
func TestRecentThreadsNeverVisitedKeepOrder(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	old, news, mid := threadsShuffled()
	a.threads = map[string][]model.Thread{"#a": {old, news, mid}}
	if got, want := recentThreadRoots(a), []string{"$new", "$mid", "$old"}; !slices.Equal(got, want) {
		t.Fatalf("threads %q want %q", got, want)
	}
	a = a.selectThread("")
	if got, want := recentThreadRoots(a), []string{"$new", "$mid", "$old"}; !slices.Equal(got, want) {
		t.Fatalf("after closing a thread %q want %q", got, want)
	}
}

// TestRecentThreadsRemovedHeadAbsent: a visited thread that leaves the channel's heads is not listed.
func TestRecentThreadsRemovedHeadAbsent(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	old, news, mid := threadsShuffled()
	a.threads = map[string][]model.Thread{"#a": {old, news, mid}}
	a = a.selectThread("$old")
	a.threads = map[string][]model.Thread{"#a": {news, mid}}
	if got, want := recentThreadRoots(a), []string{"$new", "$mid"}; !slices.Equal(got, want) {
		t.Fatalf("threads %q want %q", got, want)
	}
}

// TestRecentChansDMFirst: a room used as a DM, focused through focusName, sorts first.
func TestRecentChansDMFirst(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(RoomsMsg{"&dm"})
	a = next.(App)
	if a.recentChans()[0] == "&dm" {
		t.Fatalf("recent %q: &dm unvisited must not lead", a.recentChans())
	}
	a = a.focusName("&dm")
	got := a.recentChans()
	if got[0] != "&dm" || len(got) != len(a.chanNames()) {
		t.Fatalf("recent %q want &dm first and %d rows", got, len(a.chanNames()))
	}
	if n := len(slices.Compact(slices.Sorted(slices.Values(got)))); n != len(got) {
		t.Fatalf("recent %q has duplicates", got)
	}
}

// TestRecentPickerEnterOnDMFocusesIt: enter on a "@nick" DM row of the recent picker focuses that chat; it does not insert a mention.
func TestRecentPickerEnterOnDMFocusesIt(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(RoomsMsg{"@nick"})
	a = next.(App)
	a = a.openPick(pickRecent)
	a = typeText(t, a, "@nick")
	a, _ = press(t, a, "enter")
	if cur(a) != "@nick" {
		t.Fatalf("focus %q want @nick", cur(a))
	}
	if len(a.mru) == 0 || a.mru[0] != "@nick" {
		t.Fatalf("mru %q want @nick first", a.mru)
	}
	if strings.Contains(a.in.Value(), "@nick ") {
		t.Fatalf("input %q holds a mention", a.in.Value())
	}
	if a.mode == Insert {
		t.Fatalf("mode Insert, want it left alone")
	}
}

// TestRecentPickerEnterOnChatRowFocusesIt: enter on an ordinary chat row of the recent picker still focuses it.
func TestRecentPickerEnterOnChatRowFocusesIt(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a = a.openPick(pickRecent)
	a = typeText(t, a, "#c")
	a, _ = press(t, a, "enter")
	if cur(a) != "#c" {
		t.Fatalf("focus %q want #c", cur(a))
	}
	if len(a.mru) == 0 || a.mru[0] != "#c" {
		t.Fatalf("mru %q want #c first", a.mru)
	}
	if a.in.Value() != "" || a.mode == Insert {
		t.Fatalf("input %q mode %v: a chat row must not write the input", a.in.Value(), a.mode)
	}
}

// TestChanPickerEnterOnDMFocusesIt (mtx-z2h): ctrl+o Enter on a "@nick" room row focuses that DM, like any chat row.
func TestChanPickerEnterOnDMFocusesIt(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(RoomsMsg{"@nick"})
	a = next.(App)
	a = a.openPick(pickChans)
	a = typeText(t, a, "@nick")
	a, _ = press(t, a, "enter")
	if cur(a) != "@nick" {
		t.Fatalf("focus %q want @nick", cur(a))
	}
	if len(a.mru) == 0 || a.mru[0] != "@nick" {
		t.Fatalf("mru %q want @nick first", a.mru)
	}
	if a.in.Value() != "" || a.mode == Insert {
		t.Fatalf("input %q mode %v: no mention insert", a.in.Value(), a.mode)
	}
}
