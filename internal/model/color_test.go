package model_test

import (
	"testing"

	"github.com/littekge/LazyPlanner/internal/model"
)

func TestNearestANSI16(t *testing.T) {
	cases := []struct {
		in   string
		want int // palette index
		ok   bool
	}{
		{"#ff0000", 9, true},   // pure red → bright red
		{"#ff0000ff", 9, true}, // alpha ignored
		{"ff0000", 9, true},    // '#' optional
		{"#00ff00", 10, true},  // bright green (lime)
		{"#0000ff", 12, true},  // bright blue
		{"#000000", 0, true},   // black
		{"#ffffff", 15, true},  // white
		{"#010203", 0, true},   // near-black → black
		{"#fe0101", 9, true},   // almost pure red → bright red
		{"#7f0000", 1, true},   // mid red → maroon (dark red)
		{"#008080", 6, true},   // teal
		{"#FF2968FF", 9, true}, // Apple pink-red → bright red (nearest)
		{"", 0, false},         // empty → not ok
		{"#12345", 0, false},   // wrong length
		{"#gggggg", 0, false},  // non-hex
		{"nonsense", 0, false}, // garbage
	}
	for _, c := range cases {
		got, ok := model.NearestANSI16(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("NearestANSI16(%q) = (%d, %v), want (%d, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestANSI16IsDark(t *testing.T) {
	// By Rec. 601 luminance at a midpoint threshold (see ANSI16IsDark).
	dark := []int{0, 1, 2, 3, 4, 5, 6, 9, 12, 13}
	light := []int{7, 8, 10, 11, 14, 15}
	for _, i := range dark {
		if !model.ANSI16IsDark(i) {
			t.Errorf("ANSI16IsDark(%d) = false, want true (dark)", i)
		}
	}
	for _, i := range light {
		if model.ANSI16IsDark(i) {
			t.Errorf("ANSI16IsDark(%d) = true, want false (light)", i)
		}
	}
	// Out of range is defensively "not dark".
	if model.ANSI16IsDark(99) {
		t.Error("ANSI16IsDark(99) should be false for out-of-range")
	}
}
