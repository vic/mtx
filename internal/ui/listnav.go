package ui

// listnav is the one rule every vertical list moves by, ported from tuios, internal/listnav/listnav.go
// (github.com/Gaurav-Gosain/tuios):
//   - one step off either end wraps to the other end;
//   - any longer move, a page or a jump to an end, stops at the end: a page that wrapped would put the
//     cursor where the eye cannot follow.
//
// The keys are read in one place (listKeys), so the rule is the same wherever a list is met.
// MIT License, Copyright (c) 2025 Gaurav Gosain.

// motion is a movement a key asks a list for.
type motion int

const (
	moveNone motion = iota
	moveUp
	moveDown
	movePageUp
	movePageDown
	moveHome
	moveEnd
)

// listJump is the delta home and end stand for: further than any list is long, so it clamps.
const listJump = 1 << 30

// listKeys reads the list movement keys. Every list here has a filter to type into, so letters (j, k,
// g, G) are the filter's and only these move.
func listKeys(key string) motion {
	switch key {
	case "up", "ctrl+p":
		return moveUp
	case "down", "ctrl+n":
		return moveDown
	case "pgup":
		return movePageUp
	case "pgdown":
		return movePageDown
	case "home":
		return moveHome
	case "end":
		return moveEnd
	}
	return moveNone
}

// listStep is where the cursor at cur lands after motion m in a list of n rows, page rows a page. An
// empty list answers 0.
func listStep(cur int, m motion, n, page int) int {
	if n <= 0 {
		return 0
	}
	cur = min(max(cur, 0), n-1)
	page = max(page, 1)
	delta := map[motion]int{moveUp: -1, moveDown: 1, movePageUp: -page, movePageDown: page, moveHome: -listJump, moveEnd: listJump}[m]
	next := cur + delta
	if delta == 1 || delta == -1 {
		return (next%n + n) % n
	}
	return min(max(next, 0), n-1)
}
