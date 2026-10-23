package ui

import (
	"testing"

	"github.com/stretchr/testify/require"

	"mtx/internal/model"
)

// TestReplyHookGetsQuotedLine: with a reply hook, a quoted send goes through it with the line and
// the text stays as typed; without one, the quote is prefixed as markdown.
func TestReplyHookGetsQuotedLine(t *testing.T) {
	var log []sent
	a := newApp(t, &log, nil)
	var gotTo model.Line
	var gotText string
	a = a.WithReply(func(ch string, to model.Line, text string) error {
		gotTo, gotText = to, text
		return nil
	})
	a.quote = &model.Line{Nick: "bob", Text: "hi", ID: "$m1"}
	a.in.SetValue("yes")
	next, cmd := a.sendLine()
	a = next.(App)
	res := cmd().(SendResultMsg)
	require.NoError(t, res.Err)
	if gotTo.ID != "$m1" || gotText != "yes" {
		t.Fatalf("reply hook got %+v %q", gotTo, gotText)
	}
	if a.quote != nil {
		t.Fatal("quote not cleared")
	}
}
