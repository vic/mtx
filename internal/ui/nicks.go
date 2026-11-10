package ui

import (
	"strings"

	"mtx/internal/model"
)

// railRow is one row of a side pane: an item (a channel, a nick, a thread) or a group label.
type railRow struct {
	text  string
	label bool // a group label: drawn dim, never focused, mentioned or jumped to
}

// item is the row's text when the row is an item, not a group label.
func (r railRow) item() (string, bool) { return r.text, !r.label }

// items are plain item rows, one per name.
func items(names []string) []railRow {
	rows := make([]railRow, len(names))
	for i, n := range names {
		rows[i] = railRow{text: n}
	}
	return rows
}

// texts are the rows' texts, labels included.
func texts(rows []railRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.text
	}
	return out
}

// itemNames are the rows' item texts, group labels left out.
func itemNames(rows []railRow) []string {
	var out []string
	for _, r := range rows {
		if n, ok := r.item(); ok {
			out = append(out, n)
		}
	}
	return out
}

// nickGroupNames are the room's member groups, in pane order: by power level.
var nickGroupNames = [...]string{"admins", "mods", "members"}

// levelGroup is the index in nickGroupNames of a power level: admin 100+, mod 50+, else member.
func levelGroup(level int) int {
	switch {
	case level >= 100:
		return 0
	case level >= 50:
		return 1
	}
	return 2
}

// nickRows is the nick pane of the current channel: rows with a group label above each group of
// a room (admins first). rowOf maps each chanNicks index (0 = me) to its row. Without grouping
// (not a room, or filtering) rows are the plain pane and rowOf is the identity.
func (a App) nickRows() (rows []railRow, rowOf []int) {
	cur := a.m.Current()
	out := items(a.paneNicks())
	plain := func() ([]railRow, []int) {
		rowOf = make([]int, len(out))
		for i := range rowOf {
			rowOf[i] = i
		}
		return out, rowOf
	}
	if cur.Kind != model.RoomChan || a.fq != "" || len(out) != len(cur.Members)+1 {
		return plain()
	}
	rows = []railRow{out[0]}
	rowOf = make([]int, len(out))
	for g, name := range nickGroupNames {
		var members []int
		for j, m := range cur.Members {
			if levelGroup(m.Level) == g {
				members = append(members, j)
			}
		}
		if len(members) == 0 {
			continue
		}
		rows = append(rows, railRow{text: name, label: true})
		for _, j := range members {
			rows = append(rows, out[j+1])
			rowOf[j+1] = len(rows) - 1
		}
	}
	return rows, rowOf
}

// chanGroupNames are the channel pane's groups, in pane order.
var chanGroupNames = [...]string{"favourites", "rooms", "people", "low priority"}

// chanGroup is the index in chanGroupNames of channel c with tag group tag ("favourite", "lowpriority", "").
func chanGroup(c model.Channel, tag string) int {
	switch {
	case tag == "favourite":
		return 0
	case tag == "lowpriority":
		return 3
	case tag == "dm":
		return 2
	case strings.HasPrefix(c.Name, "@"):
		return 2
	}
	return 1
}

// chanRows is the channel pane: favourites, then each space (a tag "space:name"), rooms, people and low
// priority, each channel group under a label row when more than one group has channels.
// Label rows are not channels.
func (a App) chanRows() []railRow {
	var groups [len(chanGroupNames)][]string
	var spaceOrder []string
	spaceRooms := map[string][]string{}
	for _, c := range a.m.All() {
		tag := a.tags[c.Name]
		if name, ok := strings.CutPrefix(tag, "space:"); ok {
			if _, seen := spaceRooms[name]; !seen {
				spaceOrder = append(spaceOrder, name)
			}
			spaceRooms[name] = append(spaceRooms[name], c.Name)
			continue
		}
		g := chanGroup(c, tag)
		groups[g] = append(groups[g], c.Name)
	}
	type section struct {
		label string
		names []string
	}
	var sections []section
	if len(groups[0]) > 0 {
		sections = append(sections, section{chanGroupNames[0], groups[0]})
	}
	for _, s := range spaceOrder {
		sections = append(sections, section{s, spaceRooms[s]})
	}
	for i := 1; i < len(groups); i++ {
		if len(groups[i]) > 0 {
			sections = append(sections, section{chanGroupNames[i], groups[i]})
		}
	}
	var out []railRow
	for _, s := range sections {
		if len(sections) > 1 {
			out = append(out, railRow{text: s.label, label: true})
		}
		out = append(out, items(s.names)...)
	}
	return out
}
