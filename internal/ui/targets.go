package ui

import (
	"os/exec"
	"regexp"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
)

var (
	pathRE    = regexp.MustCompile(`^[\w@%+=.~/-]+$`)
	extRE     = regexp.MustCompile(`\.[A-Za-z]\w*$`)
	lineSufRE = regexp.MustCompile(`:\d+(:\d+)?$`) // file.nix:12:3, opened without the position
)

// clickTarget is what a click at display column col of a lines-pane row points at:
// a file path (path) or a `code span` that is not a path (cmd). ok is false when
// the click is on plain text.
func clickTarget(row string, col int) (path, cmd string, ok bool) {
	if col < 0 || col >= lipgloss.Width(row) {
		return "", "", false
	}
	b := byteAt(row, col)
	if code, span := codeSpan(row, b); span {
		if isPath(code) {
			return code, "", true
		}
		return "", code, code != ""
	}
	start := strings.LastIndexAny(row[:b], " \t") + 1
	end := b + len(row[b:])
	if i := strings.IndexAny(row[b:], " \t"); i >= 0 {
		end = b + i
	}
	word := strings.Trim(row[start:end], "\"'()[]{}<>,;:!?")
	word = lineSufRE.ReplaceAllString(strings.TrimSuffix(word, "."), "")
	if isPath(word) {
		return word, "", true
	}
	return "", "", false
}

// codeSpan returns the text of the `code span` covering byte b of row.
func codeSpan(row string, b int) (string, bool) {
	for i := 0; ; {
		o := strings.IndexByte(row[i:], '`')
		if o < 0 {
			return "", false
		}
		o += i
		c := strings.IndexByte(row[o+1:], '`')
		if c < 0 {
			return "", false
		}
		c += o + 1
		if b >= o && b <= c {
			return row[o+1 : c], true
		}
		i = c + 1
	}
}

// isPath reports whether s is one word that looks like a file: a directory part or an extension.
func isPath(s string) bool {
	return s != "." && s != ".." && pathRE.MatchString(s) && (strings.Contains(s, "/") || extRE.MatchString(s))
}

// byteAt is the byte offset of the character under display column col of s.
func byteAt(s string, col int) int {
	at := 0
	for b := range s {
		if lipgloss.Width(s[:b]) > col {
			break
		}
		at = b
	}
	return at
}

// listFiles is the tracked and untracked, not ignored, files under dir as git prints
// them (relative to dir). Outside a git repo there are none.
func listFiles(dir string) []string {
	cmd := exec.Command("git", "ls-files", "-co", "--exclude-standard", "-z")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return slices.DeleteFunc(strings.Split(string(out), "\x00"), func(s string) bool { return s == "" })
}

// urlAt is the http(s) URL of the word under column col of row, "" when there is none.
// mdLinkRE is a markdown link [text](http...): a click anywhere on it opens the URL.
var mdLinkRE = regexp.MustCompile(`\[[^\]]*\]\((https?://[^)\s]+)\)`)

func urlAt(row string, col int) string {
	if col < 0 || col >= lipgloss.Width(row) {
		return ""
	}
	b := byteAt(row, col)
	for _, m := range mdLinkRE.FindAllStringSubmatchIndex(row, -1) {
		if b >= m[0] && b < m[1] {
			return row[m[2]:m[3]]
		}
	}
	start := strings.LastIndexAny(row[:b], " \t") + 1
	if row[b] == ' ' || row[b] == '\t' { // a click on the space between words
		return ""
	}
	end := len(row)
	if i := strings.IndexAny(row[b:], " \t"); i >= 0 {
		end = b + i
	}
	word := strings.Trim(row[start:end], "\"'()[]{}<>,;")
	if strings.HasPrefix(word, "https://") || strings.HasPrefix(word, "http://") {
		return word
	}
	return ""
}

// reactAt is the reaction key under display column col of a lines-pane row whose text ends with a
// summary "  [👍2 ❤️1]", "" when the click is elsewhere.
func reactAt(row string, col int) string {
	row = strings.TrimRight(row, " ")
	i := strings.LastIndex(row, "  [")
	if i < 0 {
		return ""
	}
	return reactIn(row, i+len("  ["), col)
}

// reactAtCont is the reaction key under column col of a row that continues a wrapped summary: the
// tokens start after the row's indentation and the row may end with "]".
func reactAtCont(row string, col int) string {
	body := strings.TrimRight(row, " ")
	return reactIn(body, len(body)-len(strings.TrimLeft(body, " ")), col)
}

// reactIn is the reaction key under column col among the tokens of text from index pos, a closing "]"
// left out.
func reactIn(text string, pos, col int) string {
	text = strings.TrimSuffix(text, "]")
	for _, part := range strings.Fields(text[pos:]) {
		at := strings.Index(text[pos:], part) + pos
		start := lipgloss.Width(text[:at])
		if col >= start && col < start+lipgloss.Width(part) {
			return strings.TrimRight(part, "0123456789")
		}
		pos = at + len(part)
	}
	return ""
}
