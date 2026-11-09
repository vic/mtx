package ui

import (
	"math"
	"strconv"

	tea "charm.land/bubbletea/v2"
)

// popRise is how many rows below its place an opening popup starts.
const popRise = 3

// glideMin is the least jump of the chat that glides: a line step or a wheel notch moves at once, as a
// direct manipulation should.
const glideMin = 4

// WithMotion turns the springs on: popups slide in and chat jumps glide.
func (a App) WithMotion() App {
	a.motion = true
	return a
}

// popKey names the popup on screen, "" for none: a new name is a popup opening.
func (a App) popKey() string {
	switch {
	case a.img != "":
		return "img"
	case a.verifySAS != "":
		return "verify"
	case a.help:
		return "help"
	case a.pick:
		return "pick:" + strconv.Itoa(int(a.pickWhat)) + a.listTitle
	case a.ctxOn():
		return "ctx"
	case a.menu != "":
		return "menu:" + a.menu
	}
	return ""
}

// popDY is how many rows below its place the popup is drawn now.
func (a App) popDY() int {
	if a.pop == nil {
		return 0
	}
	dy, _ := a.pop.cell(a.now())
	return dy
}

// shownOff is the chat offset drawn now: off(), or on its way there.
func (a App) shownOff() int {
	if a.glide == nil {
		return a.off()
	}
	off, _ := a.glide.cell(a.now())
	return off
}

// direct reports whether msg is the pointer acting on the screen: what it moves follows it at once.
func direct(msg tea.Msg) bool {
	switch msg.(type) {
	case tea.MouseClickMsg, tea.MouseMotionMsg, tea.MouseReleaseMsg, tea.MouseWheelMsg, WheelMsg:
		return true
	}
	return false
}

// motionAfter starts, carries and lands the springs for the step from prev to a, and asks for the next
// frame while one moves.
func (a App) motionAfter(prev App, msg tea.Msg) (App, tea.Cmd) {
	if !a.motion {
		return a, nil
	}
	now := a.now()
	switch k := a.popKey(); {
	case k == "":
		a.pop = nil
	case k != prev.popKey():
		s := newSpring(popRise, 0, now, popDur)
		a.pop = &s
	}
	switch from := prev.shownOff(); {
	case prev.m.Current().Name != a.m.Current().Name, direct(msg):
		a.glide = nil
	case prev.off() != a.off() && math.Abs(float64(a.off()-from)) >= glideMin:
		s := newSpring(float64(from), float64(a.off()), now, glideDur)
		if prev.glide != nil {
			s = prev.glide.retarget(float64(a.off()), now, glideDur)
		}
		a.glide = &s
	}
	if a.pop != nil {
		if _, done := a.pop.cell(now); done {
			a.pop = nil
		}
	}
	if a.glide != nil {
		if _, done := a.glide.cell(now); done {
			a.glide = nil
		}
	}
	if _, ok := msg.(frameMsg); ok {
		a.moving = false
	}
	if (a.pop != nil || a.glide != nil) && !a.moving {
		a.moving = true
		return a, nextFrame()
	}
	return a, nil
}
