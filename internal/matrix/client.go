package matrix

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

const (
	longPoll     = 30 * time.Second
	failedBackof = 10 * time.Second
)

// Client is a logged-in device with E2EE on and its own sync loop.
type Client struct {
	cli    *mautrix.Client
	helper *cryptohelper.CryptoHelper
	db     *dbutil.Database
	user   id.UserID
}

// New opens the store, checks the device against the server, and starts E2EE.
func New(ctx context.Context, cfg Config) (*Client, error) {
	db, err := OpenDB(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	cli, err := mautrix.NewClient(cfg.Homeserver, id.UserID(cfg.UserID), cfg.AccessToken)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("client: %w", err)
	}
	cli.DeviceID = id.DeviceID(cfg.DeviceID)
	cli.Store = NewSyncStore(db)
	if err := checkDevice(ctx, cli, cfg.DeviceID); err != nil {
		_ = db.Close()
		return nil, err
	}
	helper, err := cryptohelper.NewCryptoHelper(cli, []byte(cfg.PickleKey), db)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("crypto helper: %w", err)
	}
	if err := helper.Init(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("crypto init: %w", err)
	}
	cli.Crypto = helper
	if cli.Crypto == nil { // a send without Crypto goes out in plaintext; never allow it
		_ = db.Close()
		return nil, errors.New("crypto not set on client")
	}
	return &Client{cli: cli, helper: helper, db: db, user: cli.UserID}, nil
}

// checkDevice fails when the token's user or device differs from the env values.
func checkDevice(ctx context.Context, cli *mautrix.Client, wantDevice string) error {
	resp, err := cli.Whoami(ctx)
	if err != nil {
		return fmt.Errorf("whoami: %w", err)
	}
	if resp.UserID != cli.UserID {
		return fmt.Errorf("user mismatch: server %q != MTX_USER_ID %q", resp.UserID, cli.UserID)
	}
	if resp.DeviceID != id.DeviceID(wantDevice) {
		return fmt.Errorf("device mismatch: server %q != MTX_DEVICE_ID %q", resp.DeviceID, wantDevice)
	}
	return nil
}

// Close releases the store.
func (c *Client) Close() error {
	return c.db.Close()
}

// Handler gets one timeline event. decErr is set when an encrypted event could not be decrypted;
// evt is then still the encrypted event.
type Handler func(ctx context.Context, evt *event.Event, decErr error) error

// Run syncs until ctx ends or the token is revoked. Per batch: key sharing first, then each
// event goes to h (a failure stops the loop before the token is saved), then the token is saved.
func (c *Client) Run(ctx context.Context, h Handler) error {
	filter, err := c.filterID(ctx)
	if err != nil {
		return err
	}
	for {
		since, err := c.store().LoadNextBatch(ctx, c.user)
		if err != nil {
			return fmt.Errorf("load token: %w", err)
		}
		timeout := 0
		if since != "" {
			timeout = int(longPoll / time.Millisecond)
		}
		resp, err := c.cli.FullSyncRequest(ctx, mautrix.ReqSync{Timeout: timeout, Since: since, FilterID: filter})
		if err != nil {
			if errors.Is(err, mautrix.MUnknownToken) || ctx.Err() != nil {
				return err
			}
			select { // transient: back off, retry
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(failedBackof):
			}
			continue
		}
		if err := c.apply(ctx, resp, since, h); err != nil {
			return err
		}
	}
}

func (c *Client) apply(ctx context.Context, resp *mautrix.RespSync, since string, h Handler) error {
	m := c.helper.Machine()
	m.ProcessSyncResponse(ctx, resp, since) // room keys, device lists, OTK counts
	// Invites are accepted by policy: join before the batch's room events are handled.
	for roomID := range resp.Rooms.Invite {
		if _, err := c.cli.JoinRoomByID(ctx, roomID); err != nil {
			return fmt.Errorf("auto-join %s: %w", roomID, err)
		}
	}
	for roomID, room := range resp.Rooms.Join {
		for i := range room.State.Events {
			st := room.State.Events[i]
			st.RoomID = roomID
			if err := st.Content.ParseRaw(st.Type); err != nil {
				return fmt.Errorf("parse state %s: %w", st.ID, err)
			}
			if st.Type == event.StateMember {
				m.HandleMemberEvent(ctx, st)
			}
			if err := c.handleOnce(ctx, st, h); err != nil {
				return err
			}
		}
		for i := range room.Timeline.Events {
			evt := room.Timeline.Events[i]
			evt.RoomID = roomID
			if err := c.handleOnce(ctx, evt, h); err != nil {
				return err
			}
		}
	}
	return c.store().SaveNextBatch(ctx, c.user, resp.NextBatch)
}

// handleOnce delivers evt unless it was delivered before, then marks it.
func (c *Client) handleOnce(ctx context.Context, evt *event.Event, h Handler) error {
	done, err := c.delivered(ctx, evt.ID)
	if err != nil {
		return err
	}
	if done {
		return nil
	}
	if err := c.deliver(ctx, evt, h); err != nil {
		return err
	}
	return c.markDelivered(ctx, evt.ID)
}

// deliver decrypts encrypted events and calls h. A handler panic becomes an error.
func (c *Client) deliver(ctx context.Context, evt *event.Event, h Handler) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("handler panicked on %s: %v", evt.ID, r)
		}
	}()
	if evt.Content.Parsed == nil { // state events were parsed for HandleMemberEvent
		if err := evt.Content.ParseRaw(evt.Type); err != nil {
			return fmt.Errorf("parse %s: %w", evt.ID, err)
		}
	}
	if evt.Type == event.EventEncrypted {
		dec, decErr := c.helper.Decrypt(ctx, evt)
		if decErr != nil {
			return h(ctx, evt, decErr)
		}
		return h(ctx, dec, nil)
	}
	return h(ctx, evt, nil)
}

func (c *Client) store() *SyncStore { return NewSyncStore(c.db) }

func (c *Client) filterID(ctx context.Context) (string, error) {
	s := c.store()
	f, err := s.LoadFilterID(ctx, c.user)
	if err != nil || f != "" {
		return f, err
	}
	resp, err := c.cli.CreateFilter(ctx, c.cli.Syncer.GetFilterJSON(c.user))
	if err != nil {
		return "", fmt.Errorf("create filter: %w", err)
	}
	return resp.FilterID, s.SaveFilterID(ctx, c.user, resp.FilterID)
}

// delivered reports whether event_id was already handled (redelivery after a crash).
func (c *Client) delivered(ctx context.Context, eventID id.EventID) (bool, error) {
	var one int
	err := c.db.RawDB.QueryRowContext(ctx, `SELECT 1 FROM seen_events WHERE event_id = ?`, eventID.String()).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (c *Client) markDelivered(ctx context.Context, eventID id.EventID) error {
	_, err := c.db.RawDB.ExecContext(ctx, `INSERT OR IGNORE INTO seen_events(event_id) VALUES(?)`, eventID.String())
	return err
}

// MX exposes the mautrix client for room and account calls outside the sync loop.
func (c *Client) MX() *mautrix.Client { return c.cli }
