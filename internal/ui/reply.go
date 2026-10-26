package ui

import (
	"fmt"
	"strings"

	"mtx/internal/model"
)

// WithEdit sets the edit hook: a send while a message is being edited (C) goes through it with the
// target message.
func (a App) WithEdit(f func(ch string, target model.Line, text string) error) App {
	a.editFn = f
	return a
}

// prepare sets the reply, edit or thread target for an INSERT key (i a A C o O T).
func (a App) prepare(k ActKind) App {
	switch k {
	case NewLine: // i: a new message in the current view (thread or main)
		a.quote, a.quoteBody, a.edit = nil, false, nil // i keeps the current thread target
	case ReplyOwn, ReplyOwnQuoted: // a: reply to the selected message, else to my message; A: the same with the quote in the body
		a.edit = nil
		if l, ok := a.selectedTarget(); ok {
			if l.Nick != a.user {
				a = a.mention(l.Nick)
			}
			a.quote, a.quoteBody = &l, k == ReplyOwnQuoted
		} else if l, ok := a.ownTarget(); ok {
			a.quote, a.quoteBody = &l, k == ReplyOwnQuoted
		}
	case EditOwn: // C: edit my message; the input holds its text
		if l, ok := a.ownTarget(); ok {
			a.quote, a.quoteBody = nil, false
			a.edit = &l
			a.in.SetValue(l.Text)
			a.in.MoveToEnd()
		}
	case ReplyThread, ReplyThreadQuoted: // o: reply in the last thread used; O: with the quote
		if a.lastThread == "" || !a.hasHead(a.lastThread) {
			return a
		}
		a = a.selectThread(a.lastThread)
		a.quote, a.quoteBody = nil, false
		if k == ReplyThreadQuoted {
			if l, ok := a.lastInThread(a.lastThread); ok {
				a.quote, a.quoteBody = &l, true
			}
		}
	case ThreadStart: // T: start a thread on the selected message, else the newest
		if l, ok := a.lastWithID(a.hit); ok {
			root := l.Thread
			if root == "" {
				root = l.ID
			}
			a.quote = nil
			a = a.selectThread(root)
		}
	}
	return a
}

// selectedTarget is the selected message when it has an ID and a nick.
func (a App) selectedTarget() (model.Line, bool) {
	ls := a.lines(a.m.Current())
	if a.hit >= 0 && a.hit < len(ls) && ls[a.hit].Nick != "" && ls[a.hit].ID != "" {
		return ls[a.hit], true
	}
	return model.Line{}, false
}

// ownTarget is the selected message when it is mine, else my newest message with an ID.
func (a App) ownTarget() (model.Line, bool) {
	ls := a.lines(a.m.Current())
	if a.hit >= 0 && a.hit < len(ls) && ls[a.hit].Nick == a.user && ls[a.hit].ID != "" {
		return ls[a.hit], true
	}
	for i := len(ls) - 1; i >= 0; i-- {
		if ls[i].Nick == a.user && ls[i].ID != "" {
			return ls[i], true
		}
	}
	return model.Line{}, false
}

// lastInThread is the newest message with an ID in thread root.
func (a App) lastInThread(root string) (model.Line, bool) {
	ls := a.lines(a.m.Current())
	for i := len(ls) - 1; i >= 0; i-- {
		if ls[i].ID != "" && (ls[i].Thread == root || ls[i].ID == root) {
			return ls[i], true
		}
	}
	return model.Line{}, false
}

// quotePrefix is the markdown quote of l put above a quoted reply's text.
func quotePrefix(l model.Line) string {
	return fmt.Sprintf("> <%s> %s\n", l.Nick, strings.ReplaceAll(l.Text, "\n", "\n> "))
}
