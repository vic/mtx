package ui

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"mtx/internal/model"
)

// benchApp is a 200x50 app with 10 channels, 1000 lines in the current one.
func benchApp(b *testing.B) App {
	b.Helper()
	m, err := model.New(1000)
	if err != nil {
		b.Fatal(err)
	}
	a := New(m, func(ch, text string) error { return nil })
	a.now = func() time.Time { return time.Date(2026, 10, 22, 12, 4, 0, 0, time.UTC) }
	var ss []model.Session
	for i := 1; i <= 10; i++ {
		ss = append(ss, model.Session{PID: i, Name: fmt.Sprintf("s%d", i)})
	}
	next, _ := a.Update(SessionsMsg(ss))
	a = next.(App)
	next, _ = a.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	a = next.(App)
	ch := a.m.Current().Name
	ls := make([]model.Line, 1000)
	for i := range ls {
		ls[i] = model.Line{At: a.now(), Nick: fmt.Sprintf("n%d", i%7), Text: fmt.Sprintf("line %d with some **bold** text and `code` @n3 mention that is long enough to wrap around the pane width at least once or twice", i)}
	}
	next, _ = a.Update(HistoryLinesMsg{Ch: ch, L: ls})
	return next.(App)
}

func BenchmarkView(b *testing.B) {
	a := benchApp(b)
	b.ReportAllocs()
	for b.Loop() {
		_ = a.View()
	}
}

func BenchmarkAddLineCurrent(b *testing.B) {
	a := benchApp(b)
	ch := a.m.Current().Name
	b.ReportAllocs()
	for b.Loop() {
		next, _ := a.Update(LineMsg{Ch: ch, L: model.Line{At: a.now(), Nick: "x", Text: "hello"}})
		a = next.(App)
	}
}

func BenchmarkWheel100(b *testing.B) {
	a := benchApp(b)
	var bm BatchMsg
	for range 100 {
		bm = append(bm, tea.MouseWheelMsg{Button: tea.MouseWheelUp, X: 100, Y: 10})
	}
	b.ReportAllocs()
	for b.Loop() {
		next, _ := a.Update(bm)
		a = next.(App)
	}
}

func BenchmarkTypeChar(b *testing.B) {
	a := benchApp(b)
	next, _ := a.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	a = next.(App)
	b.ReportAllocs()
	// one op = type a char and delete it, with a frame after each: the input stays one line
	for b.Loop() {
		next, _ := a.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
		a = next.(App)
		_ = a.View()
		next, _ = a.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
		a = next.(App)
		_ = a.View()
	}
}
