package model

import (
	"strings"
	"testing"
	"time"
)

// TestParseQuickAddNoFalseWarnings is the adversarial zero-warning table from
// the v1.2.0 plan: plausible titles that must NEVER produce a warning. Asserted
// verbatim — the warning system's whole value is that it fires only on an
// unmistakable intent anchor, so a false positive is the failure mode.
func TestParseQuickAddNoFalseWarnings(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, loc)

	silent := []string{
		"My Event!!!!!",
		"My Event !!!!",
		"do it !",
		"lunch @ noon",
		"My Title@house",
		"test task!5",
		"email bob@example.com",
		"24/7 support",
		"graded 7/45 on quiz",
		"plan next steps fri",
		"in 3 acts",
		"http://x.com",
	}
	for _, in := range silent {
		qa := ParseQuickAdd(in, now, loc)
		if len(qa.Warnings) != 0 {
			t.Errorf("%q: unexpected warning(s) %v", in, qa.Warnings)
		}
	}
}

// TestParseQuickAddWarns covers the four warning classes: a failed/duplicate
// priority, an unclosed quote, an anchor-word fuzzy near-miss, and shape
// triggers (invalid times, ranges, and dates).
func TestParseQuickAddWarns(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, loc)

	warns := []struct {
		name  string
		input string
	}{
		{"priority letters", "task !hgh"},
		{"priority single letter", "task !t"},
		{"priority zero", "task !0"},
		{"duplicate priority", "pay rent !high !low"},
		{"unclosed quoted location", "class @\"room 204"},
		{"fuzzy weekday after next", "call next tuedsay"},
		{"fuzzy unit after in N", "ship in 3 dayz"},
		{"impossible 24h time", "sync 25:00"},
		{"impossible minutes", "sync 12:99"},
		{"failed range shape", "meet 5-6xm"},
		{"incomplete range", "meet 5pm-"},
		{"impossible iso date", "due 2026-07-40"},
		{"impossible mdy date", "due 2/30/2026"},
	}
	for _, tc := range warns {
		t.Run(tc.name, func(t *testing.T) {
			qa := ParseQuickAdd(tc.input, now, loc)
			if len(qa.Warnings) == 0 {
				t.Errorf("%q: expected a warning, got none", tc.input)
			}
		})
	}
}

// TestParseQuickAddCorrectSpellingsAreSilent guards that the correctly-spelled
// forms the warnings shadow do not themselves warn.
func TestParseQuickAddCorrectSpellingsAreSilent(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, loc)

	silent := []string{
		"call next tuesday",
		"ship in 3 days",
		"class @\"room 204\"",
		"meet 5pm-6pm",
		"due 2026-07-20",
		"party !high #home",
		"rent monthly every mon",
	}
	for _, in := range silent {
		qa := ParseQuickAdd(in, now, loc)
		if len(qa.Warnings) != 0 {
			t.Errorf("%q: unexpected warning(s) %v", in, qa.Warnings)
		}
	}
}

// TestQuickAddWarningNamesToken checks a warning message names the offending
// token, so the re-prompt is actionable.
func TestQuickAddWarningNamesToken(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, loc)
	qa := ParseQuickAdd("call next tuedsay", now, loc)
	if len(qa.Warnings) == 0 || !strings.Contains(qa.Warnings[0], "tuedsay") {
		t.Errorf("warning should name the token, got %v", qa.Warnings)
	}
}
