package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"mtx/internal/model"
)

// mtx-dsq: every picker shows the digits 1 2 3 4 5 6 7 8 9 0 in a left gutter on its first ten rows;
// pressing one runs that row, as enter does, and never enters the query.

// jumpNames are twelve digit-free, distinct row texts.
var jumpNames = []string{"alpha", "bravo", "cedar", "delta", "ember", "fjord", "gamma", "haven", "iris", "jolt", "karma", "lumen"}

// jumpKinds names every pickKind in order: a new kind must be added here and in jumpSeed, or the table test fails.
var jumpKinds = []string{"chans", "lines", "speakers", "invite", "files", "fileblock", "grep", "recent", "threads", "users", "pins", "edits", "explore", "found", "info", "themes", "help"}

// jumpSeed opens the picker of kind k over at least 12 rows. ok is false for a picker that draws no rows (grep).
func jumpSeed(t *testing.T, k pickKind) (a App, ok bool) {
	t.Helper()
	var log []sent
	a = newApp(t, &log, nil)
	a.w, a.h = 120, 40
	upd := func(msg tea.Msg) {
		next, _ := a.Update(msg)
		a = next.(App)
	}
	lines := func() {
		for i, n := range jumpNames {
			upd(LineMsg{Ch: "#a", L: model.Line{Nick: n, Text: "say " + n, ID: "$" + n, At: time.Date(2026, 10, 27, 9, i, 0, 0, time.UTC)}})
		}
	}
	switch k {
	case pickChans, pickRecent:
		var rooms RoomsMsg
		for _, n := range jumpNames {
			rooms = append(rooms, "&"+n)
		}
		upd(rooms)
	case pickLines:
		lines()
	case pickSpeakers:
		lines()
	case pickInvite:
		var ss SessionsMsg
		for i, n := range jumpNames {
			ss = append(ss, model.Session{PID: 10 + i, Name: n})
		}
		upd(ss)
	case pickFiles, pickFileBlock:
		a.files = slices.Clone(jumpNames)
	case pickGrep:
		return a.openPick(k), false
	case pickThreads:
		var heads []model.Thread
		for i, n := range jumpNames {
			heads = append(heads, model.Thread{Root: "$" + n, Nick: n, Text: "thread " + n, Count: 1, At: time.Date(2026, 10, 27, 9, i, 0, 0, time.UTC)})
		}
		a.threads = map[string][]model.Thread{"#a": heads}
	case pickUsers:
		upd(RoomsMsg{"&people"})
		a = a.focusName("&people")
		upd(MembersMsg{Ch: "&people", Nicks: slices.Clone(jumpNames)})
	case pickPins:
		lines()
		upd(PinsMsg{Ch: "#a", IDs: []string{"$alpha", "$bravo", "$cedar", "$delta", "$ember", "$fjord", "$gamma", "$haven", "$iris", "$jolt", "$karma", "$lumen"}})
	case pickEdits:
		lines()
		a.hit = 0
		a.edits = map[string][]string{"$alpha": slices.Clone(jumpNames)}
	case pickExplore:
		upd(ExploreMsg{Entries: slices.Clone(jumpNames), Targets: slices.Clone(jumpNames)})
		return a, true
	case pickFound:
		lines()
		upd(SearchMsg{Entries: slices.Clone(jumpNames), IDs: []string{"$alpha", "$bravo", "$cedar", "$delta", "$ember", "$fjord", "$gamma", "$haven", "$iris", "$jolt", "$karma", "$lumen"}})
		return a, true
	case pickInfo:
		upd(ListMsg{Title: "list", Entries: slices.Clone(jumpNames)})
		return a, true
	case pickThemes:
		return a.themeCmd(nil), true
	case pickHelp:
		// the help rows come from helpSections: more than 12 of them
	default:
		t.Fatalf("pickKind %d has no jumpSeed case: add one, and its name to jumpKinds", k)
	}
	a = a.openPick(k)
	if n := len(a.pickSource()); n < 12 {
		t.Fatalf("%s: %d rows seeded, want at least 12", jumpKinds[k], n)
	}
	return a, true
}

// jumpDigit is the key of the jump gutter on 0-based row i: 1..9, then 0 on the tenth.
func jumpDigit(i int) string { return fmt.Sprint((i + 1) % 10) }

// jumpBox is the picker box lines, borders and colors stripped: a.pickWhat's matches as pickBox draws them.
func jumpBox(a App) []string {
	box := pickBox(a.w, a.h, "t", a.pq.Value(), a.matches(), a.psel, a.pickWhat == pickSpeakers, a.shownMarks())
	var out []string
	for _, l := range strings.Split(ansi.Strip(box), "\n") {
		l = strings.TrimSpace(l)
		l = strings.TrimSuffix(strings.TrimPrefix(l, "│"), "│")
		out = append(out, l)
	}
	return out
}

// jumpSnap is what a pick leaves behind; two ways of picking the same row must leave the same snap.
func jumpSnap(a App, cmd tea.Cmd) string {
	return fmt.Sprintf("pick=%v cur=%s thread=%q mode=%v in=%q speakers=%q hit=%d theme=%q marks=%q cmd=%v",
		a.pick, cur(a), a.thread, a.mode, a.in.Value(), a.speakers, a.hit, a.themeNow, a.marks, cmd != nil)
}

// TestJumpCoversEveryKind: the table walks 0..pickLast, so a new kind cannot skip the jump keys.
func TestJumpCoversEveryKind(t *testing.T) {
	if len(jumpKinds) != int(pickLast)+1 {
		t.Fatalf("jumpKinds has %d names, pickKind has %d kinds: add the new kind to jumpKinds and jumpSeed", len(jumpKinds), int(pickLast)+1)
	}
}

// TestJumpGutter: the first ten rows show 1..9,0 in a left gutter, rows 11 and 12 a blank gutter of the same width, all columns aligned.
func TestJumpGutter(t *testing.T) {
	for k := pickKind(0); k <= pickLast; k++ {
		t.Run(jumpKinds[k], func(t *testing.T) {
			a, ok := jumpSeed(t, k)
			if !ok {
				t.Skip("no rows drawn")
			}
			ms := a.matches()
			box := jumpBox(a)
			col := -1
			for i := 0; i < 12; i++ {
				name := []rune(ms[i].name)
				name = name[:min(len(name), 4)]
				line := box[3+i] // title and a blank line come first; the window starts at the top
				if !strings.Contains(line, string(name)) {
					t.Fatalf("row %d %q not drawn on its line %q", i, ms[i].name, line)
				}
				at := strings.Index(line, string(name))
				pre := line[:at]
				if c := ansi.StringWidth(pre); col < 0 {
					col = c
				} else if c != col {
					t.Fatalf("row %d text starts at column %d, row 0 at %d: %q", i, c, col, line)
				}
				gutter := strings.Trim(pre, " ◆✓")
				want := ""
				if i < 10 {
					want = jumpDigit(i)
				}
				if gutter != want {
					t.Fatalf("row %d gutter %q want %q: %q", i, gutter, want, line)
				}
			}
		})
	}
}

// TestJumpKeyPicksRow: a digit does what enter on that row does; 1..9 are rows 1..9, 0 is row 10.
func TestJumpKeyPicksRow(t *testing.T) {
	for k := pickKind(0); k <= pickLast; k++ {
		t.Run(jumpKinds[k], func(t *testing.T) {
			a, ok := jumpSeed(t, k)
			if !ok {
				t.Skip("no rows drawn")
			}
			for _, row := range []int{2, 9} {
				downs := slices.Repeat([]string{"down"}, row)
				ref, _ := press(t, a, downs...)
				want, wcmd := press(t, ref, "enter")
				got, gcmd := press(t, a, jumpDigit(row))
				if g, w := jumpSnap(got, gcmd), jumpSnap(want, wcmd); g != w {
					t.Fatalf("key %s:\n got  %s\n want %s", jumpDigit(row), g, w)
				}
			}
		})
	}
}

// TestJumpDigitNotInQuery: the digit never enters the query.
func TestJumpDigitNotInQuery(t *testing.T) {
	for k := pickKind(0); k <= pickLast; k++ {
		t.Run(jumpKinds[k], func(t *testing.T) {
			a, ok := jumpSeed(t, k)
			if !ok {
				t.Skip("no rows drawn")
			}
			for _, d := range []string{"3", "0", "9"} {
				got, _ := press(t, a, d)
				if q := got.pq.Value(); q != "" {
					t.Fatalf("after %q the query is %q", d, q)
				}
			}
		})
	}
}

// jumpFilter types a digit-free query that leaves exactly two rows, found among the substrings of the rows.
func jumpFilter(t *testing.T, a App) App {
	t.Helper()
	for _, row := range a.pickSource() {
		rs := []rune(strings.ToLower(row))
		for n := 2; n <= 4; n++ {
			for i := 0; i+n <= len(rs); i++ {
				q := string(rs[i : i+n])
				if strings.ContainsFunc(q, func(r rune) bool { return r < 'a' || r > 'z' }) {
					continue
				}
				b := a
				b.pq.SetValue(q)
				if len(b.matches()) == 2 {
					return b
				}
			}
		}
	}
	t.Fatalf("no digit-free query leaves exactly two rows of %q", a.pickSource())
	return a
}

// TestJumpFollowsFilteredList: with two rows left, 2 picks the second, 3 does nothing.
func TestJumpFollowsFilteredList(t *testing.T) {
	for k := pickKind(0); k <= pickLast; k++ {
		t.Run(jumpKinds[k], func(t *testing.T) {
			a, ok := jumpSeed(t, k)
			if !ok {
				t.Skip("no rows drawn")
			}
			a = jumpFilter(t, a)
			ref, _ := press(t, a, "down")
			want, wcmd := press(t, ref, "enter")
			got, gcmd := press(t, a, "2")
			if g, w := jumpSnap(got, gcmd), jumpSnap(want, wcmd); g != w {
				t.Fatalf("key 2 on %q:\n got  %s\n want %s", a.pq.Value(), g, w)
			}
			same, scmd := press(t, a, "3")
			if g, w := jumpSnap(same, scmd), jumpSnap(a, nil); g != w || same.pq.Value() != a.pq.Value() || same.psel != a.psel {
				t.Fatalf("key 3 on two rows changed things:\n got  %s q=%q psel=%d\n want %s q=%q psel=%d", g, same.pq.Value(), same.psel, w, a.pq.Value(), a.psel)
			}
		})
	}
}

// TestJumpPastLastRowDoesNothing: a digit above the row count, or on an empty list, does nothing and does not crash.
func TestJumpPastLastRowDoesNothing(t *testing.T) {
	for k := pickKind(0); k <= pickLast; k++ {
		t.Run(jumpKinds[k], func(t *testing.T) {
			a, ok := jumpSeed(t, k)
			if !ok {
				t.Skip("no rows drawn")
			}
			a.pq.SetValue("zzzzzzzz")
			if n := len(a.matches()); n != 0 {
				t.Fatalf("%d rows match the empty-list query", n)
			}
			for _, d := range []string{"1", "5", "0"} {
				got, cmd := press(t, a, d)
				if g, w := jumpSnap(got, cmd), jumpSnap(a, nil); g != w || got.pq.Value() != a.pq.Value() {
					t.Fatalf("key %s on an empty list:\n got  %s q=%q\n want %s", d, g, got.pq.Value(), w)
				}
			}
		})
	}
}

// TestJumpLetterStillTyped: a letter still goes into the query (negative control).
func TestJumpLetterStillTyped(t *testing.T) {
	for k := pickKind(0); k <= pickLast; k++ {
		t.Run(jumpKinds[k], func(t *testing.T) {
			a, ok := jumpSeed(t, k)
			if !ok {
				t.Skip("no rows drawn")
			}
			got, _ := press(t, a, "x")
			if q := got.pq.Value(); q != "x" {
				t.Fatalf("after x the query is %q", q)
			}
		})
	}
}

// TestJumpRowWithDigitStillFound: a row whose text holds a digit (#room7) is found by its letters, no digit typed.
func TestJumpRowWithDigitStillFound(t *testing.T) {
	for _, k := range []pickKind{pickChans, pickRecent} {
		t.Run(jumpKinds[k], func(t *testing.T) {
			var log []sent
			a := newApp(t, &log, nil)
			next, _ := a.Update(RoomsMsg{"#room7"})
			a = next.(App).openPick(k)
			a = typeText(t, a, "room")
			if q := a.pq.Value(); q != "room" {
				t.Fatalf("query %q want room", q)
			}
			found := false
			for _, m := range a.matches() {
				found = found || m.name == "#room7"
			}
			if !found {
				t.Fatalf("#room7 not found by %q: %q", "room", a.pickSource())
			}
		})
	}
}

// TestJumpGrepKeepsDigits: the grep box has no rows, so digits are filter text there: "404" stays in the query and nothing jumps.
func TestJumpGrepKeepsDigits(t *testing.T) {
	a, ok := jumpSeed(t, pickGrep)
	if ok {
		t.Fatal("grep draws no rows: jumpSeed must report ok=false")
	}
	before := cur(a)
	a = typeText(t, a, "404")
	if q := a.pq.Value(); q != "404" {
		t.Fatalf("grep query %q want 404", q)
	}
	if !a.pick || a.pickWhat != pickGrep || cur(a) != before {
		t.Fatalf("pick %v what %v cur %q: a digit in grep must not jump", a.pick, a.pickWhat, cur(a))
	}
}

// TestJumpThemeAppliesSavesAndToasts: a digit in the theme picker keeps the row's theme, saves it and closes, as enter does.
func TestJumpThemeAppliesSavesAndToasts(t *testing.T) {
	a, _ := jumpSeed(t, pickThemes)
	var saved []string
	a.themeSave = func(id string) { saved = append(saved, id) }
	ref, _ := press(t, a, "down", "down")
	ref, _ = press(t, ref, "enter")
	saved = nil // the enter run saved too: count the digit run alone
	got, _ := press(t, a, "3")
	if got.pick || got.themeNow != a.themeIDs[2] || got.themeNow == "" {
		t.Fatalf("pick %v theme %q want closed with %q", got.pick, got.themeNow, a.themeIDs[2])
	}
	if !slices.Equal(saved, []string{a.themeIDs[2]}) {
		t.Fatalf("saved %q want [%q]", saved, a.themeIDs[2])
	}
	if got.themeNow != ref.themeNow || len(got.toasts) != len(ref.toasts) {
		t.Fatalf("digit: theme %q toasts %d; enter: theme %q toasts %d", got.themeNow, len(got.toasts), ref.themeNow, len(ref.toasts))
	}
}
