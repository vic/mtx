package matrix

import (
	"context"
	"encoding/json"
	"testing"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
)

// TestDispatchVerificationOnlyVerifyEvents: only the verification to-device events reach the syncer.
func TestDispatchVerificationOnlyVerifyEvents(t *testing.T) {
	syncer := mautrix.NewDefaultSyncer()
	calls := 0
	syncer.OnEventType(event.ToDeviceVerificationRequest, func(context.Context, *event.Event) { calls++ })
	c := &Client{cli: &mautrix.Client{Syncer: syncer}}
	var req, key event.Event
	if err := json.Unmarshal([]byte(`{"type":"m.key.verification.request","sender":"@bob:mock.test","content":{"from_device":"DEV2","methods":["m.sas.v1"],"timestamp":1,"transaction_id":"t1"}}`), &req); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"type":"m.room_key","sender":"@bob:mock.test","content":{}}`), &key); err != nil {
		t.Fatal(err)
	}
	c.dispatchVerification(context.Background(), []*event.Event{&req, &key})
	if calls != 1 {
		t.Fatalf("verification handler ran %d times, want 1", calls)
	}
}

// TestShowSASFormats: the emojis reach the UI as "emoji name" pairs.
func TestShowSASFormats(t *testing.T) {
	c := &Client{}
	var got VerifyEvent
	c.OnVerify(func(e VerifyEvent) { got = e })
	verifyCallbacks{c}.ShowSAS(context.Background(), "t", []rune{'🐶', '🔑'}, []string{"dog", "key"}, nil)
	if got.Kind != "sas" || got.Emojis != "🐶 dog  🔑 key" {
		t.Fatalf("got %+v", got)
	}
}
