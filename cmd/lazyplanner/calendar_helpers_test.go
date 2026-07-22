package main

import (
	"reflect"
	"testing"
)

// TestComponents closes a Pass 18 canary hole: the --tasks→VTODO / --both / default
// →VEVENT mapping had zero coverage, so a swapped mapping (events↔tasks) would ship
// silently. It pins each flag combination to its component set.
func TestComponents(t *testing.T) {
	tests := []struct {
		name        string
		tasks, both bool
		want        []string
	}{
		{"default is an event calendar", false, false, []string{"VEVENT"}},
		{"--tasks is a VTODO task list", true, false, []string{"VTODO"}},
		{"--both supports each", false, true, []string{"VEVENT", "VTODO"}},
		{"--both wins over --tasks", true, true, []string{"VEVENT", "VTODO"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := components(tt.tasks, tt.both); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("components(tasks=%v, both=%v) = %v, want %v", tt.tasks, tt.both, got, tt.want)
			}
		})
	}
}

// TestSlugify pins the name→collection-path-segment rules (also previously
// uncovered): lower-cased, non-alphanumerics collapsed to single dashes, trimmed,
// with a fallback for an all-punctuation name.
func TestSlugify(t *testing.T) {
	tests := []struct{ in, want string }{
		{"My Calendar", "my-calendar"},
		{"  Work / Home  ", "work-home"},
		{"ECE384!!!", "ece384"},
		{"", "calendar"},
		{"???", "calendar"},
	}
	for _, tt := range tests {
		if got := slugify(tt.in); got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestJoinWarnings pins the two-string join used by the :config reload flash.
func TestJoinWarnings(t *testing.T) {
	tests := []struct{ a, b, want string }{
		{"", "", ""},
		{"x", "", "x"},
		{"", "y", "y"},
		{"x", "y", "x; y"},
	}
	for _, tt := range tests {
		if got := joinWarnings(tt.a, tt.b); got != tt.want {
			t.Errorf("joinWarnings(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.want)
		}
	}
}
