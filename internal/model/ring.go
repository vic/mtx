package model

import (
	"errors"
	"fmt"
)

// ErrBadCap means a ring capacity below 1.
var ErrBadCap = errors.New("model: ring capacity must be >= 1")

// Ring is a fixed-capacity buffer that drops the oldest item when full. Its items are
// buf[start:start+n]; buf has room to append up to len(buf).
type Ring[T any] struct {
	buf      []T
	start, n int
	cap      int
	ver      uint64 // bumped by every change: a reader can tell the ring moved
	end      *int   // shared by the copies over buf: where the newest one's items end
}

// Cap is a validated ring capacity (>= 1 when made by NewCap).
type Cap int

// NewCap validates n as a ring capacity.
func NewCap(n int) (Cap, error) {
	if n < 1 {
		return 0, fmt.Errorf("%w: %d", ErrBadCap, n)
	}
	return Cap(n), nil
}

// RingOf returns an empty ring of capacity c; it cannot fail. Storage comes with the first Push.
func RingOf[T any](c Cap) Ring[T] { return Ring[T]{cap: int(c)} }

// NewRing returns an empty ring holding at most n items. Always build rings
// with NewRing or RingOf: the zero Ring has no storage and Push on it is a no-op.
func NewRing[T any](n int) (Ring[T], error) {
	c, err := NewCap(n)
	if err != nil {
		return Ring[T]{}, err
	}
	return RingOf[T](c), nil
}

// Push appends v, evicting the oldest item when full. O(1) amortized: it writes in place while this
// copy owns the end of buf, and moves the items to a new buf only when buf is full or another copy
// already appended there, so an older copy of the ring never sees the change.
func (r *Ring[T]) Push(v T) {
	r.ver++
	if r.cap == 0 {
		return
	}
	if r.end == nil || *r.end != r.start+r.n || r.start+r.n == len(r.buf) {
		r.move()
	}
	r.buf[r.start+r.n] = v
	if r.n < r.cap {
		r.n++
	} else {
		r.start++
	}
	*r.end = r.start + r.n
}

// move copies the items to a new buf of 2×cap that this copy alone owns.
func (r *Ring[T]) move() {
	buf := make([]T, 2*r.cap)
	copy(buf, r.buf[r.start:r.start+r.n])
	r.buf, r.start, r.end = buf, 0, new(int)
	*r.end = r.n
}

// Len is the number of items held.
func (r *Ring[T]) Len() int { return r.n }

// At is item i, oldest first (0 <= i < Len); no copy.
func (r *Ring[T]) At(i int) T { return r.buf[r.start+i] }

// All returns a copy of the items, oldest first.
func (r *Ring[T]) All() []T {
	out := make([]T, r.n)
	copy(out, r.buf[r.start:r.start+r.n])
	return out
}

// clone returns a Ring with its own storage.
func (r Ring[T]) clone() Ring[T] {
	if r.cap > 0 {
		r.move()
	}
	return r
}

// Replace applies f to every item that match selects, in place (callers clone first).
func (r *Ring[T]) Replace(match func(T) bool, f func(T) T) {
	r.ver++
	for i := r.start; i < r.start+r.n; i++ {
		if match(r.buf[i]) {
			r.buf[i] = f(r.buf[i])
		}
	}
}

// Version changes whenever the ring's content changes (Push, Replace).
func (r *Ring[T]) Version() uint64 { return r.ver }
