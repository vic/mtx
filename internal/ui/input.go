package ui

import "unicode"

// input is the INSERT and prompt line: runes and a cursor, no rendering (the view draws Value and Pos).
type input struct {
	r   []rune
	pos int // cursor, a rune index in r (0..len(r))
}

// Value is the text.
func (in *input) Value() string { return string(in.r) }

// SetValue replaces the text; the cursor goes to the end.
func (in *input) SetValue(s string) {
	in.r = []rune(s)
	in.pos = len(in.r)
}

// Reset empties the text.
func (in *input) Reset() { in.r, in.pos = nil, 0 }

// InsertString inserts s at the cursor; the cursor goes after it.
func (in *input) InsertString(s string) {
	add := []rune(s)
	in.r = append(in.r[:in.pos], append(add, in.r[in.pos:]...)...)
	in.pos += len(add)
}

// Pos is the cursor's rune offset.
func (in *input) Pos() int { return in.pos }

// Line is the row of the cursor: the number of newlines before it.
func (in *input) Line() int {
	n := 0
	for _, r := range in.r[:in.pos] {
		if r == '\n' {
			n++
		}
	}
	return n
}

// Column is the number of runes between the last newline before the cursor and the cursor.
func (in *input) Column() int { return in.pos - in.lineStart(in.pos) }

// LineCount is 1 plus the number of newlines.
func (in *input) LineCount() int {
	n := 1
	for _, r := range in.r {
		if r == '\n' {
			n++
		}
	}
	return n
}

// MoveToEnd puts the cursor after the last rune.
func (in *input) MoveToEnd() { in.pos = len(in.r) }

// MoveToBegin puts the cursor before the first rune.
func (in *input) MoveToBegin() { in.pos = 0 }

// lineStart is the index of the first rune of the line that holds index p.
func (in *input) lineStart(p int) int {
	for i := p; i > 0; i-- {
		if in.r[i-1] == '\n' {
			return i
		}
	}
	return 0
}

// lineEnd is the index of the newline ending the line that holds index p, or the text's end.
func (in *input) lineEnd(p int) int {
	for i := p; i < len(in.r); i++ {
		if in.r[i] == '\n' {
			return i
		}
	}
	return len(in.r)
}

// delete removes the runes in [from, to) and puts the cursor at from.
func (in *input) delete(from, to int) {
	in.r = append(in.r[:from], in.r[to:]...)
	in.pos = from
}

// wordLeft is the index where the word before p starts: spaces (not newlines) then non-spaces.
func (in *input) wordLeft(p int) int {
	for p > 0 && unicode.IsSpace(in.r[p-1]) && in.r[p-1] != '\n' {
		p--
	}
	for p > 0 && !unicode.IsSpace(in.r[p-1]) {
		p--
	}
	return p
}

// wordRight is the index after the word at or after p: non-spaces then spaces.
func (in *input) wordRight(p int) int {
	for p < len(in.r) && !unicode.IsSpace(in.r[p]) {
		p++
	}
	for p < len(in.r) && unicode.IsSpace(in.r[p]) && in.r[p] != '\n' {
		p++
	}
	return p
}

// Key applies one editing key (KeyPressMsg.String() form) and reports whether it was an editing key.
// Printable text (text) inserts. up and down report false on the first and last line, so the caller
// can use them; the others always apply.
func (in *input) Key(k, text string) bool {
	switch k {
	case "backspace":
		if in.pos > 0 {
			in.delete(in.pos-1, in.pos)
		}
	case "delete":
		if in.pos < len(in.r) {
			in.delete(in.pos, in.pos+1)
		}
	case "left":
		in.pos = max(in.pos-1, 0)
	case "right":
		in.pos = min(in.pos+1, len(in.r))
	case "home", "ctrl+a":
		in.pos = in.lineStart(in.pos)
	case "end", "ctrl+e":
		in.pos = in.lineEnd(in.pos)
	case "ctrl+u":
		in.delete(in.lineStart(in.pos), in.pos)
	case "ctrl+k":
		in.delete(in.pos, in.lineEnd(in.pos))
	case "ctrl+w", "alt+backspace":
		in.delete(in.wordLeft(in.pos), in.pos)
	case "ctrl+left", "alt+b":
		in.pos = in.wordLeft(in.pos)
	case "ctrl+right", "alt+f":
		in.pos = in.wordRight(in.pos)
	case "up", "down":
		start := in.lineStart(in.pos)
		col := in.pos - start
		if k == "up" {
			if start == 0 {
				return false
			}
			prevEnd := start - 1
			prevStart := in.lineStart(prevEnd)
			in.pos = prevStart + min(col, prevEnd-prevStart)
			break
		}
		end := in.lineEnd(in.pos)
		if end == len(in.r) {
			return false
		}
		nextStart := end + 1
		in.pos = nextStart + min(col, in.lineEnd(nextStart)-nextStart)
	default:
		if text == "" {
			return false
		}
		in.InsertString(text)
	}
	return true
}
