package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"mtx/internal/model"
	"mtx/internal/ui"
)

// TestDemoMsgs: the demo's data shows its made-up history in the UI.
func TestDemoMsgs(t *testing.T) {
	m, err := model.New(ringCap)
	if err != nil {
		t.Fatal(err)
	}
	app := ui.New(m, func(ch, text string) error { return nil }).WithUser("vic")
	var next tea.Model = app
	next, _ = next.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	for _, msg := range demoMsgs() {
		next, _ = next.Update(msg)
	}
	if view := ansi.Strip(next.View().Content); !strings.Contains(view, "release still today") {
		t.Fatalf("demo history not on screen:\n%s", view)
	}
}
