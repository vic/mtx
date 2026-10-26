package model

import (
	"errors"
	"reflect"
	"testing"
)

func TestRing(t *testing.T) {
	tests := []struct {
		name string
		cap  int
		push []int
		want []int
	}{
		{"empty", 3, nil, []int{}},
		{"under cap", 3, []int{1, 2}, []int{1, 2}},
		{"exactly cap", 3, []int{1, 2, 3}, []int{1, 2, 3}},
		{"overflow drops oldest", 3, []int{1, 2, 3, 4, 5}, []int{3, 4, 5}},
		{"cap one", 1, []int{1, 2}, []int{2}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, err := NewRing[int](tc.cap)
			if err != nil {
				t.Fatal(err)
			}
			for _, v := range tc.push {
				r.Push(v)
			}
			if got := r.All(); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			if r.Len() != len(tc.want) {
				t.Fatalf("len %d", r.Len())
			}
		})
	}
}

func TestRingBadCap(t *testing.T) {
	for _, c := range []int{0, -1} {
		if _, err := NewRing[int](c); !errors.Is(err, ErrBadCap) {
			t.Fatalf("cap %d: got %v", c, err)
		}
	}
}

func TestRingZeroValuePush(t *testing.T) {
	var r Ring[int]
	r.Push(1)
	if r.Len() != 0 || len(r.All()) != 0 {
		t.Fatalf("zero ring held items: %v", r.All())
	}
}

// TestRingCopiesStayApart: a push on a copy never shows in the original, even when both push.
func TestRingCopiesStayApart(t *testing.T) {
	a, err := NewRing[int](3)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []int{1, 2} {
		a.Push(v)
	}
	b := a
	b.Push(3)
	a.Push(9)
	b.Push(4)
	if got := a.All(); !reflect.DeepEqual(got, []int{1, 2, 9}) {
		t.Fatalf("a = %v", got)
	}
	if got := b.All(); !reflect.DeepEqual(got, []int{2, 3, 4}) {
		t.Fatalf("b = %v", got)
	}
	for i := range 10 { // wraps buf several times
		b.Push(10 + i)
	}
	if got := b.All(); !reflect.DeepEqual(got, []int{17, 18, 19}) || b.At(0) != 17 {
		t.Fatalf("b = %v", got)
	}
}
