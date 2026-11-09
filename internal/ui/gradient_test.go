package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestGradLevel(t *testing.T) {
	r := gradRect{10, 0, 30, 8}
	for _, c := range []struct{ x, y, want int }{{10, 0, 0}, {30, 8, gradLevels - 1}, {30, 0, 4}, {20, 4, -1}} {
		if got := r.level(c.x, c.y); got != c.want {
			t.Errorf("level(%d,%d) = %d, want %d", c.x, c.y, got, c.want)
		}
	}
}

// A theme draws the focused frame from the first step at its top left to the last at its bottom right,
// and the gradient never changes a glyph.
func TestFocusedFrameGradient(t *testing.T) {
	a := scrollApp(t)
	plain := ansi.Strip(a.screen())
	if strings.Contains(a.screen(), "38;2;") {
		t.Fatal("NEG the 16-color palette has no gradient")
	}
	t.Cleanup(func() { usePalette(ansiPalette) })
	usePalette(paletteOf(gruvboxDark))
	rows := strings.Split(a.screen(), "\n")
	if len(gradPre) != gradLevels {
		t.Fatalf("a theme blends %d steps, not %d", len(gradPre), gradLevels)
	}
	page, _ := a.chatRows()
	if !strings.Contains(rows[0], gradPre[0]) || !strings.Contains(rows[page+1], gradPre[gradLevels-1]) {
		t.Fatalf("top %q bottom %q", rows[0], rows[page+1])
	}
	mid := gradRect{chanW + 1, 0, a.w - nickW - 2, page + 1}
	if l := mid.level(chanW+1, page/2); !strings.Contains(rows[page/2], gradPre[l]) {
		t.Fatalf("the left edge of row %d takes step %d: %q", page/2, l, rows[page/2])
	}
	if got := ansi.Strip(strings.Join(rows, "\n")); got != plain {
		t.Fatal("the gradient changed the text")
	}
}
