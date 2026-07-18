package model

import (
	"testing"
	"time"
)

// TestQuickAddInvalidDayOfMonth guards the fix for the regression where an
// invalid day-of-month in the slashed and month-name forms was silently
// normalized by time.Date into a wrong date (Feb 30 -> Mar 2) and rolled a year
// forward, while the ISO form correctly rejected it. An impossible day must now
// stay in the title in every form, so the same input can't parse one way slashed
// and another way as ISO.
func TestQuickAddInvalidDayOfMonth(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, loc)

	invalid := []string{
		"x 2/30",       // Feb has 28 days in 2026
		"x feb 30",     // month-name form
		"x 4/31",       // Apr has 30
		"x jun 31",     // Jun has 30
		"x 2/30/2026",  // explicit-year slashed form
		"x 2026-02-30", // ISO reference: already rejected
		"x feb 29",     // 2026 and 2027 are both non-leap
	}
	for _, in := range invalid {
		qa := ParseQuickAdd(in, now, loc)
		if qa.HasDate {
			t.Errorf("%q: impossible day parsed as a date %s (want it left in the title, Title=%q)",
				in, qa.Date.Format("2006-01-02"), qa.Title)
		}
	}

	// Valid dates must still parse — the fix must not over-reject.
	valid := map[string]string{
		"x 2/28":       "2027-02-28", // Feb 28 2026 already past -> next year
		"x feb 28":     "2027-02-28",
		"x 7/20":       "2026-07-20", // later this year
		"x 7/10":       "2027-07-10", // already past -> rolls to next year
		"x 2/29/2028":  "2028-02-29", // leap year, explicit
		"x 2026-07-20": "2026-07-20",
	}
	for in, want := range valid {
		qa := ParseQuickAdd(in, now, loc)
		if !qa.HasDate {
			t.Errorf("%q: valid date left in title (Title=%q)", in, qa.Title)
			continue
		}
		if got := qa.Date.Format("2006-01-02"); got != want {
			t.Errorf("%q: got date %s, want %s", in, got, want)
		}
	}
}
