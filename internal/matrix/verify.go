package matrix

import (
	"context"
	"strconv"
	"strings"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/verificationhelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// VerifyEvent is one step of a device verification, for the UI.
type VerifyEvent struct {
	Kind   string // "request" (another device asks), "ready", "sas" (compare Emojis), "done", "cancel"
	Txn    string
	From   string // the other device (user and device ID)
	Emojis string // for "sas": the 7 emojis with their names, "🐶 dog  🔑 key ..."
	Reason string // for "cancel"
}

// verifyCallbacks forwards the helper's callbacks to the client's onVerify hook.
type verifyCallbacks struct{ c *Client }

// OnVerify sets the hook called with each verification step.
func (c *Client) OnVerify(fn func(VerifyEvent)) { c.onVerify = fn }

func (c *Client) verify(e VerifyEvent) {
	if c.onVerify != nil {
		c.onVerify(e)
	}
}

// StartVerify asks my other devices to verify with this one and returns the transaction ID.
func (c *Client) StartVerify(ctx context.Context) (string, error) {
	txn, err := c.vh.StartVerification(ctx, c.user)
	if err != nil {
		return "", err
	}
	c.started.Store(string(txn), true)
	return string(txn), nil
}

// AcceptVerify accepts a request from another device.
func (c *Client) AcceptVerify(ctx context.Context, txn string) error {
	return c.vh.AcceptVerification(ctx, id.VerificationTransactionID(txn))
}

// ConfirmVerify says the emojis match.
func (c *Client) ConfirmVerify(ctx context.Context, txn string) error {
	return c.vh.ConfirmSAS(ctx, id.VerificationTransactionID(txn))
}

// CancelVerify cancels the verification from this side.
func (c *Client) CancelVerify(ctx context.Context, txn string) error {
	return c.vh.CancelVerification(ctx, id.VerificationTransactionID(txn), event.VerificationCancelCodeUser, "cancelled in mtx")
}

// dispatchVerification hands the verification to-device events to the syncer, whose handlers the helper
// registered. Other to-device events are left alone: the crypto machine already saw the whole sync.
func (c *Client) dispatchVerification(ctx context.Context, evts []*event.Event) {
	s, ok := c.cli.Syncer.(*mautrix.DefaultSyncer)
	if !ok {
		return
	}
	for _, e := range evts {
		if !strings.HasPrefix(e.Type.Type, "m.key.verification.") {
			continue
		}
		e.Type.Class = event.ToDeviceEventType
		if e.Content.Parsed == nil {
			_ = e.Content.ParseRaw(e.Type)
		}
		s.Dispatch(ctx, e)
	}
}

func (v verifyCallbacks) VerificationRequested(_ context.Context, txn id.VerificationTransactionID, from id.UserID, dev id.DeviceID) {
	v.c.verify(VerifyEvent{Kind: "request", Txn: string(txn), From: string(from) + " " + string(dev)})
}

func (v verifyCallbacks) VerificationReady(ctx context.Context, txn id.VerificationTransactionID, dev id.DeviceID, _, _ bool, _ *verificationhelper.QRCode) {
	v.c.verify(VerifyEvent{Kind: "ready", Txn: string(txn), From: string(dev)})
	if _, mine := v.c.started.Load(string(txn)); mine { // we asked: the SAS starts now
		go func() { _ = v.c.vh.StartSAS(context.Background(), txn) }()
	}
}

func (v verifyCallbacks) VerificationCancelled(_ context.Context, txn id.VerificationTransactionID, code event.VerificationCancelCode, reason string) {
	v.c.verify(VerifyEvent{Kind: "cancel", Txn: string(txn), Reason: string(code) + " " + reason})
}

func (v verifyCallbacks) VerificationDone(_ context.Context, txn id.VerificationTransactionID, _ event.VerificationMethod) {
	v.c.verify(VerifyEvent{Kind: "done", Txn: string(txn)})
}

func (v verifyCallbacks) ShowSAS(_ context.Context, txn id.VerificationTransactionID, emojis []rune, descs []string, decimals []int) {
	var parts []string
	if len(emojis) > 0 {
		for i, e := range emojis {
			parts = append(parts, string(e)+" "+descs[i])
		}
	} else {
		for _, d := range decimals {
			parts = append(parts, strconv.Itoa(d))
		}
	}
	v.c.verify(VerifyEvent{Kind: "sas", Txn: string(txn), Emojis: strings.Join(parts, "  ")})
}
