package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type lateKey struct{}

// WithLate marks ctx as a message that was undecryptable and is shown later, once its key arrived.
func WithLate(ctx context.Context) context.Context { return context.WithValue(ctx, lateKey{}, true) }

// IsLate reports whether the event under ctx was delivered late (decrypted after a placeholder).
func IsLate(ctx context.Context) bool {
	v, _ := ctx.Value(lateKey{}).(bool)
	return v
}

// park stores an encrypted event whose session is missing, and asks other devices for the key.
func (c *Client) park(ctx context.Context, evt *event.Event) error {
	body, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("park %s: %w", evt.ID, err)
	}
	if _, err := c.db.RawDB.ExecContext(ctx, `INSERT OR IGNORE INTO pending_events(event_id, body) VALUES(?, ?)`, evt.ID.String(), string(body)); err != nil {
		return err
	}
	enc := evt.Content.AsEncrypted()
	if enc != nil {
		c.helper.RequestSession(ctx, evt.RoomID, id.SenderKey(enc.SenderKey), id.SessionID(enc.SessionID), evt.Sender, id.DeviceID(enc.DeviceID))
	}
	return nil
}

// retryPending decrypts parked events whose key arrived. Each one goes to h once, marked late.
func (c *Client) retryPending(ctx context.Context, h Handler) error {
	rows, err := c.db.RawDB.QueryContext(ctx, `SELECT event_id, body FROM pending_events ORDER BY rowid`)
	if err != nil {
		return err
	}
	type row struct{ id, body string }
	var todo []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.body); err != nil {
			_ = rows.Close()
			return err
		}
		todo = append(todo, r)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, r := range todo {
		var evt event.Event
		if err := json.Unmarshal([]byte(r.body), &evt); err != nil {
			return fmt.Errorf("pending %s: %w", r.id, err)
		}
		if err := evt.Content.ParseRaw(event.EventEncrypted); err != nil {
			return fmt.Errorf("pending %s: %w", r.id, err)
		}
		dec, err := c.helper.Decrypt(ctx, &evt)
		outcome, err := retryDecision(err)
		if err != nil {
			return fmt.Errorf("pending %s: %w", r.id, err)
		}
		if outcome == retryKeep {
			continue
		}
		if _, err := c.db.RawDB.ExecContext(ctx, `DELETE FROM pending_events WHERE event_id = ?`, r.id); err != nil {
			return err
		}
		if outcome == retryDrop {
			c.cli.Log.Warn().Str("event_id", r.id).Msg("key withheld by sender: event dropped")
			continue
		}
		if err := h(WithLate(ctx), dec, nil); err != nil {
			return err
		}
	}
	return nil
}

type retryOutcome int

const (
	retryDelivered retryOutcome = iota // decrypted: deliver it
	retryKeep                          // no session yet: stays parked
	retryDrop                          // the sender withheld the key: final, dropped
)

// retryDecision classifies a decrypt error from the retry loop. Only an unknown failure is an error,
// so one bad event cannot stop sync.
func retryDecision(err error) (retryOutcome, error) {
	switch {
	case err == nil:
		return retryDelivered, nil
	case errors.Is(err, crypto.NoSessionFound):
		return retryKeep, nil
	case errors.Is(err, crypto.ErrGroupSessionWithheld):
		return retryDrop, nil
	}
	return retryKeep, err
}
