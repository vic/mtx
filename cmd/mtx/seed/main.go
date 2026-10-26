//go:build seed

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

const demoPass = "seed-demo-pass"

// demoUsers are the people of the demo; dan is the account the tapes log in as.
var demoUsers = []string{
	"alice", "bob", "carol", "dan",
	"erin", "frank", "grace", "heidi",
}

// catPNG is a small real PNG (an orange square) so the client can decode and show it.
func catPNG() ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, 96, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 96; x++ {
			img.Set(x, y, color.RGBA{R: 240, G: uint8(120 + y), B: 60, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// register creates a user; a user that already exists is reported as "exists".
func register(home, token, user, pass string) (userID string, err error) {
	body, _ := json.Marshal(map[string]any{
		"username":                    user,
		"password":                    pass,
		"initial_device_display_name": "seed",
		"auth":                        map[string]string{"type": "m.login.registration_token", "token": token},
	})
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(home+"/_matrix/client/v3/register", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusBadRequest && strings.Contains(string(raw), "M_USER_IN_USE") {
		fmt.Printf("%s exists\n", user)
		cli, err := mautrix.NewClient(home, "", "")
		if err != nil {
			return "", err
		}
		resp, err := cli.Login(context.Background(), &mautrix.ReqLogin{
			Type:       mautrix.AuthTypePassword,
			Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: user},
			Password:   pass,
		})
		if err != nil {
			return "", fmt.Errorf("login existing %s: %w", user, err)
		}
		return string(resp.UserID), nil
	}
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("register %s: status %d: %s", user, resp.StatusCode, raw)
	}
	var out struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	fmt.Printf("registered %s\n", out.UserID)
	return out.UserID, nil
}

func main() {
	home := os.Getenv("MTX_SEED_HOMESERVER")
	if home == "" {
		home = "http://127.0.0.1:6167"
	}
	token := os.Getenv("MTX_SEED_REG_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "MTX_SEED_REG_TOKEN is required")
		os.Exit(1)
	}
	if err := run(home, token); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run(home, token string) error {
	ids := map[string]string{}
	for _, name := range demoUsers {
		uid, err := register(home, token, name, demoPass)
		if err != nil {
			return err
		}
		ids[name] = uid
	}

	ctx := context.Background()
	clients, err := loginAll(ctx, home, ids)
	if err != nil {
		return err
	}
	dan := clients["dan"]
	fmt.Println("logged in dan")

	var invite []id.UserID
	for _, name := range demoUsers {
		if name != "dan" {
			invite = append(invite, id.UserID(ids[name]))
		}
	}
	room, err := dan.CreateRoom(ctx, &mautrix.ReqCreateRoom{
		RoomAliasName: "mtx",
		Name:          "mtx",
		Topic:         "matrix in the terminal",
		Preset:        "public_chat",
		Invite:        invite,
	})
	if err != nil {
		return fmt.Errorf("create room: %w", err)
	}
	for name, cli := range clients {
		if name == "dan" {
			continue
		}
		if _, err := cli.JoinRoomByID(ctx, room.RoomID); err != nil {
			return fmt.Errorf("join %s: %w", name, err)
		}
	}
	fmt.Println("room", room.RoomID)

	if err := playConversation(ctx, clients, room.RoomID); err != nil {
		return err
	}
	fmt.Println("conversation sent")

	cat, err := catPNG()
	if err != nil {
		return fmt.Errorf("make image: %w", err)
	}
	up, err := dan.UploadBytesWithName(ctx, cat, "image/png", "cat.png")
	if err != nil {
		return fmt.Errorf("upload image: %w", err)
	}
	if _, err := dan.SendMessageEvent(ctx, room.RoomID, event.EventMessage, &event.MessageEventContent{
		MsgType: event.MsgImage,
		Body:    "cat.png",
		URL:     up.ContentURI.CUString(),
	}); err != nil {
		return fmt.Errorf("send image: %w", err)
	}
	fmt.Println("sent image cat.png")

	space, err := dan.CreateRoom(ctx, &mautrix.ReqCreateRoom{
		Name:            "mtx space",
		Topic:           "projects",
		Preset:          "private_chat",
		CreationContent: map[string]any{"type": "m.space"},
	})
	if err != nil {
		return fmt.Errorf("create space: %w", err)
	}
	fmt.Println("space", space.RoomID)

	if err := playRooms(ctx, clients, space.RoomID); err != nil {
		return err
	}
	if err := playDMs(ctx, clients); err != nil {
		return err
	}

	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return fmt.Errorf("pickle key: %w", err)
	}
	dir := "docs/tapes/.demo"
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	env := fmt.Sprintf("MTX_HOMESERVER=%s\nMTX_USER_ID=%s\nMTX_ACCESS_TOKEN=%s\nMTX_DEVICE_ID=%s\nMTX_PICKLE_KEY=%s\n",
		home, ids["dan"], dan.AccessToken, dan.DeviceID, hex.EncodeToString(key))
	if err := os.WriteFile(dir+"/env", []byte(env), 0o600); err != nil {
		return err
	}
	fmt.Println("wrote docs/tapes/.demo/env")
	return nil
}
