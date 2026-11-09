package ui

// Motion is ported from tuios, internal/ui/animation.go (github.com/Gaurav-Gosain/tuios): a value moves to
// its target as a critically damped spring, the curve that closes as fast as a spring can without passing
// the target. The position is a closed form of the time since the last (re)start, so it does not depend
// on how often frames are drawn; a new target mid-flight carries the velocity, so the motion bends instead
// of restarting from rest.
// MIT License, Copyright (c) 2025 Gaurav Gosain.

import (
	"math"
	"time"

	tea "charm.land/bubbletea/v2"
)

// springStiffness is the angular frequency times the duration: released from rest, (1+k)e^-k of the
// distance is left when the clock runs out, 0.7% at 7, so landing exactly on the target is no visible step.
const springStiffness = 7.0

// Durations: tuios's standard 300 ms, and its fast two thirds for popups.
const (
	glideDur = 300 * time.Millisecond
	popDur   = 200 * time.Millisecond
	frameDur = 16 * time.Millisecond // one frame at 60 Hz while something moves
)

// spring is one value on its way to a target: at start it was off from it at vel units per second.
type spring struct {
	to, off, vel float64
	start        time.Time
	dur          time.Duration
}

// newSpring is a spring at rest at from, heading to to.
func newSpring(from, to float64, now time.Time, dur time.Duration) spring {
	return spring{to: to, off: from - to, start: now, dur: dur}
}

// at is the value and velocity at now, and whether the spring has landed: the clock ran out, or it is
// within half a unit of the target, which draws the target, so there is nothing left to show.
func (s spring) at(now time.Time) (pos, vel float64, done bool) {
	t := max(now.Sub(s.start).Seconds(), 0)
	if s.dur <= 0 || t >= s.dur.Seconds() {
		return s.to, 0, true
	}
	w := springStiffness / s.dur.Seconds()
	decay := math.Exp(-w * t)
	b := s.vel + w*s.off
	off := (s.off + b*t) * decay
	vel = (s.vel - w*b*t) * decay
	if s.off != 0 && math.Signbit(off) != math.Signbit(s.off) { // carried past the target: it stops there
		off, vel = 0, 0
	}
	return s.to + off, vel, math.Abs(off) < 0.5
}

// cell is the spring's value at now rounded to a cell, and whether it has landed.
func (s spring) cell(now time.Time) (int, bool) {
	pos, _, done := s.at(now)
	if done {
		return int(math.Round(s.to)), true
	}
	return int(math.Round(pos)), false
}

// retarget heads the spring to to from where it is at now, keeping its velocity.
func (s spring) retarget(to float64, now time.Time, dur time.Duration) spring {
	pos, vel, _ := s.at(now)
	return spring{to: to, off: pos - to, vel: vel, start: now, dur: dur}
}

// frameMsg asks for the next frame of the springs in flight.
type frameMsg struct{}

func nextFrame() tea.Cmd { return tea.Tick(frameDur, func(time.Time) tea.Msg { return frameMsg{} }) }
