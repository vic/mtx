package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	tea "charm.land/bubbletea/v2"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
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
