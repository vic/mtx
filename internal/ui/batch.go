package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// frameEvery is the coalescing window: at most one batch per frame (~60 fps). Like jjui's
// debounce, a burst of events from sync and history is one update and one render, not one each.
const frameEvery = 60 * time.Millisecond

// BatchMsg carries the updates that arrived within one frame. Update applies them in order, then
// the view renders once.
type BatchMsg []tea.Msg

// Coalesce returns a send func that queues messages and passes them on as one BatchMsg per frame.
// send is only called from the coalescer's goroutine.
func Coalesce(send func(tea.Msg)) func(tea.Msg) {
	in := make(chan tea.Msg, 4096)
	go func() {
		tick := time.NewTicker(frameEvery)
		defer tick.Stop()
		var buf BatchMsg
		for {
			select {
			case m := <-in:
				buf = append(buf, m)
			case <-tick.C:
				if len(buf) > 0 {
					send(buf)
					buf = nil
				}
			}
		}
	}()
	return func(m tea.Msg) { in <- m }
}

// wheelBatch is how many notches one WheelMsg step carries: a burst is computed in steps of 5.
const wheelBatch = 5

// WheelMsg is wheel notches summed over a run: N > 0 down, N < 0 up.
type WheelMsg struct {
	M tea.MouseWheelMsg
	N int
}

// notch is the direction of a wheel button: -1 up, 1 down, 0 other.
func notch(b tea.MouseButton) int {
	switch b {
	case tea.MouseWheelUp:
		return -1
	case tea.MouseWheelDown:
		return 1
	}
	return 0
}

// KeyRun is N repeats of one key (j or k), applied as one update.
type KeyRun struct {
	M tea.KeyPressMsg
	N int
}

// coalesceRuns sums same-direction wheel notches and repeated j/k keys into steps of up to wheelBatch.
// Any other message ends a run and passes through unchanged.
func coalesceRuns(in []tea.Msg) []tea.Msg {
	var out []tea.Msg
	var w *WheelMsg
	var k *KeyRun
	flush := func() {
		if w != nil {
			out = append(out, *w)
			w = nil
		}
		if k != nil {
			out = append(out, *k)
			k = nil
		}
	}
	for _, m := range in {
		switch m := m.(type) {
		case tea.MouseWheelMsg:
			n := notch(m.Button)
			if n == 0 {
				continue
			}
			if k != nil {
				flush()
			}
			if w == nil {
				w = &WheelMsg{}
			} else if (w.N > 0) != (n > 0) || w.N >= wheelBatch || w.N <= -wheelBatch {
				flush()
				w = &WheelMsg{}
			}
			w.M, w.N = m, w.N+n
		case tea.KeyPressMsg:
			if s := m.String(); s != "j" && s != "k" {
				flush()
				out = append(out, m)
				continue
			}
			if w != nil {
				flush()
			}
			if k != nil && (k.M.String() != m.String() || k.N >= wheelBatch) {
				flush()
			}
			if k == nil {
				k = &KeyRun{M: m}
			}
			k.N++
		default:
			flush()
			out = append(out, m)
		}
	}
	flush()
	return out
}
