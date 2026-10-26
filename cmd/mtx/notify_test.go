package main

import (
	"testing"
)

// TestWantsNotify: a DM from someone else, or a line that mentions me as a whole word, notifies; my own lines do not.
func TestWantsNotify(t *testing.T) {
	tests := []struct {
		ch, nick, text, me string
		want               bool
	}{
		{"@ana", "ana", "hi", "me", true},
		{"#r", "ana", "hi @me!", "me", true},
		{"#r", "ana", "hi", "me", false},
		{"#r", "me", "@me", "me", false},
		{"#r", "", "x", "me", false},
		{"#r", "ana", "@meow", "me", false},
	}
	for _, tc := range tests {
		if got := wantsNotify(tc.ch, tc.nick, tc.text, tc.me); got != tc.want {
			t.Fatalf("wantsNotify(%q, %q, %q, %q) = %v, want %v", tc.ch, tc.nick, tc.text, tc.me, got, tc.want)
		}
	}
}
