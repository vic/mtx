package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

// TestURLAtClick: a click on a link's word gives its URL; on other words, none.
func TestURLAtClick(t *testing.T) {
	row := "see https://example.com/x, here"
	require.Equal(t, "https://example.com/x", urlAt(row, 6))
	require.Equal(t, "", urlAt(row, 1))
	require.Equal(t, "", urlAt(row, 26))
}

// TestURLAtMarkdownLink: a click anywhere on a markdown link opens its URL.
func TestURLAtMarkdownLink(t *testing.T) {
	row := "see [the docs](https://x.org/a) now"
	for _, col := range []int{4, 8, 20, 30} { // on "[", inside the text, inside the URL, on ")"
		require.Equal(t, "https://x.org/a", urlAt(row, col), "col %d", col)
	}
	require.Equal(t, "", urlAt(row, 0))
}

// TestSponsorButton: the bottom right of the main screen shows the sponsor button; a click on it is on the button.
func TestSponsorButton(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	a.w, a.h = 120, 40
	rows := strings.Split(ansi.Strip(a.View().Content), "\n")
	last := rows[len(rows)-1]
	require.True(t, strings.HasSuffix(last, strings.TrimRight(sponsorLabel, " ")+" "), "last row %q", last)
	require.True(t, a.onSponsor(119, 39))
	require.False(t, a.onSponsor(119, 38))
	require.False(t, a.onSponsor(0, 39))
}

// TestReactAt: a click on an emoji of a reaction summary gives its key; elsewhere, none.
func TestReactAt(t *testing.T) {
	row := "<bob>      hi  [👍2 ❤️1]"
	at := strings.Index(row, "👍")
	if got := reactAt(row, lipgloss.Width(row[:at])); got != "👍" {
		t.Fatalf("👍: %q", got)
	}
	at = strings.Index(row, "❤️")
	if got := reactAt(row, lipgloss.Width(row[:at])); got != "❤️" {
		t.Fatalf("❤️: %q", got)
	}
	if got := reactAt(row, 2); got != "" {
		t.Fatalf("nick: %q", got)
	}
	if got := reactAt("<bob> no reacts", 3); got != "" {
		t.Fatalf("plain: %q", got)
	}
}

// TestReactAtWrapped: a summary that wraps is clickable on every row: the first row without its closing
// "]", the continuation rows by their own tokens.
func TestReactAtWrapped(t *testing.T) {
	if got := reactAt("hi  [👍2 ❤️1", 5); got != "👍" {
		t.Fatalf("first row %q", got)
	}
	if got := reactAtCont("           😂3 🎉1]", 15); got != "🎉" {
		t.Fatalf("continuation %q", got)
	}
	if got := reactAtCont("           😂3 🎉1]", 0); got != "" {
		t.Fatalf("indentation %q", got)
	}
}
