package model

import (
	"testing"
	"time"
)

func TestWindowsToIANA(t *testing.T) {
	cases := map[string]string{
		"Eastern Standard Time":   "America/New_York",
		"W. Europe Standard Time": "Europe/Berlin",
		"Pacific Standard Time":   "America/Los_Angeles",
		"Not A Zone":              "",
	}
	for name, want := range cases {
		if got := windowsToIANA(name); got != want {
			t.Errorf("windowsToIANA(%q) = %q, want %q", name, got, want)
		}
	}
}

// TestWindowsZonesResolve guards that every mapped IANA name actually loads with
// the embedded tz database — a typo in the table would otherwise silently fall
// through to the floating-time path.
func TestWindowsZonesResolve(t *testing.T) {
	for win, iana := range windowsZones {
		if _, err := time.LoadLocation(iana); err != nil {
			t.Errorf("%q -> %q does not load: %v", win, iana, err)
		}
	}
}
