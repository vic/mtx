package ui

import (
	"errors"
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
