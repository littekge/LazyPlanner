package model

import "testing"

// TestParseTimeHalfHourCeiling closes a Pass-20 mutation-canary escape: the
// 24-hour (no am/pm) branch of parseTimeHalf caps the hour at 23, but no test fed
// an out-of-range hour, so flipping the ceiling from `> 23` to `> 24` (accepting
// the invalid 24:00, which time.Date spills to the next day) went uncaught.
func TestParseTimeHalfHourCeiling(t *testing.T) {
	tests := []struct {
		tok      string
		wantHour int
		wantOK   bool
	}{
		{"0:00", 0, true},
		{"23:00", 23, true},
		{"23:59", 23, true},
		{"24:00", 0, false}, // invalid — the boundary the canary flipped
		{"25:00", 0, false},
		{"99:00", 0, false},
	}
	for _, tc := range tests {
		h, _, ok := parseTimeHalf(tc.tok, "")
		if ok != tc.wantOK {
			t.Errorf("parseTimeHalf(%q) ok = %v, want %v", tc.tok, ok, tc.wantOK)
			continue
		}
		if ok && h != tc.wantHour {
			t.Errorf("parseTimeHalf(%q) hour = %d, want %d", tc.tok, h, tc.wantHour)
		}
	}
	// The am/pm branches use a 1-12 clock — pin those ceilings too while here.
	if _, _, ok := parseTimeHalf("13pm", ""); ok {
		t.Error("parseTimeHalf(13pm) must be rejected (12-hour clock)")
	}
	if _, _, ok := parseTimeHalf("0am", ""); ok {
		t.Error("parseTimeHalf(0am) must be rejected (12-hour clock has no hour 0)")
	}
}
