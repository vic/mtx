package matrix

import (
	"context"
	"fmt"
	"slices"
	"sync"

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

// pageTokens holds, per room, the /messages end token of the oldest page loaded: LoadOlder continues from it.
var pageTokens sync.Map

// backfillLimit is how many recent events a room's history loads.
const backfillLimit = 100

// backfillRoom fetches room's last backfillLimit events with /messages (read only) and delivers
// them oldest first as history. Events already shown this run are skipped.
func (c *Client) backfillRoom(ctx context.Context, room id.RoomID, h Handler) error {
	resp, err := c.cli.Messages(ctx, room, "", "", mautrix.DirectionBackward, nil, backfillLimit)
	if err != nil {
		return fmt.Errorf("history %s: %w", room, err)
	}
	pageTokens.Store(room, resp.End) // where the next older page starts
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

// threadLimit is how many replies of a thread one load fetches.
const threadLimit = 100

// LoadThread fetches thread root's event and its newest replies (read only) and delivers them oldest
// first as history, once per thread per run.
func (c *Client) LoadThread(ctx context.Context, room id.RoomID, root id.EventID, h Handler) error {
	key := "thread:" + string(root)
	if _, loaded := c.loaded.LoadOrStore(key, true); loaded {
		return nil
	}
	ev, err := c.cli.GetEvent(ctx, room, root)
	if err != nil {
		c.loaded.Delete(key)
		return fmt.Errorf("thread root %s: %w", root, err)
	}
	rel, err := c.cli.GetRelations(ctx, room, root, &mautrix.ReqGetRelations{RelationType: event.RelThread, Dir: mautrix.DirectionBackward, Limit: threadLimit})
	if err != nil {
		c.loaded.Delete(key)
		return fmt.Errorf("thread %s: %w", root, err)
	}
	evs := append([]*event.Event{ev}, rel.Chunk...)
	slices.Reverse(evs[1:]) // backward is newest first; the root stays first
	return c.roomEvents(WithHistory(ctx), room, nil, evs, h, true)
}

// LoadContext fetches event ev with up to limit events around it (read only) and delivers them oldest
// first as history.
func (c *Client) LoadContext(ctx context.Context, room id.RoomID, ev id.EventID, h Handler) error {
	resp, err := c.cli.Context(ctx, room, ev, nil, 10)
	if err != nil {
		return fmt.Errorf("context %s: %w", ev, err)
	}
	before := slices.Clone(resp.EventsBefore)
	slices.Reverse(before) // events_before is newest first
	evs := append(append(before, resp.Event), resp.EventsAfter...)
	return c.roomEvents(WithHistory(ctx), room, nil, evs, h, true)
}

// LoadOlder fetches the page of history before the oldest page loaded (read only) and delivers it oldest
// first. done is true when the server has no older events.
func (c *Client) LoadOlder(ctx context.Context, room id.RoomID, h Handler) (done bool, err error) {
	v, ok := pageTokens.Load(room)
	if !ok {
		return false, fmt.Errorf("older %s: no history loaded yet", room)
	}
	from := v.(string)
	if from == "" {
		return true, nil
	}
	resp, err := c.cli.Messages(ctx, room, from, "", mautrix.DirectionBackward, nil, backfillLimit)
	if err != nil {
		return false, fmt.Errorf("older %s: %w", room, err)
	}
	pageTokens.Store(room, resp.End)
	evs := resp.Chunk
	slices.Reverse(evs)
	return resp.End == "" || len(evs) == 0, c.roomEvents(WithHistory(ctx), room, nil, evs, h, true)
}
