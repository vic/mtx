package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"

	"mtx/internal/matrix"
)

func TestOnboardRejectsBadHomeserver(t *testing.T) {
	m := onboard{hs: "matrix.org", user: "@a:matrix.org", focus: focusSSO}
	got, cmd := m.startLogin()
	require.Nil(t, cmd, "no login starts for a bad form")
	require.Equal(t, stageForm, got.(onboard).stage)
	require.Contains(t, got.(onboard).msg, "homeserver must be an http(s) URL")
}

func TestOnboardLoginSuccessShowsEnv(t *testing.T) {
	m := onboard{stage: stageWait, store: "/data/mtx/mtx.db", cancel: func() {}}
	got, _ := m.Update(loginDone{sess: matrix.Session{
		Homeserver: "https://matrix.org", UserID: "@a:matrix.org", DeviceID: "DEV", AccessToken: "tok",
	}})
	fm := got.(onboard)
	require.Equal(t, stageSave, fm.stage)
	require.Len(t, fm.cfg.PickleKey, 44)
	require.Equal(t, "/data/mtx/mtx.db", fm.cfg.DBPath)

	s := fm.layout().screen
	require.Contains(t, s, "bash")
	require.Contains(t, s, "Copy")
	require.NotContains(t, s, "MTX_HOMESERVER")
	require.NotContains(t, s, "MTX_ACCESS_TOKEN")
	require.Contains(t, s, "export MTX_PICKLE_KEY=")
}

func TestOnboardLateLoginIgnoredAfterCancel(t *testing.T) {
	m := onboard{stage: stageForm, msg: "login cancelled"}
	got, _ := m.Update(loginDone{err: os.ErrDeadlineExceeded})
	require.Equal(t, stageForm, got.(onboard).stage)
	require.Equal(t, "login cancelled", got.(onboard).msg)
}

func TestOnboardTypingEditsFocusedField(t *testing.T) {
	m := onboard{hs: "https://matrix.or", focus: focusHS}
	got, _ := m.keyPress(tea.KeyPressMsg{Code: 'g', Text: "g"})
	require.Equal(t, "https://matrix.org", got.(onboard).hs)
	got, _ = got.(onboard).keyPress(tea.KeyPressMsg{Code: tea.KeyBackspace})
	require.Equal(t, "https://matrix.or", got.(onboard).hs)
}

func TestShQuote(t *testing.T) {
	require.Equal(t, `'it'\''s'`, shQuote("it's"))
}

func TestStartConfigRejectsUnknownArgs(t *testing.T) {
	_, err := startConfig([]string{"--nope"}, t.TempDir())
	require.ErrorContains(t, err, "unknown arguments")
}

func TestFirstRunStopsOnExistingStore(t *testing.T) {
	for _, n := range matrix.Required {
		t.Setenv(n, "")
	}
	t.Setenv("MTX_DB", "")
	data := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(data, "mtx"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(data, "mtx", "mtx.db"), nil, 0o600))

	_, err := startConfig(nil, data)
	require.ErrorContains(t, err, "a store exists at")
	require.True(t, strings.Contains(err.Error(), "mtx login"))
}

func clickAt(h hit) tea.MouseClickMsg {
	return tea.MouseClickMsg{X: h.x + 1, Y: h.y + 1, Button: tea.MouseLeft}
}

func TestOnboardClickFocusesInput(t *testing.T) {
	m := onboard{}
	hits := m.layout().hits
	require.Len(t, hits, 4) // hs, user, SSO, quit
	got, _ := m.Update(clickAt(hits[1]))
	require.Equal(t, focusUser, got.(onboard).focus)
}

func TestOnboardClickSSOStartsLogin(t *testing.T) {
	m := onboard{hs: "matrix.org", user: "@a:matrix.org"}
	sso := m.layout().hits[2]
	got, _ := m.Update(clickAt(sso))
	require.Equal(t, focusSSO, got.(onboard).focus)
	require.Contains(t, got.(onboard).msg, "homeserver must be an http(s) URL")
}

func TestOnboardClickQuitQuits(t *testing.T) {
	m := onboard{}
	_, cmd := m.Update(clickAt(m.layout().hits[3]))
	require.NotNil(t, cmd)
	require.IsType(t, tea.QuitMsg{}, cmd())
}

func TestOnboardCursorOnFocusedInput(t *testing.T) {
	m := onboard{hs: "https://x", focus: focusHS}
	require.Contains(t, m.layout().screen, lipgloss.NewStyle().Reverse(true).Render(" "))
}

func TestOnboardCopyCopiesExports(t *testing.T) {
	m := onboard{stage: stageSave, cfg: matrix.Config{Homeserver: "https://matrix.org", UserID: "@a:matrix.org", AccessToken: "tok", PickleKey: "k"}}
	var copy *hit
	for _, h := range m.layout().hits {
		if h.w > 0 && h.y > 0 && copy == nil {
			copy = &h
		}
	}
	got, cmd := m.Update(clickAt(*copy))
	require.Equal(t, "copied to clipboard", got.(onboard).msg)
	require.NotNil(t, cmd)
}
