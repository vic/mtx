package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"mtx/internal/matrix"
)

// logo is ANSI Shadow "mtx".
const logo = `
███╗   ███╗████████╗██╗  ██╗
████╗ ████║╚══██╔══╝╚██╗██╔╝
██╔████╔██║   ██║    ╚███╔╝
██║╚██╔╝██║   ██║    ██╔██╗
██║ ╚═╝ ██║   ██║   ██╔╝ ██╗
╚═╝     ╚═╝   ╚═╝   ╚═╝  ╚═╝`

type onboardStage int

const (
	stageForm onboardStage = iota // homeserver, user ID, SSO button
	stageWait                     // browser login running
	stageSave                     // logged in: show what to save as env
)

// Focus rows of the form, in tab order.
const (
	focusHS = iota
	focusUser
	focusSSO
	focusQuit
	focusCount
)

// Placeholders are suggestions. An empty homeserver field uses its placeholder.
const (
	hsPlaceholder   = "https://matrix.org"
	userPlaceholder = "@you:matrix.org"
)

// helpBorder is the border color of the help box; inputs and buttons use the same frame.
var helpBorder = lipgloss.Color("8")

// loginURL carries the authorization URL to the screen while the browser login waits.
type loginURL string

// loginDone is the result of the browser login.
type loginDone struct {
	sess matrix.Session
	err  error
}

// onboard is the first-run screen: no MTX_* env is set. It collects the
// homeserver and user ID, runs SSO, and on success makes a new pickle key and shows the env to save.
type onboard struct {
	hs, user string
	store    string
	focus    int
	stage    onboardStage
	url      string
	msg      string
	cancel   context.CancelFunc
	show     func(authURL string) // set by runOnboard: sends loginURL to the program
	cfg      matrix.Config        // set when stage is stageSave
}

// hit is a clickable box on screen: its rectangle and what a click does.
type hit struct {
	x, y, w, h int
	act        func(onboard) (tea.Model, tea.Cmd)
}

func (m onboard) Init() tea.Cmd { return nil }

func (m onboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.keyPress(msg)
	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft {
			return m, nil
		}
		for _, h := range m.layout().hits {
			if msg.X >= h.x && msg.X < h.x+h.w && msg.Y >= h.y && msg.Y < h.y+h.h {
				return h.act(m)
			}
		}
		return m, nil
	case loginURL:
		m.url = string(msg)
		return m, nil
	case loginDone:
		if m.stage != stageWait {
			return m, nil // cancelled earlier
		}
		m.cancel()
		m.url = ""
		if msg.err != nil {
			m.stage, m.msg = stageForm, msg.err.Error()
			return m, nil
		}
		key, err := matrix.NewPickleKey()
		if err != nil {
			m.stage, m.msg = stageForm, err.Error()
			return m, nil
		}
		m.cfg = matrix.Config{
			Homeserver:  msg.sess.Homeserver,
			UserID:      msg.sess.UserID,
			AccessToken: msg.sess.AccessToken,
			DeviceID:    msg.sess.DeviceID,
			PickleKey:   key,
			DBPath:      m.store,
		}
		m.stage, m.msg = stageSave, ""
		return m, nil
	}
	return m, nil
}

func (m onboard) keyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "ctrl+c" {
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
	}
	switch m.stage {
	case stageSave:
		if k == "enter" {
			return m, tea.Quit
		}
	case stageWait:
		if k == "esc" {
			return m.cancelLogin()
		}
	case stageForm:
		return m.formKey(k, msg.Text)
	}
	return m, nil
}

func (m onboard) cancelLogin() (tea.Model, tea.Cmd) {
	m.cancel()
	m.stage, m.url, m.msg = stageForm, "", "login cancelled"
	return m, nil
}

func (m onboard) formKey(k, text string) (tea.Model, tea.Cmd) {
	switch k {
	case "esc":
		return m, tea.Quit
	case "tab", "down":
		m.focus = (m.focus + 1) % focusCount
	case "shift+tab", "up":
		m.focus = (m.focus + focusCount - 1) % focusCount
	case "enter":
		if m.focus == focusQuit {
			return m, tea.Quit
		}
		if m.focus != focusSSO {
			m.focus++
			return m, nil
		}
		return m.startLogin()
	case "backspace":
		m.setField(func(s string) string {
			if s == "" {
				return s
			}
			r := []rune(s)
			return string(r[:len(r)-1])
		})
	default:
		if m.focus != focusSSO && m.focus != focusQuit && text != "" {
			m.setField(func(s string) string { return s + text })
		}
	}
	return m, nil
}

func (m *onboard) setField(edit func(string) string) {
	switch m.focus {
	case focusHS:
		m.hs = edit(m.hs)
	case focusUser:
		m.user = edit(m.user)
	}
}

// startLogin checks the form and starts the browser login. The login runs as a tea.Cmd.
func (m onboard) startLogin() (tea.Model, tea.Cmd) {
	hs := m.hs
	if hs == "" {
		hs = hsPlaceholder
	}
	if err := matrix.ValidateHomeserver(hs); err != nil {
		m.msg = err.Error()
		return m, nil
	}
	if err := matrix.ValidateUserID(m.user); err != nil {
		m.msg = err.Error()
		return m, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.stage, m.msg = stageWait, ""
	return m, func() tea.Msg {
		s, err := matrix.Authenticate(ctx, hs, m.show)
		return loginDone{sess: s, err: err}
	}
}

func (m onboard) View() tea.View {
	v := tea.NewView(m.layout().screen)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// page builds the screen top to bottom and records where each clickable box lands.
type page struct {
	lines []string
	hits  []hit
}

func (p *page) add(block string, act func(onboard) (tea.Model, tea.Cmd)) {
	if act != nil {
		p.hits = append(p.hits, hit{x: 0, y: len(p.lines), w: lipgloss.Width(block), h: lipgloss.Height(block), act: act})
	}
	p.lines = append(p.lines, strings.Split(block, "\n")...)
}

func (p *page) text(lines ...string) {
	p.lines = append(p.lines, lines...)
}

type layoutResult struct {
	screen string
	hits   []hit
}

func (m onboard) layout() layoutResult {
	p := &page{}
	p.text(strings.Split(logo, "\n")[1:]...)
	p.text("", "  A terminal Matrix client. mtx never asks for or stores your password.", "")
	switch m.stage {
	case stageForm:
		m.formPage(p)
	case stageWait:
		p.text("  Waiting for the browser to finish login.", "  If no browser opened, open this link:", "", "  "+m.url, "")
		p.add(button("Cancel", false, false), func(m onboard) (tea.Model, tea.Cmd) { return m.cancelLogin() })
	case stageSave:
		m.savePage(p)
	}
	if m.msg != "" {
		p.text("", "  ! "+m.msg)
	}
	return layoutResult{screen: strings.Join(p.lines, "\n"), hits: p.hits}
}

func (m onboard) formPage(p *page) {
	p.text("  Homeserver")
	p.add(m.input(focusHS, m.hs, hsPlaceholder), focusAct(focusHS))
	p.text("  User ID")
	p.add(m.input(focusUser, m.user, userPlaceholder), focusAct(focusUser))
	p.text("")
	p.add(button("Login with SSO", true, m.focus == focusSSO), func(m onboard) (tea.Model, tea.Cmd) {
		m.focus = focusSSO
		return m.startLogin()
	})
	p.add(button("Quit", false, m.focus == focusQuit), func(m onboard) (tea.Model, tea.Cmd) {
		return m, tea.Quit
	})
	p.text("", "  tab: move   enter: next or login   esc: quit")
}

func (m onboard) savePage(p *page) {
	p.text("  Logged in. Save the pickle key now. mtx keeps everything else in the store, never the pickle key.", "")
	p.code("bash", exportLines(m.cfg))
	p.text("")
	p.add(button("Copy", false, false), func(m onboard) (tea.Model, tea.Cmd) {
		m.msg = "copied to clipboard"
		return m, tea.SetClipboard(strings.Join(exportLines(m.cfg), "\n"))
	})
	p.text("", "  The pickle key is a secret. Keep it out of git.",
		"  Lose the pickle key and the store at "+m.cfg.DBPath+" is unreadable.", "")
	p.add(button("Start mtx", true, true), func(m onboard) (tea.Model, tea.Cmd) { return m, tea.Quit })
}

// exportLines is the env block for the saved values, one export per line.
func exportLines(c matrix.Config) []string {
	return []string{
		"export MTX_PICKLE_KEY=" + shQuote(c.PickleKey),
	}
}

// code adds a fenced-style block: a language label, then the lines in a bordered box.
func (p *page) code(lang string, lines []string) {
	p.text("  " + lang)
	block := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(helpBorder).
		Padding(0, 1).MarginLeft(2).Render(strings.Join(lines, "\n"))
	p.text(strings.Split(block, "\n")...)
}

// focusAct focuses field i on click.
func focusAct(i int) func(onboard) (tea.Model, tea.Cmd) {
	return func(m onboard) (tea.Model, tea.Cmd) {
		m.focus = i
		return m, nil
	}
}

// input draws a bordered text box. A focused box shows a block cursor at the end of its text.
// An empty box shows its placeholder dimmed.
func (m onboard) input(i int, value, placeholder string) string {
	focused := m.stage == stageForm && m.focus == i
	var content string
	switch {
	case focused:
		content = value + lipgloss.NewStyle().Reverse(true).Render(" ")
		if value == "" {
			content += lipgloss.NewStyle().Faint(true).Render(placeholder)
		}
	case value != "":
		content = value
	default:
		content = lipgloss.NewStyle().Faint(true).Render(placeholder)
	}
	return box(content)
}

// box is the help box frame: rounded border, color 8, one cell of padding.
func box(content string) string {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(helpBorder).
		Width(56).Padding(0, 1).Render(content)
}

// button is a box with a label. The primary button is bold; the focused one is underlined.
func button(label string, primary, focused bool) string {
	s := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(helpBorder).
		Padding(0, 1).Underline(focused)
	return s.Bold(primary).Render(label)
}

// shQuote single-quotes v for a POSIX shell.
func shQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

// runOnboard shows the first-run screen and returns the config for this run. The caller starts
// the store with it. The store is created only on start, after the user has saved the env.
func runOnboard(store string) (matrix.Config, error) {
	m := onboard{store: store}
	var p *tea.Program
	m.show = func(authURL string) {
		if cmd := exec.Command("xdg-open", authURL); cmd.Start() == nil {
			go func() { _ = cmd.Wait() }()
		}
		p.Send(loginURL(authURL))
	}
	p = tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return matrix.Config{}, err
	}
	fm, ok := final.(onboard)
	if !ok || fm.stage != stageSave {
		return matrix.Config{}, errors.New("start cancelled, no login")
	}
	return fm.cfg, nil
}

// startConfig picks the config for this run: `mtx login` replaces the store, env is used when
// set, and otherwise the first-run screen runs.
func startConfig(args []string, dataHome string) (matrix.Config, error) {
	if len(args) == 1 && args[0] == "login" {
		return loginNew(dataHome)
	}
	if len(args) > 0 {
		return matrix.Config{}, fmt.Errorf("unknown arguments %q (usage: mtx [login|demo])", args)
	}
	cfg, err := matrix.LoadConfig(os.Getenv, dataHome)
	if errors.Is(err, matrix.ErrNoEnv) {
		return firstRun(dataHome)
	}
	return cfg, err
}

// firstRun needs no env and no store. A store without env means its env values were lost.
func firstRun(dataHome string) (matrix.Config, error) {
	path, err := matrix.StorePath(os.Getenv, dataHome)
	if err != nil {
		return matrix.Config{}, err
	}
	if _, err := os.Stat(path); err == nil {
		return matrix.Config{}, fmt.Errorf("no MTX_* env set, but a store exists at %s\n"+
			"set MTX_PICKLE_KEY to the value\n"+
			"saved at login, or run \"mtx login\" to replace the store", path)
	}
	return runOnboard(path)
}

// loginNew replaces the store after a typed yes, then runs the first-run screen.
func loginNew(dataHome string) (matrix.Config, error) {
	path, err := matrix.StorePath(os.Getenv, dataHome)
	if err != nil {
		return matrix.Config{}, err
	}
	fmt.Fprintf(os.Stderr, "mtx login deletes the store at %s\n"+
		"Lost: E2EE keys and sync state. Old messages become unreadable.\n"+
		"The old device stays on the server until you remove it there.\n"+
		"Type yes to continue: ", path)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if strings.TrimSpace(line) != "yes" {
		return matrix.Config{}, errors.New("login cancelled, store kept")
	}
	if err := matrix.RemoveStore(path); err != nil {
		return matrix.Config{}, err
	}
	return runOnboard(path)
}
