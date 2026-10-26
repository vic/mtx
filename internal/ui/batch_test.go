package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

// TestCoalesceRunsSumsRuns: a run of notches is one step, and a change of direction or another
// message starts a new step.
func TestCoalesceRunsSumsRuns(t *testing.T) {
	down := tea.MouseWheelMsg{Button: tea.MouseWheelDown}
	up := tea.MouseWheelMsg{Button: tea.MouseWheelUp}
	var in []tea.Msg
	for i := 0; i < 12; i++ {
		in = append(in, down)
	}
	in = append(in, up, tea.MouseClickMsg{Button: tea.MouseLeft})
	got := coalesceRuns(in)
	require.Len(t, got, 3)
	require.Equal(t, 12, got[0].(WheelMsg).N)
	require.Equal(t, -1, got[1].(WheelMsg).N)
	require.Equal(t, tea.MouseClickMsg{Button: tea.MouseLeft}, got[2])
}

// TestCoalesceRunsSumsRepeatedKeys: a run of j is one KeyRun; other keys pass through.
func TestCoalesceRunsSumsRepeatedKeys(t *testing.T) {
	j := tea.KeyPressMsg{Code: 'j', Text: "j"}
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	got := coalesceRuns([]tea.Msg{j, j, j, j, j, j, enter})
	require.Len(t, got, 2)
	require.Equal(t, 6, got[0].(KeyRun).N)
	require.Equal(t, enter, got[1])
}
