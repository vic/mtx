//go:build conduit

package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// TestConduitCreate: /create against the real server (alias from env MTX_CONDUIT_ROOM, or random).
func TestConduitCreate(t *testing.T) {
	home := os.Getenv("MTX_CONDUIT_HOMESERVER")
	user := os.Getenv("MTX_CONDUIT_A_USER")
	tok := os.Getenv("MTX_CONDUIT_A_TOKEN")
	require.NotEmpty(t, tok)
	cli, err := mautrix.NewClient(home, id.UserID(user), tok)
	require.NoError(t, err)
	cli.DeviceID = id.DeviceID(os.Getenv("MTX_CONDUIT_A_DEVICE"))
	var out []tea.Msg
	r := newRunner(cli, func(m tea.Msg) { out = append(out, m) })
	cs := &cmds{r: r, me: id.UserID(user), server: "oeiuwq.com"}
	name := os.Getenv("MTX_CONDUIT_ROOM")
	if name == "" {
		name = fmt.Sprintf("&tst%d", time.Now().UnixNano()%1e6)
	}
	err = cs.create(name)
	t.Logf("CREATE %s err=%v msgs=%d", name, err, len(out))
	for _, m := range out {
		t.Logf("MSG %T %v", m, m)
	}
	require.NoError(t, err)
}
