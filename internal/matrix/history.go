package matrix

import (
	"context"

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

// backfillLimit is how many events before the first sync's timeline are replayed per room.
const backfillLimit = 50

// backfill replays the events before from (a prev_batch), oldest first, through h as history.
func (c *Client) backfill(ctx context.Context, room id.RoomID, from string, h Handler) error {
	resp, err := c.cli.Messages(ctx, room, from, "", mautrix.DirectionBackward, nil, backfillLimit)
	if err != nil {
		return err
	}
	for i := len(resp.Chunk) - 1; i >= 0; i-- {
		evt := resp.Chunk[i]
		evt.RoomID = room
		if err := c.handleOnce(ctx, evt, h); err != nil {
			return err
		}
	}
	return nil
}
