package main

import (
	"os/exec"
	"strings"
)

// notifyBin is the desktop notifier; "" when none is installed (then notifications are off).
var notifyBin, _ = exec.LookPath("notify-send")

// wantsNotify reports whether a live line in channel ch from nick should notify me (me = my localpart):
// a DM from someone else, or a line that mentions @me.
func wantsNotify(ch, nick, text, me string) bool {
	if nick == "" || nick == me {
		return false
	}
	return strings.HasPrefix(ch, "@") || mentionsMe(text, me)
}

// mentionsMe reports whether text has @me as a whole word.
func mentionsMe(text, me string) bool {
	for _, f := range strings.Fields(text) {
		if strings.TrimRight(f, ",.:;!?") == "@"+me {
			return true
		}
	}
	return false
}

// notify shows a desktop notification, off the caller's goroutine; errors are ignored.
func notify(title, body string) {
	if notifyBin == "" {
		return
	}
	go func() { _ = exec.Command(notifyBin, "--app-name=mtx", title, body).Run() }()
}
