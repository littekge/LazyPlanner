package model

import (
	"fmt"
	"sort"
	"time"

	"github.com/emersion/go-ical"
	"github.com/teambition/rrule-go"
)

// Occurrence is a single materialized instance of an event within a queried
// window. A non-recurring event yields at most one. Start and End are the
// instance's concrete times; Event points to the underlying component — the
// series master, or a RECURRENCE-ID override for a modified instance — so the
// UI can show details and route edits to the right resource.
type Occurrence struct {
	Start time.Time
	End   time.Time
	Event *Event
}

// Occurrences expands this event's own recurrence within the half-open window
// [from, to) and returns every instance overlapping it, in chronological order.
// Recurrence comes from the event's RRULE, RDATE, and EXDATE properties
// anchored at its DTSTART; a non-recurring event yields at most its single
// instance. Expansion is timezone-aware: instances keep the event's wall-clock
// time across DST transitions, matching other CalDAV clients.
//
// Occurrences considers only this one component. RECURRENCE-ID overrides, which
// live in sibling components, are applied by Parsed.EventOccurrences.
func (e *Event) Occurrences(from, to time.Time) ([]Occurrence, error) {
	dur := e.Duration()

	hasRRULE := e.Raw.Props.Get(ical.PropRecurrenceRule) != nil
	hasRDATE := len(e.Raw.Props.Values(ical.PropRecurrenceDates)) > 0

	if !hasRRULE && !hasRDATE {
		if overlaps(e.Start, e.Start.Add(dur), from, to) {
			return []Occurrence{{Start: e.Start, End: e.Start.Add(dur), Event: e}}, nil
		}
		return nil, nil
	}

	set, err := e.recurrenceSet(hasRRULE)
	if err != nil {
		return nil, err
	}

	// Start the query one duration early so an instance that begins before the
	// window but runs into it is still found — Between filters on start alone.
	var out []Occurrence
	for _, start := range set.Between(from.Add(-dur), to, true) {
		end := start.Add(dur)
		if overlaps(start, end, from, to) {
			out = append(out, Occurrence{Start: start, End: end, Event: e})
		}
	}
	return out, nil
}

// Duration returns the event's length, or zero when the end is absent or not
// after the start (a point-in-time event).
func (e *Event) Duration() time.Duration {
	if e.End.After(e.Start) {
		return e.End.Sub(e.Start)
	}
	return 0
}

// recurrenceSet builds the rrule.Set from RRULE, RDATE, and EXDATE, anchored at
// DTSTART in the start's location so DST is handled correctly. With no RRULE,
// DTSTART is added explicitly: it belongs to the recurrence set per RFC 5545,
// but rrule-go emits it only through an RRULE.
func (e *Event) recurrenceSet(hasRRULE bool) (*rrule.Set, error) {
	loc := e.Start.Location()
	set := &rrule.Set{}
	set.DTStart(e.Start)

	var roption *rrule.ROption
	if hasRRULE {
		var err error
		roption, err = e.Raw.Props.RecurrenceRule()
		if err != nil {
			return nil, fmt.Errorf("event %q: parsing RRULE: %w", e.UID, err)
		}
	}
	if roption != nil {
		roption.Dtstart = e.Start
		rule, err := rrule.NewRRule(*roption)
		if err != nil {
			return nil, fmt.Errorf("event %q: building recurrence: %w", e.UID, err)
		}
		set.RRule(rule)
	} else {
		set.RDate(e.Start)
	}

	for _, prop := range e.Raw.Props.Values(ical.PropRecurrenceDates) {
		// resolveDateTime (not prop.DateTime) so a Windows/Outlook TZID on an
		// RDATE recovers the same way DTSTART does, instead of erroring out and
		// blanking the whole event's expansion.
		dt, err := resolveDateTime(&prop, loc)
		if err != nil {
			return nil, fmt.Errorf("event %q: parsing RDATE: %w", e.UID, err)
		}
		set.RDate(dt)
	}
	for _, prop := range e.Raw.Props.Values(ical.PropExceptionDates) {
		dt, err := resolveDateTime(&prop, loc)
		if err != nil {
			return nil, fmt.Errorf("event %q: parsing EXDATE: %w", e.UID, err)
		}
		set.ExDate(dt)
	}
	return set, nil
}

// EventOccurrences expands every event in the parsed object within [from, to),
// applying RECURRENCE-ID overrides. A component that shares a master's UID but
// carries a RECURRENCE-ID replaces the single instance it identifies: the
// master's instance in that slot is suppressed and the override contributes its
// own instance (at its possibly-moved DTSTART, with its own details). An
// override whose UID has no master is treated as a standalone instance.
// Results are sorted by start time.
//
// The RANGE=THISANDFUTURE parameter is not yet handled — such an override
// affects only its own instance here. That refinement can land with the
// recurrence-editing step.
func (p *Parsed) EventOccurrences(from, to time.Time) ([]Occurrence, error) {
	masters := map[string]*Event{}
	overrides := map[string][]*Event{}
	var uidOrder []string
	seen := map[string]bool{}

	for _, ev := range p.Events {
		if !seen[ev.UID] {
			seen[ev.UID] = true
			uidOrder = append(uidOrder, ev.UID)
		}
		if _, ok := recurrenceID(ev); ok {
			overrides[ev.UID] = append(overrides[ev.UID], ev)
		} else {
			masters[ev.UID] = ev
		}
	}

	var out []Occurrence
	for _, uid := range uidOrder {
		// Slots (by second) that an override has taken over from the master.
		replaced := map[int64]bool{}
		for _, ov := range overrides[uid] {
			if rid, ok := recurrenceID(ov); ok {
				replaced[rid.Unix()] = true
			}
		}

		if master := masters[uid]; master != nil {
			occs, err := master.Occurrences(from, to)
			if err != nil {
				return nil, err
			}
			for _, occ := range occs {
				if !replaced[occ.Start.Unix()] {
					out = append(out, occ)
				}
			}
		}

		for _, ov := range overrides[uid] {
			dur := ov.Duration()
			if overlaps(ov.Start, ov.Start.Add(dur), from, to) {
				out = append(out, Occurrence{Start: ov.Start, End: ov.Start.Add(dur), Event: ov})
			}
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out, nil
}

// recurrenceID returns the instant an override targets in its series, from its
// RECURRENCE-ID property. ok is false for a master (no RECURRENCE-ID).
func recurrenceID(e *Event) (time.Time, bool) {
	prop := e.Raw.Props.Get(ical.PropRecurrenceID)
	if prop == nil {
		return time.Time{}, false
	}
	// resolveDateTime so a Windows/Outlook TZID resolves (matching how the
	// master's DTSTART is parsed); prop.DateTime would fail on such a zone and
	// the override would be misclassified as a second master, dropping the series.
	t, err := resolveDateTime(prop, e.Start.Location())
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// overlaps reports whether [aStart, aEnd) intersects the half-open window
// [bStart, bEnd). A zero-length instance (aStart == aEnd) is treated as the
// instant aStart.
func overlaps(aStart, aEnd, bStart, bEnd time.Time) bool {
	if !aEnd.After(aStart) {
		return !aStart.Before(bStart) && aStart.Before(bEnd)
	}
	return aStart.Before(bEnd) && aEnd.After(bStart)
}
