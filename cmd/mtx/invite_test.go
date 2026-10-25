package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	tea "charm.land/bubbletea/v2"
	"testing"

	"github.com/stretchr/testify/require"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/mockserver"
)

// TestInviteFullUserID: a bare @bob is sent as @bob:<our server>, so the server gets a valid user ID.
func TestInviteFullUserID(t *testing.T) {
	ts := mockserver.Create(t)
	var mu sync.Mutex
	var sent []string
	ts.Router.HandleFunc("POST /_matrix/client/v3/rooms/{roomID}/invite", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m struct {
			UserID string `json:"user_id"`
		}
		_ = json.Unmarshal(b, &m)
		mu.Lock()
		sent = append(sent, m.UserID)
		mu.Unlock()
		fmt.Fprint(w, `{}`)
	})
	cli, err := mautrix.NewClient(ts.Server.URL, "@alice:mock.test", "tok")
	require.NoError(t, err)
	r := newRunner(cli, func(tea.Msg) {})
	r.register(testRoom, "r")
	cs := &cmds{r: r, me: "@alice:mock.test", server: "mock.test"}

	require.NoError(t, cs.invite("@bob", "r"))
	require.NoError(t, cs.invite("@carol:other.test", "r"))
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"@bob:mock.test", "@carol:other.test"}, sent)
}
