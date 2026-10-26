package matrix

import (
	"context"
	"fmt"
	"slices"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

type historyKey struct{}

// WithHistory marks ctx as history: events delivered under it were replayed, not new.
func WithHistory(ctx context.Context) context.Context {
	return context.WithValue(ctx, historyKey{}, true)
}

// IsHistory reports whether the events under ctx are history (first sync, backfill).
func IsHistory(ctx context.Context) bool {
	v, _ := ctx.Value(historyKey{}).(bool)
	return v
}

// backfillLimit is how many recent events a room's history loads.
const backfillLimit = 100

// backfillRoom fetches room's last backfillLimit events with /messages (read only) and delivers
// them oldest first as history. Events already shown this run are skipped.
func (c *Client) backfillRoom(ctx context.Context, room id.RoomID, h Handler) error {
	resp, err := c.cli.Messages(ctx, room, "", "", mautrix.DirectionBackward, nil, backfillLimit)
	if err != nil {
		return fmt.Errorf("history %s: %w", room, err)
	}
	evs := resp.Chunk
	slices.Reverse(evs) // /messages backward is newest first
	return c.roomEvents(WithHistory(ctx), room, nil, evs, h, true)
}

// Progress is one toast-sized status: Total > 0 draws a bar; Finished ends it.
type Progress struct {
	ID, Text    string
	Done, Total int
	Finished    bool
}

// report hands p to the OnProgress callback, if any.
func (c *Client) report(p Progress) {
	if c.progress != nil {
		c.progress(p)
	}
}

// OnProgress sets the callback that hears what start is doing.
func (c *Client) OnProgress(f func(Progress)) { c.progress = f }

// LoadRoom gives room its recent history, once per client. The UI calls it when the room is entered.
func (c *Client) LoadRoom(ctx context.Context, room id.RoomID, h Handler) {
	if _, loaded := c.loaded.LoadOrStore(room, true); loaded {
		return
	}
	pid := "history:" + room.String()
	c.report(Progress{ID: pid, Text: "loading history"})
	if err := c.backfillRoom(ctx, room, h); err != nil {
		c.loaded.Delete(room)
		if ctx.Err() == nil {
			c.report(Progress{ID: pid, Text: fmt.Sprintf("history not loaded: %v", err), Finished: true})
		}
		return
	}
	c.report(Progress{ID: pid, Text: "history loaded", Finished: true})
}
