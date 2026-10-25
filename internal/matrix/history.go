package matrix

import (
	"context"
	"fmt"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
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

// backfillLimit is how many events before the first sync's timeline are replayed per room.
const backfillLimit = 100

// backfillRoom syncs one room with a timeline limit, so the room's recent events and state come
// back in one request, and delivers them as history. Events already handled are skipped.
func (c *Client) backfillRoom(ctx context.Context, room id.RoomID, h Handler) error {
	filter, err := c.cli.CreateFilter(ctx, &mautrix.Filter{
		Presence: &mautrix.FilterPart{NotTypes: []event.Type{{Type: "*"}}},
		Room: &mautrix.RoomFilter{
			Rooms:    []id.RoomID{room},
			Timeline: &mautrix.FilterPart{Limit: backfillLimit},
			// state is skipped: the room's members come from the room list, and their join lines are not history
			State: &mautrix.FilterPart{NotTypes: []event.Type{{Type: "*"}}},
		},
	})
	if err != nil {
		return fmt.Errorf("backfill filter %s: %w", room, err)
	}
	resp, err := c.cli.FullSyncRequest(ctx, mautrix.ReqSync{Timeout: 0, FilterID: filter.FilterID})
	if err != nil {
		return fmt.Errorf("backfill %s: %w", room, err)
	}
	if rr, ok := resp.Rooms.Join[room]; ok {
		if err := c.roomEvents(WithHistory(ctx), room, rr.State.Events, rr.Timeline.Events, h, true); err != nil {
			return err
		}
	}
	_, err = c.db.RawDB.ExecContext(ctx, `INSERT OR IGNORE INTO room_backfill(room_id) VALUES(?)`, room.String())
	return err
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
