package model

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 22, 15, 4, 30, 0, time.UTC)

func roomModel(t *testing.T, lines ...Line) Model {
	t.Helper()
	m := newModel(t)
	r := RingOf[Line](m.cap)
	for _, l := range lines {
		r.Push(l)
	}
	m.chans = []Channel{{Name: "#a", Kind: SessionChan, PID: 1, Unread: 2}, {Name: "&review", Kind: RoomChan, PID: 0, Unread: 3, Lines: r, Members: []Member{{Nick: "a", PID: 2}, {Nick: "b", PID: 3}}}}
	return m
}

func mustNew(t *testing.T) Model {
	t.Helper()
	return newModel(t)
}

func TestWithRoomMembers(t *testing.T) {
	m := roomModel(t)
	m2, err := m.WithRoomMembers("&review", []Member{{Nick: "x", PID: 1}, {Nick: "y", PID: 2}, {Nick: "z", PID: 3}})
	if err != nil {
		t.Fatal(err)
	}
	if got := m2.Channels()[1].Members; !reflect.DeepEqual(got, []Member{{Nick: "x", PID: 1}, {Nick: "y", PID: 2}, {Nick: "z", PID: 3}}) {
		t.Fatalf("members %v", got)
	}
	if len(m.Channels()[1].Members) != 2 { // old Model unchanged
		t.Fatal("old model changed")
	}
	m2, err = m2.Focus(1)
	if err != nil {
		t.Fatal(err)
	}
	if got := Status(m2, "NORMAL", nil, t0).Left; got != "&review 3 members 0/min" {
		t.Fatalf("left %q", got)
	}
	// copy on read: mutating a returned slice does not reach the model
	m2.Channels()[1].Members[0].Nick = "zzz"
	if m2.Channels()[1].Members[0].Nick != "x" {
		t.Fatal("Channels leaks Members")
	}
	// copy on write: caller's slice is not aliased
	in := []Member{{Nick: "x", PID: 1}}
	m3, _ := m.WithRoomMembers("&review", in)
	in[0].Nick = "y"
	if m3.Channels()[1].Members[0].Nick != "x" {
		t.Fatal("input aliased")
	}
}

func TestWithRoomMembersErrors(t *testing.T) {
	m := roomModel(t)
	if _, err := m.WithRoomMembers("#a", []Member{{Nick: "x", PID: 1}}); !errors.Is(err, ErrNotRoom) {
		t.Fatalf("session chan: %v", err)
	}
	if _, err := m.WithRoomMembers("&nope", nil); !errors.Is(err, ErrUnknownChannel) {
		t.Fatalf("unknown: %v", err)
	}
}

func TestStatusSanitizesInfo(t *testing.T) {
	const esc = "\x1b[2J"
	for _, si := range []SessionInfo{{Status: esc, PermMode: "default"}, {Status: "idle", PermMode: esc}} {
		got := Status(roomModel(t), "NORMAL", map[int]SessionInfo{1: si}, t0)
		if strings.ContainsRune(got.Left, 0x1b) {
			t.Errorf("escape reached the modeline: %q", got.Left)
		}
	}
	got := Status(roomModel(t), "NORMAL", map[int]SessionInfo{1: {Status: "idle", PermMode: esc}}, t0)
	if got.Mode != "NORMAL" || got.Left != "#a idle" || got.Red {
		t.Fatalf("got %+v", got)
	}
}

// TestStatusMatrixRoom: a Matrix room's title shows its name, members, rate and room ID, not red.
func TestStatusMatrixRoom(t *testing.T) {
	m := newModel(t).WithRooms([]string{"#den"}).WithIDs(map[string]string{"#den": "!x:s"})
	m, err := m.WithRoomMembers("#den", []Member{{Nick: "a", PID: 0}, {Nick: "b", PID: 0}})
	if err != nil {
		t.Fatal(err)
	}
	got := Status(m, "NORMAL", nil, t0)
	if got.Left != "#den · 2 members · 0/min · !x:s" || got.Red {
		t.Fatalf("got %+v", got)
	}
}

func TestStatus(t *testing.T) {
	const byp, def = "bypassPermissions", "default"
	rate := []Line{{At: t0.Add(-10 * time.Second)}, {At: t0.Add(-59 * time.Second)},
		{At: t0.Add(-61 * time.Second)}, {At: t0.Add(5 * time.Second)}}
	// room members are pids 2 and 3; focusing the room clears its unread 3 (total 2)
	tests := []struct {
		name string
		m    Model
		cur  int
		mode string
		info map[int]SessionInfo
		want Modeline
	}{
		{"session channel", roomModel(t), 0, "NORMAL", map[int]SessionInfo{1: {Status: "busy", PermMode: def}},
			Modeline{Mode: "NORMAL", Left: "#a busy", Right: "5 unread 15:04"}},
		{"unknown session info", roomModel(t), 0, "NORMAL", nil,
			Modeline{Mode: "NORMAL", Left: "#a ?", Right: "5 unread 15:04"}},
		{"room lines/min", roomModel(t, rate...), 1, "NORMAL", map[int]SessionInfo{2: {Status: "idle", PermMode: byp}, 3: {Status: "idle", PermMode: def}},
			Modeline{Mode: "NORMAL", Left: "&review 2 members 2/min", Right: "2 unread 15:04"}},
		{"no channels", mustNew(t), 0, "NORMAL", nil,
			Modeline{Mode: "NORMAL", Right: "0 unread 15:04"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.m
			if tc.cur != 0 {
				var err error
				if m, err = m.Focus(tc.cur); err != nil {
					t.Fatal(err)
				}
			}
			if got := Status(m, tc.mode, tc.info, t0); got != tc.want {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}
