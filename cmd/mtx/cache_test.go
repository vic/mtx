package main

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"maunium.net/go/mautrix/id"

	"mtx/internal/matrix"
	"mtx/internal/ui"
)

func TestStartCacherBatchesInOrder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan []matrix.CachedLine, 1)
	enqueue := startCacher(ctx, func(_ context.Context, ls []matrix.CachedLine) error {
		got <- ls
		return nil
	})
	for _, ts := range []int64{1, 2, 3} {
		enqueue(matrix.CachedLine{TS: ts})
	}
	select {
	case ls := <-got:
		if len(ls) != 3 || ls[0].TS != 1 || ls[1].TS != 2 || ls[2].TS != 3 {
			t.Fatalf("batch %v", ls)
		}
	case <-time.After(time.Second):
		t.Fatal("no batch within 1s")
	}
}

func TestLoadCacheShowsRoomsAndLines(t *testing.T) {
	var got []tea.Msg
	r := newRunner(nil, func(m tea.Msg) { got = append(got, m) })
	rooms := func(context.Context) (map[id.RoomID]string, error) {
		return map[id.RoomID]string{"!a:s": "#den"}, nil
	}
	lines := func(_ context.Context, room id.RoomID, _ int) ([]matrix.CachedLine, error) {
		return []matrix.CachedLine{
			{Room: room, EventID: "$1", TS: 1, Nick: "n", Text: "one"},
			{Room: room, EventID: "$2", TS: 2, Nick: "n", Text: "two"},
		}, nil
	}
	r.loadCache(context.Background(), rooms, lines)
	var sawRooms, sawHistory bool
	for _, m := range got {
		switch m := m.(type) {
		case ui.RoomsMsg:
			sawRooms = len(m) == 1 && m[0] == "#den"
		case ui.HistoryLinesMsg:
			sawHistory = m.Ch == "#den" && len(m.L) == 2 && m.L[0].Text == "one" && m.L[1].Text == "two"
		}
	}
	if !sawRooms || !sawHistory {
		t.Fatalf("messages %#v", got)
	}
}
