package main

import (
	"context"
	"time"

	"maunium.net/go/mautrix/id"

	"mtx/internal/matrix"
	"mtx/internal/model"
	"mtx/internal/ui"
)

// cacheEvery is how long the cache writer gathers lines before one write.
const cacheEvery = 200 * time.Millisecond

// startCacher returns a non-blocking enqueue for lines to cache. A goroutine writes them with save in
// batches, so sync never waits on the disk. A full queue drops the line: the cache is best effort, the
// server still has it.
func startCacher(ctx context.Context, save func(context.Context, []matrix.CachedLine) error) func(matrix.CachedLine) {
	in := make(chan matrix.CachedLine, 4096)
	go func() {
		var buf []matrix.CachedLine
		tick := time.NewTimer(cacheEvery)
		tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case l := <-in:
				if len(buf) == 0 {
					tick.Reset(cacheEvery)
				}
				buf = append(buf, l)
			case <-tick.C:
				if err := save(ctx, buf); err != nil {
					logf("cache: %v", err)
				}
				buf = nil
			}
		}
	}()
	return func(l matrix.CachedLine) {
		select {
		case in <- l:
		default:
		}
	}
}

// cachedLines is how many lines per room a start shows from the cache.
const cachedLines = 200

// loadCache registers the rooms saved last run and shows their recent lines as history, so the UI
// fills before the server answers. Errors only log: the server path still loads everything.
func (r *runner) loadCache(ctx context.Context, rooms func(context.Context) (map[id.RoomID]string, error), lines func(context.Context, id.RoomID, int) ([]matrix.CachedLine, error)) {
	labels, err := rooms(ctx)
	if err != nil {
		logf("cache rooms: %v", err)
		return
	}
	for room, label := range labels {
		r.register(room, label)
	}
	r.emitRooms()
	for room := range labels {
		name, ok := r.nameOf(room)
		if !ok {
			continue
		}
		ls, err := lines(ctx, room, cachedLines)
		if err != nil {
			logf("cache lines %s: %v", room, err)
			continue
		}
		out := make([]model.Line, len(ls))
		for i, l := range ls {
			out[i] = model.Line{At: time.UnixMilli(l.TS), Nick: l.Nick, Text: l.Text, Dim: l.Dim, ID: l.EventID.String(), Thread: l.Thread}
		}
		if len(out) > 0 {
			r.emit(ui.HistoryLinesMsg{Ch: name, L: out})
		}
	}
}

// labels is a copy of every registered room's channel name, for the cache.
func (r *runner) labels() map[id.RoomID]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[id.RoomID]string, len(r.ids))
	for room, name := range r.ids {
		out[room] = name
	}
	return out
}
