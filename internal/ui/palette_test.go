package ui

import (
	"strings"
	"testing"
)

// gruvboxDark is the base16 gruvbox dark hard scheme, inline.
var gruvboxDark = map[string]string{
	"base00": "#1d2021", "base01": "#3c3836", "base02": "#504945", "base03": "#665c54",
	"base04": "#bdae93", "base05": "#d5c4a1", "base06": "#ebdbb2", "base07": "#fbf1c7",
	"base08": "#fb4934", "base09": "#fe8019", "base0A": "#fabd2f", "base0B": "#b8bb26",
	"base0C": "#8ec07c", "base0D": "#83a598", "base0E": "#d3869b", "base0F": "#d65d0e",
}

func TestUsePaletteRestyles(t *testing.T) {
	t.Cleanup(func() { usePalette(ansiPalette) })
	usePalette(paletteOf(gruvboxDark))
	// base03 is 102 92 84: the dim style must draw in it
	if got := dimmed.Render("x"); !strings.Contains(got, "102;92;84") {
		t.Fatalf("dimmed in gruvbox: %q", got)
	}
	usePalette(ansiPalette)
	if got := dimmed.Render("x"); strings.Contains(got, "102;92;84") {
		t.Fatalf("dimmed still gruvbox: %q", got)
	}
}
