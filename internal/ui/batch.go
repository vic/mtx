package ui

import (
	tea "charm.land/bubbletea/v2"
)

// BatchMsg carries the updates that arrived within one frame. Update applies them in order, then
// the view renders once.
type BatchMsg []tea.Msg

// Coalesce returns an enqueue func for input events. The coalescer goroutine sends each batch as
// soon as it has one: what queued while the previous batch was being applied goes out together.
// enqueue never blocks: it returns false when the queue is full, and the caller passes the event on.
func Coalesce(send func(tea.Msg)) func(tea.Msg) bool {
	in := make(chan tea.Msg, 4096)
	go func() {
		for m := range in {
			buf := BatchMsg{m}
			for more := true; more; {
				select {
				case m := <-in:
					buf = append(buf, m)
				default:
					more = false
				}
			}
			send(buf)
		}
	}()
	return func(m tea.Msg) bool {
		select {
		case in <- m:
			return true
		default:
			return false
		}
	}
}

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

// coalesceRuns sums same-direction wheel notches and repeated j/k keys into one step per run.
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
			} else if (w.N > 0) != (n > 0) {
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
			if k != nil && k.M.String() != m.String() {
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
