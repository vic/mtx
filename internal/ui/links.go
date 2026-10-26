package ui

import (
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	repoURL   = "https://github.com/vic/mtx"
	donateURL = "https://github.com/sponsors/vic"
)

// helpButtons are the two links at the top right of the help box: label, URL, background.
var helpButtons = []struct {
	label, url string
	bg         string
}{
	{"github:vic/mtx", repoURL, "8"},
	{"sponsor @vic", donateURL, "1"},
}

// renderHelpButtons is the help buttons row: white text on each button's background.
func renderHelpButtons() string {
	var parts []string
	for _, b := range helpButtons {
		parts = append(parts, lipgloss.NewStyle().Background(lipgloss.Color(b.bg)).Foreground(lipgloss.Color("15")).Render(" "+b.label+" "))
	}
	return strings.Join(parts, " ")
}

// openURL opens url with $BROWSER, else xdg-open, else open. It does not wait for the browser.
func openURL(url string) tea.Cmd {
	var candidates []string
	if b := os.Getenv("BROWSER"); b != "" {
		candidates = append(candidates, strings.Split(b, ":")[0])
	}
	candidates = append(candidates, "xdg-open", "open")
	for _, c := range candidates {
		path, err := exec.LookPath(c)
		if err != nil {
			continue
		}
		return func() tea.Msg {
			if err := exec.Command(path, url).Start(); err != nil {
				return ToastMsg{Text: "open " + url + ": " + err.Error(), Err: true}
			}
			return nil
		}
	}
	return func() tea.Msg {
		return ToastMsg{Text: "no browser: set $BROWSER or install xdg-open", Err: true}
	}
}

// helpButtonAt is the URL of the help button at screen cell (x, y), if any. The buttons are on the
// first text row of the help box.
func (a App) helpButtonAt(x, y int) (string, bool) {
	bl := strings.Split(helpBox(a.h, a.helpOff), "\n")
	if len(bl) < 2 {
		return "", false
	}
	bw := lipgloss.Width(bl[0])
	top, left := max((a.h-len(bl))/2, 0), max((a.w-bw)/2, 0)
	if y != top+1 {
		return "", false
	}
	row := ansi.Strip(bl[1])
	for _, b := range helpButtons {
		idx := strings.Index(row, b.label)
		if idx < 0 {
			continue
		}
		x0 := left + lipgloss.Width(row[:idx]) - 1
		if x >= x0 && x < x0+lipgloss.Width(b.label)+2 {
			return b.url, true
		}
	}
	return "", false
}

// helpClick handles a left click on the help box: a button opens its URL.
func (a App) helpClick(m tea.Mouse) (App, tea.Cmd) {
	if url, ok := a.helpButtonAt(m.X, m.Y); ok {
		return a, openURL(url)
	}
	return a, nil
}

// sponsorLabel is the muted button at the bottom right of the main screen.
const sponsorLabel = " sponsor @vic "

// sponsorButton is sponsorLabel in the muted button style.
var sponsorButton = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Background(lipgloss.Color("236")).Render(sponsorLabel)

// withSponsor draws sponsorButton at the right end of the screen's last row (w columns wide).
func withSponsor(screen string, w int) string {
	bw := lipgloss.Width(sponsorLabel)
	if w < bw+10 {
		return screen
	}
	i := strings.LastIndexByte(screen, '\n') + 1
	return screen[:i] + ansi.Cut(screen[i:], 0, w-bw) + sponsorButton
}

// onSponsor reports whether screen cell (x, y) is on the sponsor button.
func (a App) onSponsor(x, y int) bool {
	bw := lipgloss.Width(sponsorLabel)
	return a.w >= bw+10 && y == a.h-1 && x >= a.w-bw
}
