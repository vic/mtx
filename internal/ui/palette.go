package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"mtx/internal/theme"
)

// palette is the 16 base16 slots the UI draws with (base16 styling guide): 00 background, 01 lighter
// background, 02 selection, 03 comments/dim, 04 dark foreground, 05 foreground, 06-07 light, 08 red,
// 09 orange, 0A yellow, 0B green, 0C cyan, 0D blue, 0E magenta, 0F brown. nicks are the nick colors
// and donate the sponsor help button's background: the default keeps today's ANSI picks for them.
type palette struct {
	slots  [16]color.Color
	nicks  []color.Color
	donate color.Color
}

// ansiPalette is the default: ANSI colors, so the terminal theme picks the shades. Slots that serve
// two roles with one ANSI color repeat it (02 and 03 are both 8, the selection and the dim gray).
var ansiPalette = palette{
	slots: [16]color.Color{
		lipgloss.Color("0"), lipgloss.Color("236"), lipgloss.Color("8"), lipgloss.Color("8"),
		lipgloss.Color("7"), lipgloss.Color("15"), lipgloss.Color("15"), lipgloss.Color("15"),
		lipgloss.Color("9"), lipgloss.Color("3"), lipgloss.Color("3"), lipgloss.Color("2"),
		lipgloss.Color("6"), lipgloss.Color("13"), lipgloss.Color("5"), lipgloss.Color("4"),
	},
	nicks: []color.Color{lipgloss.Color("2"), lipgloss.Color("3"), lipgloss.Color("4"), lipgloss.Color("5"),
		lipgloss.Color("6"), lipgloss.Color("10"), lipgloss.Color("11"), lipgloss.Color("12"),
		lipgloss.Color("13"), lipgloss.Color("14")},
	donate: lipgloss.Color("1"),
}

// pal is the palette in use; usePalette sets it and every style built from it.
var pal = ansiPalette

// paletteOf is the palette of a base16 scheme: its "base00".."base0F" colors as hex. Nick colors are
// slots 0A-0F and 09 (not red or gray), and the donate button is slot 08.
func paletteOf(colors map[string]string) palette {
	var p palette
	for i := range p.slots {
		p.slots[i] = lipgloss.Color(colors[hexSlot(i)])
	}
	for _, i := range []int{0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 9} {
		p.nicks = append(p.nicks, p.slots[i])
	}
	p.donate = p.slots[8]
	return p
}

// hexSlot is the base16 name of slot i: "base00".."base0F".
func hexSlot(i int) string {
	const digits = "0123456789ABCDEF"
	return "base0" + string(digits[i])
}

func init() { usePalette(ansiPalette) }

// usePalette makes p the palette: every style is rebuilt from it and the render caches are dropped, so
// the next frame draws in it.
func usePalette(p palette) {
	pal = p
	cell = lipgloss.NewStyle().Inline(true)
	dimmed = fg(pal.slots[3])
	border = dimmed
	focusBorder = fg(pal.slots[0x0D]).Bold(true) // the pane that takes keys
	faint = dimmed
	title = fg(pal.slots[0x0E]).Bold(true)
	shortcut = fg(pal.slots[0x0E])
	prompt = fg(pal.slots[0x0C]).Bold(true)
	unread = fg(pal.slots[0x0A]).Bold(true)
	roomName = fg(pal.slots[0x0D])
	alert = fg(pal.slots[8]).Bold(true)
	current = cell.Background(pal.slots[1])                                      // selected pane row
	cursor = cell.Background(pal.slots[0x0D])                                    // the input cursor: the focused pane border color
	selected = cell.Bold(true).Background(pal.slots[2]).Foreground(pal.slots[5]) // current channel
	helpHead = fg(pal.slots[0x0B]).Bold(true)
	// nickColors leaves out red (bypass) and gray (dim lines).
	nickColors = pal.nicks
	codeSt = fg(pal.slots[0x0C])
	hitStyle = lipgloss.NewStyle().Background(pal.slots[0x0A]).Foreground(pal.slots[0])
	gutter = lipgloss.NewStyle().Background(pal.slots[0x0D]) // the current message's lead cell: the cursor color
	readMark = dimmed.Render("✓")
	pinMark = alert.Render("▴")
	readMarkOn = dimmed.Background(pal.slots[0x0D]).Render("✓")
	pinMarkOn = alert.Background(pal.slots[0x0D]).Render("▴")
	blankOn = gutter.Render(" ")
	unreadMark = unread
	sponsorButton = lipgloss.NewStyle().Foreground(pal.slots[3]).Background(pal.slots[1]).Render(sponsorLabel)
	cursorOn, cursorOff = func() (string, string) {
		parts := strings.SplitN(cursor.Render("\x00"), "\x00", 2)
		return parts[0], parts[1]
	}()
	clearRenderCaches()
}

// clearRenderCaches drops every rendered row and cell, which were drawn in the old palette.
func clearRenderCaches() {
	styled.mu.Lock()
	styled.m = map[styledKey]string{}
	styled.mu.Unlock()
	fitCells.mu.Lock()
	fitCells.m = map[fitKey]string{}
	fitCells.mu.Unlock()
	chanCells.mu.Lock()
	chanCells.m = map[chanKey]string{}
	chanCells.mu.Unlock()
	borderGlyphs.mu.Lock()
	borderGlyphs.m = map[[2]string]string{}
	borderGlyphs.mu.Unlock()
}

// applyTheme makes the theme with ID id (a scheme of internal/theme; "" is the terminal default) the
// one in use, and remembers it as themeNow.
func (a App) applyTheme(id string) App {
	if id == "" {
		usePalette(ansiPalette)
	} else if s, ok := theme.ByID(id); ok {
		usePalette(paletteOf(s.Colors))
	}
	a.themeNow = id
	return a
}
