package ui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"mtx/internal/model"
)

// groupedRoom is a 160x40 app on room &r, whose nick pane groups boss, mod1 and pal under labels.
func groupedRoom(t *testing.T) App {
	var log []sent
	next, _ := newApp(t, &log, nil).Update(RoomsMsg{"&r"})
	a := next.(App).focusName("&r")
	a.m, _ = a.m.WithRoomMembers("&r", []model.Member{{Nick: "boss", Level: 100}, {Nick: "mod1", Level: 50}, {Nick: "pal", Level: 0}})
	return mouse(t, a, tea.WindowSizeMsg{Width: 160, Height: 40})
}

// TestClickGroupedNick: a click on a nick's row mentions that nick, not the nick at the same index of the bare list.
func TestClickGroupedNick(t *testing.T) {
	a := groupedRoom(t)
	rows, _ := a.nickRows()
	for _, nick := range []string{"boss", "mod1", "pal"} {
		y := slices.IndexFunc(rows, func(r railRow) bool { return strings.HasSuffix(r.text, nick) }) + 1 // row 0 is the frame title
		next, _ := clickUp(a, a.w-nickW-1, y)
		if b := next.(App); b.in.Value() != "@"+nick+" " {
			t.Errorf("click on the %s row (y %d): input %q", nick, y, b.in.Value())
		}
	}
	label := slices.IndexFunc(rows, func(r railRow) bool { return r.label }) + 1
	next, _ := clickUp(a, a.w-nickW-1, label)
	if b := next.(App); b.in.Value() != "" { // NEG: a label names no one
		t.Errorf("click on a group label: input %q", b.in.Value())
	}
}

// TestClickScrolledChannel: a click on a channel row of a scrolled pane opens the channel drawn there.
func TestClickScrolledChannel(t *testing.T) {
	a := mouse(t, scrollApp(t), tea.WindowSizeMsg{Width: 160, Height: 40}) // channels #a #b #c
	a.ptop[focusChans] = 1                                                 // the pane starts at #b
	next, _ := clickUp(a, 2, 1)
	if got := next.(App).m.Current().Name; got != "#b" {
		t.Fatalf("click on the first row of a pane scrolled by one: %s, want #b", got)
	}
}

// shown is rows as the pane draws them, mark then text, comma separated.
func shown(rows []railRow) string {
	var out []string
	for _, r := range rows {
		out = append(out, r.mark+r.text)
	}
	return strings.Join(out, ",")
}
