package model

import (
	"fmt"
	"time"
)

// SessionInfo is the live registry data the modeline shows for one pid.
type SessionInfo struct {
	Status, PermMode string
}

// Modeline is the line above the input (D:323-327).
type Modeline struct {
	Mode        string // NORMAL / INSERT, drawn as a badge
	Left, Right string
	Red         bool
}

// rateWindow is the span lines/min counts over (D:359).
const rateWindow = time.Minute

// Status builds the modeline for the focused channel. mode is the edit mode label. A session
// channel shows its registry status (info[PID]); a Matrix room shows its members, rate and ID.
// lines/min counts only the lines still in the ring, so it undercounts above its cap.
// Unread totals every channel; now is injected so the function stays pure.
func Status(m Model, mode string, info map[int]SessionInfo, now time.Time) Modeline {
	unread := 0
	for _, c := range m.chans {
		unread += c.Unread
	}
	ml := Modeline{Mode: mode, Right: fmt.Sprintf("%d unread %s", unread, now.Format("15:04"))}
	c := m.Current()
	switch {
	case c.Name == "":
	case c.Kind == RoomChan && c.ID != "": // a Matrix room
		ml.Left = fmt.Sprintf("%s · %d members · %d/min · %s", c.Name, len(c.Members), roomRate(c, now), c.ID)
	case c.Kind == RoomChan:
		ml.Left = fmt.Sprintf("%s %d members %d/min", c.Name, len(c.Members), roomRate(c, now))
	default:
		st := "?" // untrusted registry text: the status is cleaned before it prints
		if s, ok := info[c.PID]; ok {
			st = clean(s.Status)
		}
		ml.Left = fmt.Sprintf("%s %s", c.Name, st)
	}
	return ml
}

// roomRate is how many of c's lines are from the last rateWindow.
func roomRate(c Channel, now time.Time) int {
	rate := 0
	for i := range c.Lines.Len() { // At, not All: no copy of the ring per frame
		if d := now.Sub(c.Lines.At(i).At); d >= 0 && d < rateWindow {
			rate++
		}
	}
	return rate
}
