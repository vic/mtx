package model

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
)

var (
	// ErrUnknownChannel means no channel has the given name.
	ErrUnknownChannel = errors.New("model: unknown channel")
	// ErrBadIndex means a channel index outside the list.
	ErrBadIndex = errors.New("model: channel index out of range")
)

// AddLine appends l to channel ch, counting it unread unless ch is focused.
// The returned Model gives ch its own Ring; m is never modified.
func (m Model) AddLine(ch string, l Line) (Model, error) { return m.add(ch, l, true) }

// AddHistory appends l like AddLine but as history: no unread count, and the
// channel keeps its place.
func (m Model) AddHistory(ch string, l Line) (Model, error) { return m.add(ch, l, false) }

func (m Model) add(ch string, l Line, live bool) (Model, error) {
	for i, c := range m.chans {
		if c.Name != ch {
			continue
		}
		l.Nick, l.Text = clean(l.Nick), cleanText(l.Text)
		c.Lines.Push(l)
		if !live {
			return m.withChan(i, c), nil
		}
		if i != m.cur {
			c.Unread++
		}
		return m.withChan(i, c).toFront(i), nil
	}
	return m, fmt.Errorf("%w: %s", ErrUnknownChannel, ch)
}

// Focus makes channel i current and clears its unread count.
func (m Model) Focus(i int) (Model, error) {
	if i < 0 || i >= len(m.chans) {
		return m, fmt.Errorf("%w: %d", ErrBadIndex, i)
	}
	c := m.chans[i]
	c.Unread = 0
	n := m.withChan(i, c)
	n.cur = i
	n.ver++
	return n, nil
}

// Current returns the focused channel, or the zero Channel when there are none.
func (m Model) Current() Channel {
	if m.cur < 0 || m.cur >= len(m.chans) {
		return Channel{}
	}
	return m.chans[m.cur]
}

func (m Model) withChan(i int, c Channel) Model {
	m.chans = append([]Channel(nil), m.chans...)
	m.chans[i] = c
	m.ver++
	return m
}

// clean makes untrusted text safe to draw: invalid UTF-8 becomes U+FFFD, tab and
// line breaks become spaces, and every other control (C0, DEL, C1: ESC, BEL, CSI)
// and format char (bidi override, zero width) is dropped. Applied at the write
// site so no view can render a terminal escape from a peer line.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			return ' '
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r):
			return -1
		}
		return r
	}, strings.ToValidUTF8(s, "�"))
}

// toFront moves channel i to index 0 (most recent first) and keeps focus on
// the same channel.
func (m Model) toFront(i int) Model {
	c := m.chans[i]
	m.chans = append([]Channel{c}, slices.Delete(slices.Clone(m.chans), i, i+1)...)
	m.ver++
	switch {
	case m.cur == i:
		m.cur = 0
	case m.cur < i:
		m.cur++
	}
	return m
}

// cleanText is clean for message text: it keeps line breaks and indentation (tab =
// 4 spaces) so agent replies keep their layout; other control runes still go.
func cleanText(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\t", "    ")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = clean(l)
	}
	return strings.Join(lines, "\n")
}

// AddHistoryLines merges ls into channel ch as history, as one update: the scrollback stays in time
// order (history usually arrives after live lines) and a line whose ID is already there is skipped.
func (m Model) AddHistoryLines(ch string, ls []Line) (Model, error) {
	for i, c := range m.chans {
		if c.Name != ch {
			continue
		}
		all := c.Lines.All()
		have := make(map[string]bool, len(all))
		for _, l := range all {
			if l.ID != "" {
				have[l.ID] = true
			}
		}
		for _, l := range ls {
			if l.ID != "" && have[l.ID] {
				continue
			}
			l.Nick, l.Text = clean(l.Nick), cleanText(l.Text)
			all = append(all, l)
		}
		slices.SortStableFunc(all, func(a, b Line) int { return a.At.Compare(b.At) })
		c.Lines = RingOf[Line](m.cap)
		for _, l := range all {
			c.Lines.Push(l)
		}
		return m.withChan(i, c), nil
	}
	return m, fmt.Errorf("%w: %s", ErrUnknownChannel, ch)
}

// ReplaceLine swaps the line with the given ID in channel ch for l (a decrypted placeholder, or an edit).
// No line with that ID is a no-op.
func (m Model) ReplaceLine(ch, id string, l Line) (Model, error) {
	for i, c := range m.chans {
		if c.Name != ch {
			continue
		}
		c.Lines = c.Lines.clone()
		c.Lines.Replace(func(old Line) bool { return old.ID == id }, func(old Line) Line {
			l.At, l.Thread = old.At, old.Thread // a decrypt or an edit changes the text, not the place
			l.Nick, l.Text = clean(l.Nick), cleanText(l.Text)
			return l
		})
		return m.withChan(i, c), nil
	}
	return m, fmt.Errorf("%w: %s", ErrUnknownChannel, ch)
}

// SortRecent orders the channels by their newest line, newest first; channels with no lines keep their
// order after them. Focus stays on the same channel.
func (m Model) SortRecent() Model {
	newest := func(c Channel) time.Time {
		if c.Lines.Len() == 0 {
			return time.Time{}
		}
		return c.Lines.At(c.Lines.Len() - 1).At
	}
	cur := m.Current().Name
	chans := slices.Clone(m.chans)
	slices.SortStableFunc(chans, func(a, b Channel) int { return newest(b).Compare(newest(a)) })
	m.chans = chans
	for i, c := range chans {
		if c.Name == cur {
			m.cur = i
		}
	}
	m.ver++
	return m
}

// RemoveLine drops the line with the given ID from channel ch (a placeholder whose event turned out
// to be an edit). No line with that ID is a no-op.
func (m Model) RemoveLine(ch, id string) (Model, error) {
	for i, c := range m.chans {
		if c.Name != ch {
			continue
		}
		all := slices.DeleteFunc(c.Lines.All(), func(l Line) bool { return l.ID == id })
		c.Lines = RingOf[Line](m.cap)
		for _, l := range all {
			c.Lines.Push(l)
		}
		return m.withChan(i, c), nil
	}
	return m, fmt.Errorf("%w: %s", ErrUnknownChannel, ch)
}

// SetReacts sets the reaction summary of the line with the given ID in channel ch. No such line is a no-op.
func (m Model) SetSeen(ch, id, seen string) (Model, error) {
	for i, c := range m.chans {
		if c.Name != ch {
			continue
		}
		c.Lines = c.Lines.clone()
		c.Lines.Replace(func(l Line) bool { return l.ID == id }, func(l Line) Line { l.Seen = clean(seen); return l })
		return m.withChan(i, c), nil
	}
	return m, fmt.Errorf("%w: %s", ErrUnknownChannel, ch)
}

func (m Model) SetReacts(ch, id, reacts string) (Model, error) {
	for i, c := range m.chans {
		if c.Name != ch {
			continue
		}
		c.Lines = c.Lines.clone()
		c.Lines.Replace(func(l Line) bool { return l.ID == id }, func(l Line) Line { l.Reacts = clean(reacts); return l })
		return m.withChan(i, c), nil
	}
	return m, fmt.Errorf("%w: %s", ErrUnknownChannel, ch)
}
