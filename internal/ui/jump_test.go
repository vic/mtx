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

// newJumpApp is newApp over a scrollback that holds 200 lines, so 30 lines are all kept.
func newJumpApp(t *testing.T) App {
	t.Helper()
	m, err := model.New(200)
	if err != nil {
		t.Fatal(err)
	}
	a := New(m, func(ch, text string) error { return nil })
	a.now = func() time.Time { return time.Date(2026, 10, 22, 12, 4, 0, 0, time.UTC) }
	next, _ := a.Update(SessionsMsg{{PID: 1, Name: "a"}, {PID: 2, Name: "b"}, {PID: 3, Name: "c"}})
	return next.(App)
}

// jumpRows is count digit-free, distinct row texts: jumpNames first, then extra rows.
func jumpRows(count int) []string {
	out := slices.Clone(jumpNames)
	for i := 0; len(out) < count; i++ {
		out = append(out, "extra"+string(rune('a'+i/26))+string(rune('a'+i%26)))
	}
	return out[:count]
}

// jumpIDs are the event IDs of the lines jumpSeedN makes for names.
func jumpIDs(names []string) []string {
	var out []string
	for _, n := range names {
		out = append(out, "$"+n)
	}
	return out
}

// jumpSeed opens the picker of kind k over 12 rows.
func jumpSeed(t *testing.T, k pickKind) (App, bool) { return jumpSeedN(t, k, 12) }

// jumpSeedN opens the picker of kind k over at least count rows. ok is false for a picker that draws no rows (grep).
func jumpSeedN(t *testing.T, k pickKind, count int) (a App, ok bool) {
	names := jumpRows(count)
	t.Helper()
	a = newJumpApp(t)
	a.w, a.h = 120, 40
	upd := func(msg tea.Msg) {
		next, _ := a.Update(msg)
		a = next.(App)
	}
	lines := func() {
		for i, n := range names {
			upd(LineMsg{Ch: "#a", L: model.Line{Nick: n, Text: "say " + n, ID: "$" + n, At: time.Date(2026, 10, 27, 9, i, 0, 0, time.UTC)}})
		}
	}
	switch k {
	case pickChans, pickRecent:
		var rooms RoomsMsg
		for _, n := range names {
			rooms = append(rooms, "&"+n)
		}
		upd(rooms)
	case pickLines:
		lines()
	case pickSpeakers:
		lines()
	case pickInvite:
		var ss SessionsMsg
		for i, n := range names {
			ss = append(ss, model.Session{PID: 10 + i, Name: n})
		}
		upd(ss)
	case pickFiles, pickFileBlock:
		a.files = slices.Clone(names)
	case pickGrep:
		return a.openPick(k), false
	case pickThreads:
		var heads []model.Thread
		for i, n := range names {
			heads = append(heads, model.Thread{Root: "$" + n, Nick: n, Text: "thread " + n, Count: 1, At: time.Date(2026, 10, 27, 9, i, 0, 0, time.UTC)})
		}
		a.threads = map[string][]model.Thread{"#a": heads}
	case pickUsers:
		upd(RoomsMsg{"&people"})
		a = a.focusName("&people")
		upd(MembersMsg{Ch: "&people", Nicks: slices.Clone(names)})
	case pickPins:
		lines()
		upd(PinsMsg{Ch: "#a", IDs: jumpIDs(names)})
	case pickEdits:
		lines()
		a.hit = 0
		a.edits = map[string][]string{"$alpha": slices.Clone(names)}
	case pickExplore:
		upd(ExploreMsg{Entries: slices.Clone(names), Targets: slices.Clone(names)})
		return a, true
	case pickFound:
		lines()
		upd(SearchMsg{Entries: slices.Clone(names), IDs: jumpIDs(names)})
		return a, true
	case pickInfo:
		upd(ListMsg{Title: "list", Entries: slices.Clone(names)})
		return a, true
	case pickThemes:
		return a.themeCmd(nil), true
	case pickHelp:
		// the help rows come from helpSections: more than 12 of them
	default:
		t.Fatalf("pickKind %d has no jumpSeed case: add one, and its name to jumpKinds", k)
	}
	a = a.openPick(k)
	if n := len(a.pickSource()); n < min(count, 3) {
		t.Fatalf("%s: %d rows seeded, want at least %d", jumpKinds[k], n, count)
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

// mtx-hll: the digits follow the scrolling (the first row drawn is always 1) and are drawn like which-key keys.

// jumpScrollApp opens kind k over 30 rows on a short screen and moves the cursor to row 15, so the window starts below row 1.
func jumpScrollApp(t *testing.T, k pickKind) (a App, start, end int) {
	t.Helper()
	a, _ = jumpSeedN(t, k, 30)
	a.h = 20
	a, _ = press(t, a, slices.Repeat([]string{"down"}, 14)...)
	n := len(a.matches())
	start, end = pickWindow(n, a.psel, max(a.h-6, 1))
	if start == 0 || n < 30 {
		t.Fatalf("%s: window %d..%d over %d rows with the cursor on %d: want it scrolled", jumpKinds[k], start, end, n, a.psel)
	}
	return a, start, end
}

// jumpItems is the item lines of the box, the window markers dropped.
func jumpItems(a App) []string {
	var out []string
	for _, l := range jumpBox(a)[3:] {
		if strings.HasPrefix(strings.TrimSpace(l), "↑") {
			continue
		}
		if t := strings.TrimSpace(l); t == "" || strings.HasPrefix(t, "↓") {
			break
		}
		out = append(out, l)
	}
	return out
}

// TestJumpGutterFollowsScrolling (mtx-hll 1): on a scrolled window the first row drawn shows 1, the next 2, the tenth 0.
func TestJumpGutterFollowsScrolling(t *testing.T) {
	for k := pickKind(0); k <= pickLast; k++ {
		t.Run(jumpKinds[k], func(t *testing.T) {
			if k == pickGrep {
				t.Skip("no rows drawn")
			}
			a, start, end := jumpScrollApp(t, k)
			ms := a.matches()
			box := jumpBox(a)
			if !strings.Contains(box[3], "↑") {
				t.Fatalf("scrolled window lacks its ↑ marker: %q", box[3])
			}
			for j := 0; j < end-start; j++ {
				line := box[4+j]
				name := []rune(ms[start+j].name)
				name = name[:min(len(name), 4)]
				at := strings.Index(line, string(name))
				if at < 0 {
					t.Fatalf("visible row %d %q not on its line %q", j, ms[start+j].name, line)
				}
				want := ""
				if j < 10 {
					want = jumpDigit(j)
				}
				if g := strings.Trim(line[:at], " ◆✓"); g != want {
					t.Fatalf("visible row %d (list row %d) gutter %q want %q: %q", j, start+j, g, want, line)
				}
			}
		})
	}
}

// TestJumpKeyPicksVisibleRow (mtx-hll 1): on a scrolled window "1" picks the first row drawn and "0" the tenth drawn.
func TestJumpKeyPicksVisibleRow(t *testing.T) {
	for k := pickKind(0); k <= pickLast; k++ {
		t.Run(jumpKinds[k], func(t *testing.T) {
			if k == pickGrep {
				t.Skip("no rows drawn")
			}
			a, start, end := jumpScrollApp(t, k)
			if end-start < 10 {
				t.Fatalf("window shows %d rows, want at least 10", end-start)
			}
			for _, d := range []string{"1", "0"} {
				row := start
				if d == "0" {
					row = start + 9
				}
				fresh, _ := jumpSeedN(t, k, 30)
				fresh.h = 20
				want, wcmd := press(t, fresh, append(slices.Repeat([]string{"down"}, row), "enter")...)
				got, gcmd := press(t, a, d)
				if g, w := jumpSnap(got, gcmd), jumpSnap(want, wcmd); g != w {
					t.Fatalf("key %s should pick list row %d:\n got  %s\n want %s", d, row, g, w)
				}
			}
		})
	}
}

// TestJumpUnscrolledMovedCursor (mtx-hll 2): with the window at the top, a moved cursor changes nothing: 1 is still the top row.
func TestJumpUnscrolledMovedCursor(t *testing.T) {
	for k := pickKind(0); k <= pickLast; k++ {
		t.Run(jumpKinds[k], func(t *testing.T) {
			if k == pickGrep {
				t.Skip("no rows drawn")
			}
			a, _ := jumpSeed(t, k)
			a, _ = press(t, a, "down", "down", "down")
			if start, _ := pickWindow(len(a.matches()), a.psel, max(a.h-6, 1)); start != 0 {
				t.Fatalf("window starts at %d, want the top", start)
			}
			if g := strings.Trim(jumpItems(a)[0][:2], " ◆✓"); g != "1" {
				t.Fatalf("top row gutter %q want 1", g)
			}
			fresh, _ := jumpSeed(t, k)
			want, wcmd := press(t, fresh, "enter")
			got, gcmd := press(t, a, "1")
			if g, w := jumpSnap(got, gcmd), jumpSnap(want, wcmd); g != w {
				t.Fatalf("key 1 with the cursor on row 4:\n got  %s\n want %s", g, w)
			}
		})
	}
}

// jumpFilterFew types a digit-free query that leaves between 3 and 9 rows, and returns how many.
func jumpFilterFew(t *testing.T, a App) (App, int) {
	t.Helper()
	for _, row := range a.pickSource() {
		rs := []rune(strings.ToLower(row))
		for l := 2; l <= 10; l++ {
			for i := 0; i+l <= len(rs); i++ {
				q := string(rs[i : i+l])
				if strings.ContainsFunc(q, func(r rune) bool { return r < 'a' || r > 'z' }) {
					continue
				}
				b := a
				b.pq.SetValue(q)
				if n := len(b.matches()); n >= 3 && n <= 9 {
					return b, n
				}
			}
		}
	}
	t.Fatalf("no digit-free query leaves 3 to 9 rows of %q", a.pickSource())
	return a, 0
}

// TestJumpFewRowsStopAtLastRow (mtx-hll 3): with fewer than ten rows drawn the gutter stops at the last row; the next digit does nothing.
func TestJumpFewRowsStopAtLastRow(t *testing.T) {
	for k := pickKind(0); k <= pickLast; k++ {
		t.Run(jumpKinds[k], func(t *testing.T) {
			if k == pickGrep {
				t.Skip("no rows drawn")
			}
			a, _ := jumpSeedN(t, k, 4)
			n := len(a.matches())
			if n < 3 || n > 9 { // a picker with fixed rows (help, themes): filter it down
				a, n = jumpFilterFew(t, a)
			}
			items := jumpItems(a)
			if len(items) != n {
				t.Fatalf("%d rows drawn, want %d: %q", len(items), n, items)
			}
			for j, l := range items {
				if g := strings.Trim(l[:2], " ◆✓"); g != jumpDigit(j) {
					t.Fatalf("row %d gutter %q want %q: %q", j, g, jumpDigit(j), l)
				}
			}
			same, scmd := press(t, a, jumpDigit(n))
			if g, w := jumpSnap(same, scmd), jumpSnap(a, nil); g != w || same.pq.Value() != a.pq.Value() {
				t.Fatalf("key %s past the last of %d rows:\n got  %s q=%q\n want %s q=%q", jumpDigit(n), n, g, same.pq.Value(), w, a.pq.Value())
			}
			ref, _ := press(t, a, slices.Repeat([]string{"down"}, n-1)...)
			want, wcmd := press(t, ref, "enter")
			got, gcmd := press(t, a, jumpDigit(n-1))
			if g, w := jumpSnap(got, gcmd), jumpSnap(want, wcmd); g != w {
				t.Fatalf("key %s (the last row):\n got  %s\n want %s", jumpDigit(n-1), g, w)
			}
		})
	}
}

// TestJumpDigitStyleIsWhichKeyKey (mtx-hll 4): the digit cell is drawn like a which-key key: shortcut.Bold(true), as menuBox does.
// Checked on the raw box string: the exact styled digit must be in the row line and the plain-gutter style must not be.
func TestJumpDigitStyleIsWhichKeyKey(t *testing.T) {
	want := func(d string) string { return shortcut.Bold(true).Render(d) }
	if want("1") == "1" || want("1") == dimmed.Render("1") {
		t.Fatalf("test palette draws a which-key key as %q: the style check would prove nothing", want("1"))
	}
	for k := pickKind(0); k <= pickLast; k++ {
		t.Run(jumpKinds[k], func(t *testing.T) {
			if k == pickGrep {
				t.Skip("no rows drawn")
			}
			a, _ := jumpSeed(t, k)
			raw := strings.Split(pickBox(a.w, a.h, "t", a.pq.Value(), a.matches(), a.psel, a.pickWhat == pickSpeakers, a.shownMarks()), "\n")
			for j := 0; j < 10; j++ {
				line := raw[3+j]
				if !strings.Contains(line, want(jumpDigit(j))) {
					t.Fatalf("row %d: %q lacks the which-key styled digit %q", j, line, want(jumpDigit(j)))
				}
				if strings.Contains(line, dimmed.Render(jumpDigit(j))+" ") {
					t.Fatalf("row %d: %q draws the digit dimmed", j, line)
				}
			}
		})
	}
}

// TestJumpThemeDigitWhenScrolled (mtx-hll 5): in the scrolled theme picker a digit applies, saves and toasts the visible row, as enter does.
func TestJumpThemeDigitWhenScrolled(t *testing.T) {
	a, start, _ := jumpScrollApp(t, pickThemes)
	var saved []string
	a.themeSave = func(id string) { saved = append(saved, id) }
	nToasts := len(a.toasts)
	got, _ := press(t, a, "1")
	if got.pick || got.themeNow != a.themeIDs[start] || got.themeNow == "" {
		t.Fatalf("pick %v theme %q want closed with the first visible row %q", got.pick, got.themeNow, a.themeIDs[start])
	}
	if !slices.Equal(saved, []string{a.themeIDs[start]}) {
		t.Fatalf("saved %q want [%q]", saved, a.themeIDs[start])
	}
	if len(got.toasts) != nToasts+1 || !strings.Contains(got.toasts[len(got.toasts)-1].text, "theme") {
		t.Fatalf("toasts %+v want one more mentioning the theme", got.toasts)
	}
}

// TestJumpDigitsStayWhileRowsScroll (mtx-hll c): one more down key moves the row text up, but the gutter reads 1 2 ... on the same screen lines.
func TestJumpDigitsStayWhileRowsScroll(t *testing.T) {
	for k := pickKind(0); k <= pickLast; k++ {
		t.Run(jumpKinds[k], func(t *testing.T) {
			if k == pickGrep {
				t.Skip("no rows drawn")
			}
			a, _, _ := jumpScrollApp(t, k)
			b, _ := press(t, a, "down", "down")
			ia, ib := jumpItems(a), jumpItems(b)
			if ia[0][2:] == ib[0][2:] {
				t.Fatalf("two more down keys left the first drawn row %q unchanged", ia[0])
			}
			for j := 0; j < 10; j++ {
				ga, gb := strings.Trim(ia[j][:2], " ◆✓"), strings.Trim(ib[j][:2], " ◆✓")
				if ga != jumpDigit(j) || gb != jumpDigit(j) {
					t.Fatalf("screen line %d: gutter %q then %q want %q both times", j, ga, gb, jumpDigit(j))
				}
			}
		})
	}
}
