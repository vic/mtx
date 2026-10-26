package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	tea "charm.land/bubbletea/v2"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/mockserver"
)

// TestFetchMediaPlain: an unencrypted mxc file downloads as its bytes.
func TestFetchMediaPlain(t *testing.T) {
	ts := mockserver.Create(t)
	serve := func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "hello") }
	ts.Router.HandleFunc("GET /_matrix/client/v1/media/download/{server}/{id}", serve)
	ts.Router.HandleFunc("GET /_matrix/media/v3/download/{server}/{id}", serve)
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})
	data, err := r.fetchMedia(context.Background(), &event.MessageEventContent{URL: "mxc://mock.test/abc"})
	require.NoError(t, err)
	require.Equal(t, "hello", string(data))
}

// TestOpenNoMedia: /open on a line without media is an error, and nothing opens.
func TestOpenNoMedia(t *testing.T) {
	r := newRunner(nil, func(tea.Msg) {})
	require.Error(t, r.open("$x"))
}

// TestUploadPlain: an image uploads and is sent as m.image with its mxc URI.
func TestUploadPlain(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var sent string
	upload := func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"content_uri":"mxc://mock.test/up1"}`) }
	ts.Router.HandleFunc("POST /_matrix/media/v3/upload", upload)
	ts.Router.HandleFunc("POST /_matrix/client/v1/media/upload", upload)
	ts.Router.HandleFunc("PUT /_matrix/client/v3/rooms/{room}/send/m.room.message/{txn}", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		sent = string(b)
		mu.Unlock()
		fmt.Fprint(w, `{"event_id":"$u"}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})
	r.register(testRoom, "#r")
	path := filepath.Join(t.TempDir(), "a.png")
	require.NoError(t, os.WriteFile(path, []byte("\x89PNG\r\n\x1a\nxxxx"), 0o600))
	require.NoError(t, r.upload("#r", path))
	mu.Lock()
	defer mu.Unlock()
	require.Contains(t, sent, `"msgtype":"m.image"`)
	require.Contains(t, sent, "mxc://mock.test/up1")
}

// TestSetAvatar: an image uploads and is set as the avatar by its mxc URI.
func TestSetAvatar(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var avatar string
	upload := func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"content_uri":"mxc://mock.test/av1"}`) }
	ts.Router.HandleFunc("POST /_matrix/media/v3/upload", upload)
	ts.Router.HandleFunc("POST /_matrix/client/v1/media/upload", upload)
	ts.Router.HandleFunc("PUT /_matrix/client/v3/profile/{user}/avatar_url", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		avatar = string(b)
		mu.Unlock()
		fmt.Fprint(w, `{}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})
	path := filepath.Join(t.TempDir(), "me.png")
	require.NoError(t, os.WriteFile(path, []byte("\x89PNG\r\n\x1a\nxxxx"), 0o600))
	require.NoError(t, r.setAvatar(path))
	mu.Lock()
	defer mu.Unlock()
	require.Contains(t, avatar, "mxc://mock.test/")
}

// TestSetAvatarNotImage: a file that is not an image is refused before any upload.
func TestSetAvatarNotImage(t *testing.T) {
	r := newRunner(nil, func(tea.Msg) {})
	path := filepath.Join(t.TempDir(), "notes.txt")
	require.NoError(t, os.WriteFile(path, []byte("just some words\n"), 0o600))
	err := r.setAvatar(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not an image")
}

// TestEncryptedMediaRoundTrip: an upload to an encrypted room sends the bytes encrypted, with a file (not
// url) in the message, and fetchMedia decrypts them back to the plaintext.
func TestEncryptedMediaRoundTrip(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var stored []byte
	var sent []byte
	ts.Router.HandleFunc("POST /_matrix/media/v3/upload", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		stored = b
		mu.Unlock()
		fmt.Fprint(w, `{"content_uri":"mxc://mock.test/enc1"}`)
	})
	download := func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_, _ = w.Write(stored)
	}
	ts.Router.HandleFunc("GET /_matrix/client/v1/media/download/{server}/{id}", download)
	ts.Router.HandleFunc("GET /_matrix/media/v3/download/{server}/{id}", download)
	ts.Router.HandleFunc("PUT /_matrix/client/v3/rooms/{room}/send/{type}/{txn}", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		sent = b
		mu.Unlock()
		fmt.Fprint(w, `{"event_id":"$m1"}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	ctx := context.Background()
	store := mautrix.NewMemoryStateStore()
	require.NoError(t, store.SetEncryptionEvent(ctx, testRoom, &event.EncryptionEventContent{Algorithm: id.AlgorithmMegolmV1}))
	cli.StateStore = store
	r := newRunner(cli, func(tea.Msg) {})
	r.register(testRoom, "#r")

	plain := []byte(strings.Repeat("hello encrypted ", 4))
	path := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(path, plain, 0o600))
	require.NoError(t, r.upload("#r", path))

	mu.Lock()
	require.NotEqual(t, plain, stored, "the upload must not carry the plaintext")
	var msg map[string]any
	require.NoError(t, json.Unmarshal(sent, &msg))
	mu.Unlock()
	require.Contains(t, msg, "file")
	require.NotContains(t, msg, "url")

	var content event.MessageEventContent
	require.NoError(t, json.Unmarshal(sent, &content))
	got, err := r.fetchMedia(ctx, &content)
	require.NoError(t, err)
	require.Equal(t, plain, got)
}

// TestViewNoImage: :view of a line without an image says so, without a request.
func TestViewNoImage(t *testing.T) {
	r := newRunner(nil, func(tea.Msg) {})
	err := r.view("$unknown")
	require.ErrorContains(t, err, "no image")
	r.recordMedia("$file", &event.MessageEventContent{MsgType: event.MsgFile, Body: "x.pdf", URL: "mxc://mock.test/x"})
	require.ErrorContains(t, r.view("$file"), "no image")
}
