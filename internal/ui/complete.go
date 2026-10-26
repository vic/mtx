package ui

import (
	"slices"
	"strings"

	"mtx/internal/model"
)

// completion is tab completion of the last word of an INSERT line.
type completion struct {
	base  string // the line as typed before the first tab
	cands []string
	next  int // index of the candidate the next tab puts in
}

// lastWord splits line into everything before its last word and that word.
func lastWord(line string) (head, word string) {
	i := strings.LastIndexAny(line, " \t") + 1
	return line[:i], line[i:]
}

// emojis are the shortcodes INSERT completes after ":" (":smi" -> 😄), most used first.
var emojis = []struct{ name, e string }{
	{"thumbsup", "👍"}, {"heart", "❤️"}, {"joy", "😂"}, {"smile", "😄"}, {"tada", "🎉"}, {"fire", "🔥"}, {"eyes", "👀"},
	{"pray", "🙏"}, {"thinking", "🤔"}, {"rocket", "🚀"}, {"check", "✅"}, {"x", "❌"}, {"wave", "👋"}, {"clap", "👏"},
	{"ok", "👌"}, {"cry", "😢"}, {"wink", "😉"}, {"grin", "😁"}, {"sweat_smile", "😅"}, {"rofl", "🤣"}, {"sunglasses", "😎"},
	{"party", "🥳"}, {"hug", "🤗"}, {"shrug", "🤷"}, {"facepalm", "🤦"}, {"100", "💯"}, {"star", "⭐"}, {"sparkles", "✨"},
	{"bug", "🐛"}, {"coffee", "☕"}, {"beer", "🍺"}, {"thumbsdown", "👎"}, {"muscle", "💪"}, {"bulb", "💡"}, {"warning", "⚠️"},
	{"lock", "🔒"}, {"key", "🔑"}, {"zap", "⚡"}, {"skull", "💀"}, {"poop", "💩"}, {"robot", "🤖"}, {"snake", "🐍"}, {"penguin", "🐧"},
}

// candidates for the last word of line: command words for the first word of a /
// line, channels for & and #, nicks (with the @ kept) for the rest. Prefix match,
// in the order of the sources: channels most recent first, nicks as listed.
// emoji (INSERT only) completes a :shortcode to its emoji.
func candidates(line string, chans, nicks []string, emoji bool) []string {
	head, word := lastWord(line)
	if name, ok := strings.CutPrefix(word, ":"); ok && emoji && name != "" { // :shortcode -> the emoji itself
		var out []string
		for _, e := range emojis {
			if strings.HasPrefix(e.name, name) {
				out = append(out, e.e)
			}
		}
		return out
	}
	var pool []string
	switch {
	case head == "" && strings.HasPrefix(word, "/"):
		for _, w := range model.CmdWords {
			pool = append(pool, "/"+w)
		}
	case head == "" && strings.HasPrefix(word, ":"):
		for _, w := range model.CmdWords {
			pool = append(pool, ":"+w)
		}
	case strings.HasPrefix(word, "&") || strings.HasPrefix(word, "#"):
		pool = chans
	case strings.HasPrefix(word, "@"):
		for _, n := range nicks {
			pool = append(pool, "@"+n)
		}
	case word != "":
		pool = nicks
	}
	var out []string
	for _, c := range pool {
		if strings.HasPrefix(strings.ToLower(c), strings.ToLower(word)) && c != word && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

// step returns the line with the next candidate in place of the last word, and the
// completion for the tab after it. The first tab starts from line; later tabs cycle.
func (c completion) step(line string, chans, nicks []string, emoji bool) (string, completion) {
	if c.cands == nil || line != c.line() {
		c = completion{base: line, cands: candidates(line, chans, nicks, emoji)}
	}
	if len(c.cands) == 0 {
		return line, completion{}
	}
	c.next = (c.next + 1) % len(c.cands)
	return c.line(), c
}

// line is base with the current candidate (next-1) as its last word.
func (c completion) line() string {
	if len(c.cands) == 0 {
		return c.base
	}
	head, _ := lastWord(c.base)
	return head + c.cands[(c.next-1+len(c.cands))%len(c.cands)]
}
