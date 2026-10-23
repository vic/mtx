//go:build conduit

package matrix

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// registerUser creates a user with a random password, using MTX_CONDUIT_REG_TOKEN.
func registerUser(t *testing.T, ctx context.Context, home, name string) (user, password string) {
	t.Helper()
	tok := os.Getenv("MTX_CONDUIT_REG_TOKEN")
	require.NotEmpty(t, tok, "MTX_CONDUIT_REG_TOKEN")
	cli, err := mautrix.NewClient(home, "", "")
	require.NoError(t, err)
	password = fmt.Sprintf("pw-%d-%d", time.Now().UnixNano(), len(name))
	resp, _, err := cli.Register(ctx, &mautrix.ReqRegister[any]{Username: name, Password: password,
		InitialDeviceDisplayName: "mtx-test", Auth: map[string]any{"type": "m.login.registration_token", "token": tok}})
	require.NoError(t, err)
	return resp.UserID.String(), password
}

// loginDevice logs user in with password as device dev and returns its Config.
func loginDevice(t *testing.T, ctx context.Context, home string, user id.UserID, password string, dev string) Config {
	t.Helper()
	cli, err := mautrix.NewClient(home, "", "")
	require.NoError(t, err)
	resp, err := cli.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypePassword,
		Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: user.String()},
		Password:   password, DeviceID: id.DeviceID(dev), StoreCredentials: true})
	require.NoError(t, err)
	return Config{Homeserver: home, UserID: user.String(), AccessToken: resp.AccessToken, DeviceID: string(resp.DeviceID),
		PickleKey: "pickle-conduit", DBPath: filepath.Join(t.TempDir(), dev, "mtx.db")}
}

// TestConduitMultiDevice: a second device of bob (same account) receives alice's room key and
// decrypts the same message as the first device.
func TestConduitMultiDevice(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	home := os.Getenv("MTX_CONDUIT_HOMESERVER")
	require.NotEmpty(t, home)
	alice, err := New(ctx, conduitEnv(t, "A"))
	require.NoError(t, err)
	defer alice.Close()

	bobUser, bobPw := registerUser(t, ctx, home, fmt.Sprintf("mtxmd%d", time.Now().UnixNano()%1e9))
	cfg1 := loginDevice(t, ctx, home, id.UserID(bobUser), bobPw, "DEV1")
	cfg2 := loginDevice(t, ctx, home, id.UserID(bobUser), bobPw, "DEV2")
	b1, err := New(ctx, cfg1)
	require.NoError(t, err)
	defer b1.Close()
	b2, err := New(ctx, cfg2)
	require.NoError(t, err)
	defer b2.Close()

	room := encryptedRoom(t, ctx, alice, bobUser)
	got1, got2 := make(chan *event.Event, 64), make(chan *event.Event, 64)
	stop1, done1 := collect(ctx, b1, got1)
	defer func() { stop1(); <-done1 }()
	stop2, done2 := collect(ctx, b2, got2)
	defer func() { stop2(); <-done2 }()

	heard := map[string]bool{}
	deadline := time.After(90 * time.Second)
	for !(heard["1"] && heard["2"]) {
		_, err := alice.MX().SendText(ctx, room, "hello devices")
		require.NoError(t, err)
		select {
		case e := <-got1:
			if e.Content.AsMessage() != nil && e.Type == event.EventMessage && e.Content.AsMessage().Body == "hello devices" {
				heard["1"] = true
			}
		case e := <-got2:
			if e.Content.AsMessage() != nil && e.Type == event.EventMessage && e.Content.AsMessage().Body == "hello devices" {
				heard["2"] = true
			}
		case <-time.After(3 * time.Second):
		case <-deadline:
			t.Fatalf("devices heard: %v", heard)
		}
	}
}

// TestConduitLateDecrypt: bob's DEV1 is in the room when alice sends. DEV2 (same account) logs in
// later: it shows a placeholder, parks the event and asks its own devices for the key; DEV1 answers;
// DEV2 shows the message once, late. Key requests are only answered for the user's own devices.
func TestConduitLateDecrypt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	home := os.Getenv("MTX_CONDUIT_HOMESERVER")
	require.NotEmpty(t, home)
	alice, err := New(ctx, conduitEnv(t, "A"))
	require.NoError(t, err)
	defer alice.Close()

	bobUser, bobPw := registerUser(t, ctx, home, fmt.Sprintf("mtxld%d", time.Now().UnixNano()%1e9))
	dev1, err := New(ctx, loginDevice(t, ctx, home, id.UserID(bobUser), bobPw, "DEV1"))
	require.NoError(t, err)
	defer dev1.Close()

	room := encryptedRoom(t, ctx, alice, bobUser)
	got1 := make(chan *event.Event, 64)
	stop1, done1 := collect(ctx, dev1, got1)
	defer func() { stop1(); <-done1 }()
	waitFor(t, got1, 60*time.Second, func(e *event.Event) bool { return e.RoomID == room && e.Type == event.StateMember })

	_, err = alice.MX().SendText(ctx, room, "early")
	require.NoError(t, err)
	waitFor(t, got1, 60*time.Second, func(e *event.Event) bool {
		return e.Content.AsMessage() != nil && e.Content.AsMessage().Body == "early"
	})

	dev2, err := New(ctx, loginDevice(t, ctx, home, id.UserID(bobUser), bobPw, "DEV2"))
	require.NoError(t, err)
	defer dev2.Close()
	placeholder := make(chan struct{}, 1)
	late := make(chan struct{}, 1)
	runCtx, stop2 := context.WithCancel(ctx)
	defer stop2()
	go func() {
		_ = dev2.Run(runCtx, func(hctx context.Context, evt *event.Event, decErr error) error {
			if evt.RoomID != room {
				return nil
			}
			if evt.Type == event.EventEncrypted && decErr != nil {
				select {
				case placeholder <- struct{}{}:
				default:
				}
			}
			if m := evt.Content.AsMessage(); evt.Type == event.EventMessage && decErr == nil && m != nil && m.Body == "early" && IsLate(hctx) {
				select {
				case late <- struct{}{}:
				default:
				}
			}
			return nil
		})
	}()
	select {
	case <-placeholder:
	case <-time.After(60 * time.Second):
		t.Fatal("no placeholder for the early message on DEV2")
	}
	select {
	case <-late:
	case <-time.After(60 * time.Second):
		t.Fatal("early message not shown late on DEV2 after the key arrived")
	}
}
