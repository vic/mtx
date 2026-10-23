package ui

import "mtx/internal/model"

// nickLabel marks a group label row in the nick pane (never a selectable nick).
const nickLabel = "\x00"

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
func (a App) nickRows() (rows []string, rowOf []int) {
	cur := a.m.Current()
	out := a.paneNicks()
	plain := func() ([]string, []int) {
		rowOf = make([]int, len(out))
		for i := range rowOf {
			rowOf[i] = i
		}
		return out, rowOf
	}
	if cur.Kind != model.RoomChan || a.fq != "" || len(out) != len(cur.Members)+1 {
		return plain()
	}
	rows = []string{out[0]}
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
		rows = append(rows, nickLabel+name)
		for _, j := range members {
			rows = append(rows, out[j+1])
			rowOf[j+1] = len(rows) - 1
		}
	}
	return rows, rowOf
}
