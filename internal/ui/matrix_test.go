package ui

import (
	"charm.land/lipgloss/v2"
	"errors"

	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"

	"mtx/internal/model"
)

// TestTypingMarksRoomMember: a typing room member shows 💬, others idle.
func TestTypingMarksRoomMember(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(RoomsMsg{"&r"})
	next, _ = next.(App).Update(MembersMsg{Ch: "&r", Nicks: []string{"lib1", "lib2"}})
	a = next.(App).focusName("&r")
	a.m, _ = a.m.WithRoomMembers("&r", []model.Member{{Nick: "lib1"}, {Nick: "lib2"}})
	next, _ = a.Update(TypingMsg{Ch: "&r", Nicks: []string{"lib1"}})
	a = next.(App)
	if got := strings.Join(a.paneNicks(), ","); !strings.Contains(got, "💬lib1") || !strings.Contains(got, "💤lib2") {
		t.Fatalf("typing marks %q", got)
	}
}

// TestSeenNewLineInCurrentChannel: a live line in the focused channel is a seen event; old lines are not.
func TestSeenNewLineInCurrentChannel(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	if !newLineFor(LineMsg{Ch: "#a", L: model.Line{Text: "x"}}, "#a") {
		t.Fatal("live line in the channel must count as seen")
	}
	if newLineFor(LineMsg{Old: true, Ch: "#a"}, "#a") {
		t.Fatal("history is not seen")
	}
	if newLineFor(LineMsg{Ch: "#b"}, "#a") {
		t.Fatal("another channel's line is not this channel's")
	}
	var seen []string
	_ = a.WithSeen(func(ch string) error { seen = append(seen, ch); return nil })
	seenCmd(func(ch string) error { seen = append(seen, ch); return errors.New("ignored") }, "#a")()
	if len(seen) != 1 || seen[0] != "#a" {
		t.Fatalf("seen calls %v", seen)
	}
}

// TestNickGroupsByRole: in a room, admins come first under an 'admins' label, then mods, then members.
func TestNickGroupsByRole(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	next, _ := a.Update(RoomsMsg{"&r"})
	a = next.(App).focusName("&r")
	a.m, _ = a.m.WithRoomMembers("&r", []model.Member{{Nick: "boss", Level: 100}, {Nick: "mod1", Level: 50}, {Nick: "pal", Level: 0}})
	rows, _ := a.nickRows()
	var got []string
	for _, r := range rows {
		got = append(got, strings.TrimPrefix(ansi.Strip(r), " "))
	}
	s := strings.Join(got, "|")
	iAdm, iMod, iMem := strings.Index(s, "admins"), strings.Index(s, "mods"), strings.Index(s, "members")
	if iAdm < 0 || iMod < 0 || iMem < 0 || !(iAdm < strings.Index(s, "boss") && strings.Index(s, "boss") < iMod && iMod < strings.Index(s, "mod1") && strings.Index(s, "mod1") < iMem) {
		t.Fatalf("groups: %q", s)
	}
}

// TestHelpButtonsOpenLinks: a click on the repo button hits repoURL, on donate hits donateURL, and
// elsewhere nothing.
func TestHelpButtonsOpenLinks(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.help = true
	a.w, a.h = 120, 60 // tall enough that the first help row is on screen
	bl := strings.Split(helpBox(a.h, a.helpOff), "\n")
	bw := lipgloss.Width(bl[0])
	top, left := max((a.h-len(bl))/2, 0), max((a.w-bw)/2, 0)
	row := ansi.Strip(bl[1])
	x := left + lipgloss.Width(row[:strings.Index(row, "github:vic/mtx")]) + 1
	if u, ok := a.helpButtonAt(x, top+1); !ok || u != repoURL {
		t.Fatalf("repo button: %q %v", u, ok)
	}
	x = left + lipgloss.Width(row[:strings.Index(row, "sponsor @vic")]) + 1
	if u, ok := a.helpButtonAt(x, top+1); !ok || u != donateURL {
		t.Fatalf("donate button: %q %v", u, ok)
	}
	if _, ok := a.helpButtonAt(0, 0); ok {
		t.Fatal("click outside buttons hit one")
	}
}
