package main

import (
	"os"

	tea "charm.land/bubbletea/v2"

	"mtx/internal/model"
	"mtx/internal/ui"
)

// runDemo is the UI with made-up rooms and people, for docs and recordings: it sends nothing anywhere.
// MTX_DEMO_VERIFY=1 also opens the verify box with fixed emojis (the docs gif; no real check runs).
func runDemo() error {
	m, err := model.New(ringCap)
	if err != nil {
		return err
	}
	app := ui.New(m, func(ch, text string) error { return nil }).WithUser("vic")
	p := tea.NewProgram(app)
	go func() {
		p.Send(ui.ThemeMsg("base24/dracula")) // the docs gifs use mtx's own Dracula, not the terminal's
		for _, msg := range demoMsgs() {
			p.Send(msg)
		}
		if os.Getenv("MTX_DEMO_VERIFY") == "1" {
			p.Send(ui.VerifyMsg{Kind: "sas", Txn: "demo", Emojis: "🐶 🍕 🚀 🎲 🌵 ⭐ 🔑"})
		}
	}()
	_, err = p.Run()
	return err
}

// demoMsgs is the demo's data, in the order the UI gets it.
func demoMsgs() []tea.Msg {
	return []tea.Msg{
		ui.RoomsMsg{"#mtx"},
		ui.HistoryLinesMsg{Ch: "#mtx", L: []model.Line{
			{ID: "$1", Nick: "alice", Text: "morning! is the release still today?"},
			{ID: "$2", Nick: "vic", Text: "yes, after the docs land"},
		}},
	}
}
