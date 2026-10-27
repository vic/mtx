package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"mtx/internal/model"
)

// zenRowWords are the words zen's one row may show besides the state values: the mode, the keys SPC, ? and z with their labels, and the sponsor button.
var zenRowWords = []string{"NORMAL", "INSERT", "CHANNELS", "THREADS", "NICKS", "SPC", "menu", "?", "help", "z", "zen:on", "zen:off", "sponsor", "@vic"}

// zenStateWords are the prefixes of the state values the one row may show.
var zenStateWords = []string{"thread:", "mentions:", "time:", "speaker:", "grep:"}

// zenRowOK reports whether screen is zen at its bottom: the bottom row carries SPC, menu, ?, help and z, no other usage hint, and the row above it holds content.
func zenRowOK(screen, content string) bool {
	rows := strings.Split(ansi.Strip(screen), "\n")
	bottom, above := rows[len(rows)-1], rows[len(rows)-2]
	words := strings.Fields(bottom)
	for _, k := range []string{"SPC", "menu", "?", "help", "z"} {
		if !slices.Contains(words, k) {
			return false
		}
	}
	for _, w := range words {
		if !slices.Contains(zenRowWords, w) && !slices.ContainsFunc(zenStateWords, func(p string) bool { return strings.HasPrefix(w, p) }) {
			return false
		}
	}
	return strings.Contains(above, content)
}

// TestZenRow: in zen the bottom row is the one modeline row, and the row above it is content.
func TestZenRow(t *testing.T) {
	prev := zen
	zen = false
	t.Cleanup(func() { zen = prev })
	var log []sent
	a := newApp(t, &log, nil)
	a.w, a.h = 120, 30
	for i := range 20 { // long lines wrap, so the chat fills the rows and the newest ends on the row above the bottom
		text := fmt.Sprintf("msg %d %sEND%d", i, strings.Repeat("lorem ipsum ", 30), i)
		next, _ := a.Update(LineMsg{Ch: "#a", L: model.Line{Nick: "bob", Text: text}})
		a = next.(App)
	}
	nonZen := a.screen() // negative control: the same content without z
	a, _ = press(t, a, "z")
	if !zenRowOK(a.screen(), "END19") {
		t.Fatalf("zen bottom row:\n%s", ansi.Strip(a.screen()))
	}
	// hit-NORMAL and a focus pane keep SPC menu and ? help in zen too
	for _, s := range []struct {
		name string
		set  func(App) App
	}{
		{"hit", func(a App) App { a.hit = 3; return a }},
		{"focus", func(a App) App { a.focus = focusChans; return a }},
	} {
		if b := s.set(a); !zenRowOK(b.screen(), "END19") {
			t.Fatalf("zen %s row:\n%s", s.name, ansi.Strip(b.screen()))
		}
	}
	// the non-zen screen still shows its own footer, so it fails the check for the zen reason
	bottom := strings.Split(ansi.Strip(nonZen), "\n")
	if last := bottom[len(bottom)-1]; !strings.Contains(last, "C-g grep:off") || !strings.Contains(last, "/ search") {
		t.Fatalf("non-zen footer missing:\n%s", ansi.Strip(nonZen))
	}
	if zenRowOK(nonZen, "END19") {
		t.Fatalf("non-zen screen passes the zen check:\n%s", ansi.Strip(nonZen))
	}
}
