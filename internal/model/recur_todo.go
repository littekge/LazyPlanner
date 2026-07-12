package model

import "time"

// DuesInRange returns this todo's due instants within [from, to). A non-recurring
// todo yields its single due (when in range); a recurring todo expands its RRULE
// anchored at the due date, so it appears on every occurrence's due day across the
// calendar and agenda — not only its current one.
//
// Recurrence is anchored at DUE (not DTSTART) because the due date is what a task
// is shown on; the two have the same cadence, so this yields the same occurrences
// a DTSTART-anchored expansion would, shifted onto the due dates. Occurrences
// before the current due are never produced (the series starts at the current
// due), so a task that has been advanced past never shows its passed occurrences.
func (t *Todo) DuesInRange(from, to time.Time) []time.Time {
	if !t.HasDue {
		return nil
	}
	inRange := func(x time.Time) bool { return !x.Before(from) && x.Before(to) }
	if !t.Recurring {
		if inRange(t.Due) {
			return []time.Time{t.Due}
		}
		return nil
	}
	set, err := componentRecurrenceSet(t.Raw, t.Due)
	if err != nil {
		if inRange(t.Due) {
			return []time.Time{t.Due}
		}
		return nil
	}
	var out []time.Time
	for _, x := range set.Between(from, to, true) {
		if inRange(x) {
			out = append(out, x)
		}
	}
	return out
}
