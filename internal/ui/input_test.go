package ui

import "testing"

func TestInputKeys(t *testing.T) {
	cases := []struct {
		name  string
		start string // text, cursor at end unless pos is set
		pos   int    // cursor, -1 for the end
		key   string
		text  string
		want  string
		wantP int
		ok    bool
	}{
		{"insert text", "ab", -1, "", "c", "abc", 3, true},
		{"insert unicode", "héllo ", -1, "", "日本", "héllo 日本", 8, true},
		{"unknown key", "ab", -1, "f5", "", "ab", 2, false},
		{"backspace", "héllo", -1, "backspace", "", "héll", 4, true},
		{"backspace at start", "ab", 0, "backspace", "", "ab", 0, true},
		{"delete", "ab", 0, "delete", "", "b", 0, true},
		{"delete at end", "ab", -1, "delete", "", "ab", 2, true},
		{"left", "ab", -1, "left", "", "ab", 1, true},
		{"left at start", "ab", 0, "left", "", "ab", 0, true},
		{"right", "ab", 0, "right", "", "ab", 1, true},
		{"right at end", "ab", -1, "right", "", "ab", 2, true},
		{"home on a line", "ab\ncd", -1, "home", "", "ab\ncd", 3, true},
		{"end on a line", "ab\ncd", 3, "end", "", "ab\ncd", 5, true},
		{"ctrl+a", "ab\ncd", 4, "ctrl+a", "", "ab\ncd", 3, true},
		{"ctrl+e", "ab\ncd", 3, "ctrl+e", "", "ab\ncd", 5, true},
		{"ctrl+u deletes to line start", "ab\ncdef", 6, "ctrl+u", "", "ab\nf", 3, true},
		{"ctrl+k deletes to line end", "ab\ncdef", 4, "ctrl+k", "", "ab\nc", 4, true},
		{"ctrl+w deletes the word", "one two  ", -1, "ctrl+w", "", "one ", 4, true},
		{"alt+backspace deletes the word", "héllo 日本", -1, "alt+backspace", "", "héllo ", 6, true},
		{"ctrl+w stops at newline", "ab\ncd", 4, "ctrl+w", "", "ab\nd", 3, true},
		{"ctrl+left moves a word", "one two", -1, "ctrl+left", "", "one two", 4, true},
		{"alt+b moves a word", "one two", 4, "alt+b", "", "one two", 0, true},
		{"ctrl+right moves a word", "one two", 0, "ctrl+right", "", "one two", 4, true},
		{"alt+f moves a word", "one two", 4, "alt+f", "", "one two", 7, true},
		{"up clamps column", "abcd\nx", 3, "up", "", "abcd\nx", 3, false},
		{"down clamps column", "abcd\nx", 3, "down", "", "abcd\nx", 6, true},
		{"up keeps column", "abcd\nxyzw", 7, "up", "", "abcd\nxyzw", 2, true},
		{"up on first line", "ab\ncd", 1, "up", "", "ab\ncd", 1, false},
		{"down on last line", "ab\ncd", 4, "down", "", "ab\ncd", 4, false},
		{"down from the first line", "ab\ncd", 1, "down", "", "ab\ncd", 4, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var in input
			in.SetValue(tc.start)
			if tc.pos >= 0 {
				in.pos = tc.pos
			}
			if got := in.Key(tc.key, tc.text); got != tc.ok {
				t.Fatalf("Key returned %v want %v", got, tc.ok)
			}
			if in.Value() != tc.want || in.Pos() != tc.wantP {
				t.Fatalf("got %q at %d want %q at %d", in.Value(), in.Pos(), tc.want, tc.wantP)
			}
		})
	}
}

func TestInputPosition(t *testing.T) {
	var in input
	in.SetValue("ab\ncd\nef")
	if in.LineCount() != 3 || in.Line() != 2 || in.Column() != 2 || in.Pos() != 8 {
		t.Fatalf("end: lines %d line %d col %d pos %d", in.LineCount(), in.Line(), in.Column(), in.Pos())
	}
	in.MoveToBegin()
	if in.Line() != 0 || in.Column() != 0 || in.Pos() != 0 {
		t.Fatalf("begin: line %d col %d pos %d", in.Line(), in.Column(), in.Pos())
	}
	in.MoveToEnd()
	if in.Pos() != 8 {
		t.Fatalf("MoveToEnd pos %d", in.Pos())
	}
	in.Reset()
	if in.Value() != "" || in.Pos() != 0 || in.LineCount() != 1 {
		t.Fatalf("reset: %q %d %d", in.Value(), in.Pos(), in.LineCount())
	}
}

func TestInputInsertAtCursor(t *testing.T) {
	var in input
	in.SetValue("héllo")
	in.pos = 1
	in.InsertString("日本")
	if in.Value() != "h日本éllo" || in.Pos() != 3 {
		t.Fatalf("got %q at %d", in.Value(), in.Pos())
	}
	if in.Column() != 3 || in.Line() != 0 {
		t.Fatalf("col %d line %d", in.Column(), in.Line())
	}
}

func TestInputColumnUnicode(t *testing.T) {
	var in input
	in.SetValue("日本\nét")
	if in.Column() != 2 || in.Line() != 1 {
		t.Fatalf("col %d line %d", in.Column(), in.Line())
	}
}
