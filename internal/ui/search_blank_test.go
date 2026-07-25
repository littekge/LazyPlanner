package ui

import (
	"strings"
	"testing"
	"time"
)

// TestBlankSearchQueryDoesNotActivateSearch guards the whitespace-only search
// defect: `/` + Space + Enter used to store searchQuery=" " (runSearch returned
// early without selecting or flashing), and `n` then matched every row because
// matchIndices trims the query to "" — so the selection jumped and the status bar
// reported a match count for a search the user never made.
func TestBlankSearchQueryDoesNotActivateSearch(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeCalendar)
	if a.calendars.GetItemCount() < 2 {
		t.Skipf("fixture needs >=2 calendars, has %d", a.calendars.GetItemCount())
	}
	a.calendars.SetCurrentItem(0)

	a.runSearch(" ") // typing Space into the `/` input

	if a.searchQuery != "" {
		t.Errorf("whitespace-only query was stored as active search: %q", a.searchQuery)
	}

	before := a.calendars.GetCurrentItem()
	a.searchNext(1) // `n`

	if got := a.calendars.GetCurrentItem(); got != before {
		t.Errorf("n moved the selection %d -> %d on a blank search", before, got)
	}
	if got := a.statusLeft.GetText(true); !strings.Contains(got, "no active search") {
		t.Errorf("expected a no-active-search flash, got %q", got)
	}
}

// TestBlankGuardStillAllowsRealSearches is the other half of the blank-query
// class: the guard must reject only *effectively* empty queries. A legitimate
// query — including one the user padded with spaces — still activates the search,
// moves the selection and cycles under n.
func TestBlankGuardStillAllowsRealSearches(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)

	for _, q := range []string{"beta", "  beta  ", "\tbeta"} {
		t.Run(strings.TrimSpace(q)+"/"+q, func(t *testing.T) {
			a := newWritableTestApp(t, now)
			a.setMode(modeTasks)
			cal := a.selectedTasklistID()
			for _, s := range []string{"Alpha", "Beta apples", "Gamma", "Beta bananas"} {
				a.createTask(cal, "", s)
			}
			a.buildTree()

			a.runSearch(q)
			if a.searchQuery == "" {
				t.Fatalf("query %q was rejected as blank", q)
			}
			first := currentTaskSummary(a)
			if !strings.Contains(strings.ToLower(first), "beta") {
				t.Fatalf("search landed on %q, want a Beta match", first)
			}

			a.searchNext(1)
			if second := currentTaskSummary(a); second == first {
				t.Errorf("n did not advance (still %q)", second)
			} else if !strings.Contains(strings.ToLower(second), "beta") {
				t.Errorf("n landed on %q, not a Beta match", second)
			}
			if got := a.statusLeft.GetText(true); strings.Contains(got, "no active search") {
				t.Errorf("real search reported as inactive: %q", got)
			}
		})
	}
}
