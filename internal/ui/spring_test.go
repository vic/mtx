package ui

import (
	"math"
	"testing"
	"time"
)

func TestSpringLandsWithoutOvershoot(t *testing.T) {
	t0 := time.Unix(0, 0)
	s := newSpring(20, 0, t0, glideDur)
	if p, _, done := s.at(t0); p != 20 || done {
		t.Fatalf("at the start: %v done %v", p, done)
	}
	last := 20.0
	for ms := 16; ms < 300; ms += 16 {
		p, _, _ := s.at(t0.Add(time.Duration(ms) * time.Millisecond))
		if p > last || p < 0 {
			t.Fatalf("NEG at %d ms the spring went back or past the target: %v after %v", ms, p, last)
		}
		last = p
	}
	if p, _, done := s.at(t0.Add(glideDur)); p != 0 || !done {
		t.Fatalf("when the clock runs out it is on the target: %v done %v", p, done)
	}
	// (1+k)e^-k of the distance is left at the end of the duration: 0.7% at k = 7
	if p, _, _ := s.at(t0.Add(glideDur - time.Nanosecond)); math.Abs(p) > 20*0.0073 {
		t.Fatalf("residual %v", p)
	}
}

// A new target mid-flight keeps the velocity: the motion bends, it does not restart from rest.
func TestSpringRetargetKeepsVelocity(t *testing.T) {
	t0 := time.Unix(0, 0)
	mid := t0.Add(50 * time.Millisecond)
	s := newSpring(0, 30, t0, glideDur)
	p, v, _ := s.at(mid)
	r := s.retarget(60, mid, glideDur)
	if q, w, _ := r.at(mid); math.Abs(q-p) > 1e-9 || math.Abs(w-v) > 1e-9 || v == 0 {
		t.Fatalf("retarget at %v/%v, was %v/%v", q, w, p, v)
	}
}
