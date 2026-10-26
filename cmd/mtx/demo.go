package main

import (
	tea "charm.land/bubbletea/v2"

	"mtx/internal/model"
	"mtx/internal/ui"
)

// runDemo is the UI with made-up rooms and people, for docs and recordings: it sends nothing anywhere.
func runDemo() error {
	m, err := model.New(ringCap)
	if err != nil {
		return err
	}
	app := ui.New(m, func(ch, text string) error { return nil }).WithUser("vic")
	p := tea.NewProgram(app)
	go func() {
		for _, msg := range demoMsgs() {
			p.Send(msg)
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
