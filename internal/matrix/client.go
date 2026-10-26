package matrix

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto"
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
	cli         *mautrix.Client
	helper      *cryptohelper.CryptoHelper
	db          *dbutil.Database
	user        id.UserID
	onInvite    func(Invite)
	onTyping    func(id.RoomID, []id.UserID)
	onReceipt   func(id.RoomID, id.EventID, id.UserID)
	progress    func(Progress)
	mu          sync.Mutex          // one event at a time: see lock
	loaded      sync.Map            // rooms whose history LoadRoom has started
	shown       map[id.EventID]bool // events this run has handed to the UI, history or live
	decryptOnce sync.Once
	decryptSem  chan struct{} // bounds decrypts in flight; see decryptSlot
}

// New opens the store, checks the device against the server, and starts E2EE. With no
// MTX_ACCESS_TOKEN, the token and device come from the stored session; a good start saves them.
func New(ctx context.Context, cfg Config) (*Client, error) {
	db, err := OpenDB(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	s, ok, err := loadSession(ctx, db.RawDB, cfg.PickleKey)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if !ok && (cfg.Homeserver == "" || cfg.UserID == "" || cfg.AccessToken == "") {
		_ = db.Close()
		return nil, errors.New("no stored session: run mtx login, or set MTX_HOMESERVER, MTX_USER_ID and MTX_ACCESS_TOKEN")
	}
	// env wins; the stored session fills what env leaves empty
	cfg.Homeserver = cmp.Or(cfg.Homeserver, s.Homeserver)
	cfg.UserID = cmp.Or(cfg.UserID, s.UserID)
	cfg.AccessToken = cmp.Or(cfg.AccessToken, s.AccessToken)
	cfg.DeviceID = cmp.Or(cfg.DeviceID, s.DeviceID)
	if err := ValidateHomeserver(cfg.Homeserver); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ValidateUserID(cfg.UserID); err != nil {
		_ = db.Close()
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
	if err := saveSession(ctx, db.RawDB, cfg.PickleKey, savedSession{Homeserver: cfg.Homeserver, UserID: cfg.UserID, AccessToken: cfg.AccessToken, DeviceID: string(cli.DeviceID)}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("save session: %w", err)
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
	// Own devices may ask for room keys; the default policy wants cross-signing trust, which
	// unverified devices of this account never have. Other users keep the default policy.
	defaultShare := helper.Machine().AllowKeyShare
	helper.Machine().AllowKeyShare = func(ctx context.Context, dev *id.Device, evt event.RequestedKeyInfo) *crypto.KeyShareRejection {
		if dev.UserID == cli.UserID && dev.DeviceID != cli.DeviceID && dev.Trust != id.TrustStateBlacklisted {
			return nil
		}
		return defaultShare(ctx, dev, evt)
	}
	cli.Crypto = helper
	if cli.Crypto == nil { // a send without Crypto goes out in plaintext; never allow it
		_ = db.Close()
		return nil, errors.New("crypto not set on client")
	}
	return &Client{cli: cli, helper: helper, db: db, user: cli.UserID, shown: map[id.EventID]bool{}}, nil
}

// checkDevice fails when the token's user differs from MTX_USER_ID, or when MTX_DEVICE_ID is set
// and differs from the token's device. The device then comes from the server.
func checkDevice(ctx context.Context, cli *mautrix.Client, wantDevice string) error {
	resp, err := cli.Whoami(ctx)
	if err != nil {
		return fmt.Errorf("whoami: %w", err)
	}
	if resp.UserID != cli.UserID {
		return fmt.Errorf("user mismatch: server %q != MTX_USER_ID %q", resp.UserID, cli.UserID)
	}
	if wantDevice != "" && resp.DeviceID != id.DeviceID(wantDevice) {
		return fmt.Errorf("device mismatch: server %q != MTX_DEVICE_ID %q", resp.DeviceID, wantDevice)
	}
	cli.DeviceID = resp.DeviceID
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
			wait, limited := RetryAfter(err)
			if !limited {
				wait = failedBackof // transient: back off, retry
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
			continue
		}
		if err := c.apply(ctx, resp, since, h); err != nil {
			return err
		}
	}
}

func (c *Client) apply(ctx context.Context, resp *mautrix.RespSync, since string, h Handler) error {
	if since == "" { // first sync: everything here is history, not new
		ctx = WithHistory(ctx)
	}
	m := c.helper.Machine()
	m.ProcessSyncResponse(ctx, resp, since)        // room keys, device lists, OTK counts
	if err := c.retryPending(ctx, h); err != nil { // keys that just arrived unlock parked events
		return err
	}
	// Invites go to the hook; the user accepts or rejects them (/accept, /reject).
	for roomID, inv := range resp.Rooms.Invite {
		in, err := c.inviteOf(roomID, inv)
		if err != nil {
			return err
		}
		if c.onInvite != nil {
			c.onInvite(in)
		}
	}
	var newRooms []id.RoomID
	for roomID, room := range resp.Rooms.Join {
		rctx := ctx
		known, err := c.roomKnown(ctx, roomID)
		if err != nil {
			return err
		}
		if !known { // a room this store has not seen: its older events are history, from prev_batch
			rctx = WithHistory(ctx)
			newRooms = append(newRooms, roomID)
		}
		if err := c.receiptsOf(roomID, room.Ephemeral.Events); err != nil {
			return err
		}
		if err := c.typingOf(roomID, room.Ephemeral.Events); err != nil {
			return err
		}
		if err := c.roomEvents(rctx, roomID, room.State.Events, room.Timeline.Events, h, false); err != nil {
			return err
		}
	}
	// rooms we were removed from or left: the membership change reaches the handler (a kick shows,
	// and the client drops the room)
	for roomID, room := range resp.Rooms.Leave {
		if err := c.roomEvents(ctx, roomID, room.State.Events, room.Timeline.Events, h, false); err != nil {
			return err
		}
	}
	if err := c.markRooms(ctx, newRooms); err != nil {
		return err
	}
	if err := pruneSeen(ctx, c.db.RawDB); err != nil {
		return err
	}
	return c.store().SaveNextBatch(ctx, c.user, resp.NextBatch)
}

// handleOnce delivers evt unless it was delivered before, then marks it.

// deliver decrypts encrypted events and calls h. A handler panic becomes an error.
func (c *Client) deliver(ctx context.Context, evt *event.Event, h Handler) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("handler panicked on %s: %v", evt.ID, r)
		}
	}()
	if evt.Content.Parsed == nil { // state events were parsed for HandleMemberEvent
		if err := evt.Content.ParseRaw(evt.Type); err != nil {
			return h(ctx, evt, nil) // a type this client does not model: the handler sees it raw and ignores it
		}
	}
	if evt.Type == event.EventEncrypted {
		// show a placeholder now; the decrypt runs beside the UI and calls h again with the result
		if err := h(ctx, evt, ErrPending); err != nil {
			return err
		}
		c.decryptAsync(evt, h)
		return nil
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

// roomKnown reports whether the store has seen room in a sync before.
func (c *Client) roomKnown(ctx context.Context, room id.RoomID) (bool, error) {
	var one int
	err := c.db.RawDB.QueryRowContext(ctx, `SELECT 1 FROM rooms_seen WHERE room_id = ?`, room.String()).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// markRooms records rooms as seen. Called before the token is saved, so a failed batch is replayed.
func (c *Client) markRooms(ctx context.Context, rooms []id.RoomID) error {
	for _, r := range rooms {
		if _, err := c.db.RawDB.ExecContext(ctx, `INSERT OR IGNORE INTO rooms_seen(room_id) VALUES(?)`, r.String()); err != nil {
			return err
		}
	}
	return nil
}

// roomEvents applies one room's state and timeline from a batch, in that order.
func (c *Client) roomEvents(ctx context.Context, roomID id.RoomID, state, timeline []*event.Event, h Handler, replay bool) error {
	once := c.handleOnce
	if replay {
		once = c.handleHistory
	}
	m := c.helper.Machine()
	for _, st := range state {
		st.RoomID = roomID
		st.Type.Class = event.StateEventType // the state section is state-class
		if err := st.Content.ParseRaw(st.Type); err != nil {
			continue // a state type this client does not model: nothing to apply, the batch goes on
		}
		if st.Type == event.StateMember {
			m.HandleMemberEvent(ctx, st)
		}
		c.cli.StateStoreSyncHandler(ctx, st) // room state (encryption, members) for sends and lookups
		if err := once(ctx, st, h); err != nil {
			return err
		}
	}
	for _, evt := range timeline {
		evt.RoomID = roomID
		if evt.StateKey != nil {
			c.cli.StateStoreSyncHandler(ctx, evt)
		}
		if err := once(ctx, evt, h); err != nil {
			return err
		}
	}
	return nil
}

// handleOnce delivers a live event unless it was delivered before, then marks it. Each event goes
// to the UI once per run: shown is the in-memory record, seen_events survives restarts.
func (c *Client) handleOnce(ctx context.Context, evt *event.Event, h Handler) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.shown[evt.ID] {
		return nil
	}
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
	c.shown[evt.ID] = true
	return c.markDelivered(ctx, evt.ID)
}

// handleHistory delivers a history event unless this run already showed it. seen_events is not
// consulted: after a restart the UI has no scrollback, so old events must come again.
func (c *Client) handleHistory(ctx context.Context, evt *event.Event, h Handler) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.shown[evt.ID] {
		return nil
	}
	if err := c.deliver(ctx, evt, h); err != nil {
		return err
	}
	c.shown[evt.ID] = true
	return nil
}

// ErrPending is the decrypt error a placeholder carries: the event is shown, its text is coming.
var ErrPending = errors.New("decrypting")

// decryptWorkers is how many decrypts run at once.
const decryptWorkers = 8

// decryptSlot is the semaphore of decrypts in flight; made on first use so a Client built in tests works too.
func (c *Client) decryptSlot() chan struct{} {
	c.decryptOnce.Do(func() { c.decryptSem = make(chan struct{}, decryptWorkers) })
	return c.decryptSem
}

// decryptAsync decrypts evt off the UI path and delivers the result (or its failure) to h with the
// same event ID, so the UI replaces the placeholder. h runs under c.mu, like every delivery.
func (c *Client) decryptAsync(evt *event.Event, h Handler) {
	go func() {
		sem := c.decryptSlot()
		sem <- struct{}{}
		defer func() { <-sem }()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		out, decErr := c.helper.Decrypt(ctx, evt)
		if decErr != nil && errors.Is(decErr, crypto.NoSessionFound) {
			decErr = errors.Join(decErr, c.park(ctx, evt))
		}
		if decErr != nil {
			out = evt
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		if err := h(ctx, out, decErr); err != nil {
			c.report(Progress{ID: "decrypt", Text: fmt.Sprintf("decrypt %s: %v", evt.ID, err), Finished: true})
		}
	}()
}
