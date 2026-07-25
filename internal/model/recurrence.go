package model

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/teambition/rrule-go"
)

// dateOnlyLayout is the iCalendar DATE value form (RFC 5545 §3.3.4) — the value
// type UNTIL must use when the recurrence anchor is a VALUE=DATE (all-day).
const dateOnlyLayout = "20060102"

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
	return e.occurrences(from, to, nil)
}

// occurrences is Occurrences with an optional shared step budget. A nil budget
// gives the event the full per-event step cap on its own (the standalone case).
// A non-nil budget is drawn down by the steps this expansion consumes and shared
// across a batch (Parsed.EventOccurrences, or a whole redraw via the store), so a
// flood of pathological events can't multiply the per-event bound into a freeze;
// once the shared budget is spent, further events degrade to their base instance.
func (e *Event) occurrences(from, to time.Time, budget *StepBudget) ([]Occurrence, error) {
	dur := e.Duration()

	hasRRULE := e.Raw.Props.Get(ical.PropRecurrenceRule) != nil
	hasRDATE := len(e.Raw.Props.Values(ical.PropRecurrenceDates)) > 0

	if !hasRRULE && !hasRDATE {
		return e.baseInstance(from, to), nil
	}

	// Each event may step at most the per-event cap, further limited to whatever
	// the shared budget has left. An exhausted budget degrades straight to the
	// base instance without iterating — the same graceful fallback as a bad rule.
	maxSteps := maxOccurrenceSteps
	if budget != nil {
		if budget.remaining <= 0 {
			return e.baseInstance(from, to), nil
		}
		if budget.remaining < maxSteps {
			maxSteps = budget.remaining
		}
	}

	set, err := e.recurrenceSet(hasRRULE)
	if err != nil {
		// Graceful degradation (iron rule): a malformed RRULE/RDATE/EXDATE must
		// never hide the event or blank the calendar view. Fall back to the
		// single base instance at DTSTART so the event stays visible, just
		// un-expanded, instead of propagating an error that a caller might turn
		// into an empty result for every calendar.
		return e.baseInstance(from, to), nil
	}

	// Start the query one duration early so an instance that begins before the
	// window but runs into it is still found — Between filters on start alone.
	starts, used, ok := safeBetween(set, from.Add(-dur), to, maxSteps)
	if budget != nil {
		budget.remaining -= used
	}
	if !ok {
		// rrule-go panics (index out of range in calcDaySet) while iterating some
		// degenerate rules — e.g. a near-zero DTSTART year. Degrade to the base
		// instance, the same graceful fallback as a rule that fails to build,
		// rather than let a malformed .ics crash the UI (iron rule).
		return e.baseInstance(from, to), nil
	}
	var out []Occurrence
	for _, start := range starts {
		end := start.Add(dur)
		if overlaps(start, end, from, to) {
			out = append(out, Occurrence{Start: start, End: end, Event: e})
		}
	}
	return out, nil
}

const (
	// maxOccurrenceSteps bounds how many raw recurrence instances one event is
	// stepped through when expanding a window, counting those skipped before the
	// window as well as those collected. It stops a syntactically valid but
	// pathological rule — FREQ=SECONDLY with no COUNT/UNTIL, or a rule anchored
	// centuries before the query window — from iterating millions of times and
	// freezing the UI or exhausting memory. This is a scale limit that doubles as
	// a malformed-input safeguard; ~1M steps is far beyond any real calendar view.
	maxOccurrenceSteps = 1 << 20

	// maxOccurrencesPerEvent bounds how many in-window instances one event
	// contributes, so a single high-frequency event can't flood a view. Far above
	// any realistic count (a month of hourly instances is < 800).
	maxOccurrencesPerEvent = 10000

	// maxAggregateOccurrenceSteps bounds the TOTAL raw recurrence steps across a
	// batch of expansions sharing one StepBudget — every event in a resource, or
	// every event across all visible calendars on a single redraw. The per-event
	// cap (maxOccurrenceSteps) alone leaves the SUM unbounded: N far-anchored
	// FREQ=SECONDLY events each burn the full per-event budget, so a redraw scales
	// as N × per-event and freezes (measured: 50 such events ≈ 5s). This ceiling
	// caps the sum instead; it sits comfortably above the per-event cap so a lone
	// pathological event never starves legitimate siblings, yet far below what any
	// realistic calendar view needs (real rules step a handful of times to reach
	// the window), so it only ever bites hostile/degenerate input.
	maxAggregateOccurrenceSteps = 2 << 20
)

// StepBudget is a shared ceiling on raw recurrence-iteration steps across a batch
// of event expansions. Pass one budget to Parsed.EventOccurrencesBudgeted for
// every resource in a redraw so the aggregate cost stays bounded regardless of
// how many pathological events the cache holds; once it is spent, remaining
// events degrade to their base instance rather than iterating. A nil *StepBudget
// means "no shared cap" — each event still gets the per-event bound.
type StepBudget struct {
	remaining int
}

// NewStepBudget returns a StepBudget primed with the aggregate step ceiling.
func NewStepBudget() *StepBudget {
	return &StepBudget{remaining: maxAggregateOccurrenceSteps}
}

// safeBetween returns the recurrence-set instances in [from, to], bounded so a
// pathological rule can neither hang nor exhaust memory: iteration stops after
// maxSteps raw steps or maxOccurrencesPerEvent collected instances. It reports
// used, the number of steps actually consumed, so a shared StepBudget can be
// drawn down across events. It also contains any panic rrule-go raises on a
// degenerate rule (ok=false) so the caller can degrade instead of crashing.
// Vendored code must not be hand-edited, so both guards live here at the call
// boundary. Within the bounds the result is identical to set.Between(from, to, true).
func safeBetween(set *rrule.Set, from, to time.Time, maxSteps int) (starts []time.Time, used int, ok bool) {
	ok = true
	defer func() {
		if r := recover(); r != nil {
			starts, ok = nil, false
		}
	}()
	next := set.Iterator()
	for used = 0; used < maxSteps; used++ {
		v, valid := next()
		if !valid || v.After(to) {
			return
		}
		if !v.Before(from) {
			starts = append(starts, v)
			if len(starts) >= maxOccurrencesPerEvent {
				return
			}
		}
	}
	return
}

// safeAfter returns the first recurrence instant strictly after `after` (or at or
// after `after` when inc is true), with the same bound and panic guards as
// safeBetween — so a write-side caller (grab/complete/split of a recurring item)
// degrades instead of crashing on a degenerate rule. ok is false when rrule-go
// panics; a zero time with ok=true means the series has no such instant. Within
// the bounds the result matches set.After(after, inc).
func safeAfter(set *rrule.Set, after time.Time, inc bool) (t time.Time, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			t, ok = time.Time{}, false
		}
	}()
	next := set.Iterator()
	for steps := 0; steps < maxOccurrenceSteps; steps++ {
		v, valid := next()
		if !valid {
			return time.Time{}, true
		}
		if v.After(after) || (inc && v.Equal(after)) {
			return v, true
		}
	}
	return time.Time{}, true
}

// baseInstance returns the event's single un-recurred instance if it overlaps
// [from, to). It serves both the non-recurring path and the graceful fallback
// when a malformed recurrence rule can't be expanded.
func (e *Event) baseInstance(from, to time.Time) []Occurrence {
	dur := e.Duration()
	if overlaps(e.Start, e.Start.Add(dur), from, to) {
		return []Occurrence{{Start: e.Start, End: e.Start.Add(dur), Event: e}}
	}
	return nil
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
		applyDateOnlyUntilBound(e.Raw.Props, roption, loc)
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
		// resolveDateTimeValues (not prop.DateTime) so a Windows/Outlook TZID on
		// an RDATE recovers the same way DTSTART does, instead of erroring out and
		// blanking the whole event's expansion — and so a comma-listed
		// multi-valued RDATE contributes every value, not zero.
		dts, err := resolveDateTimeValues(&prop, loc)
		if err != nil {
			return nil, fmt.Errorf("event %q: parsing RDATE: %w", e.UID, err)
		}
		for _, dt := range dts {
			set.RDate(dt)
		}
	}
	for _, prop := range e.Raw.Props.Values(ical.PropExceptionDates) {
		dts, err := resolveDateTimeValues(&prop, loc)
		if err != nil {
			return nil, fmt.Errorf("event %q: parsing EXDATE: %w", e.UID, err)
		}
		for _, dt := range dts {
			set.ExDate(dt)
		}
	}
	return set, nil
}

// dateOnlyUntilValue returns an RRULE string's UNTIL value when it is a DATE
// (YYYYMMDD — the form RFC 5545 §3.3.10 requires against a VALUE=DATE anchor),
// and "" when the rule has no UNTIL or carries a DATE-TIME one.
func dateOnlyUntilValue(rule string) string {
	for _, part := range strings.Split(rule, ";") {
		if !strings.HasPrefix(part, "UNTIL=") {
			continue
		}
		if v := part[len("UNTIL="):]; len(v) == len(dateOnlyLayout) {
			return v
		}
		return ""
	}
	return ""
}

// applyDateOnlyUntilBound re-bounds a DATE-valued UNTIL at the END of that day in
// loc, in place on a freshly parsed ROption.
//
// UNTIL is inclusive (RFC 5545 §3.3.10): "ends on D" must still fire on D. A DATE
// value carries no zone — it names a calendar day — but rrule-go's StrToROption
// parses the bare YYYYMMDD as UTC midnight and its iterator compares that instant
// against occurrences generated at the anchor's own local midnight. In UTC and
// every zone west of it, D's occurrence therefore lands after the bound and is
// dropped. Anchoring the bound at the last second of D in the series' own zone
// restores the inclusive reading everywhere without touching the stored bytes:
// this is a read-side interpretation, so a server-authored UNTIL is never
// rewritten (iron rule).
func applyDateOnlyUntilBound(props ical.Props, roption *rrule.ROption, loc *time.Location) {
	if roption == nil || roption.Until.IsZero() {
		return
	}
	prop := props.Get(ical.PropRecurrenceRule)
	if prop == nil || dateOnlyUntilValue(prop.Value) == "" {
		return
	}
	if loc == nil {
		loc = time.Local
	}
	day := roption.Until.UTC() // rrule-go read the bare date as UTC midnight
	roption.Until = time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 59, 0, loc)
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
	return p.eventOccurrences(from, to, NewStepBudget())
}

// EventOccurrencesBudgeted is EventOccurrences with a caller-supplied StepBudget
// shared across resources, so a whole redraw (store.EventOccurrencesVisible over
// every visible calendar) is bounded in aggregate, not just per resource.
func (p *Parsed) EventOccurrencesBudgeted(from, to time.Time, budget *StepBudget) ([]Occurrence, error) {
	return p.eventOccurrences(from, to, budget)
}

func (p *Parsed) eventOccurrences(from, to time.Time, budget *StepBudget) ([]Occurrence, error) {
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
			// Skip a master that fails to expand rather than blanking every
			// sibling component in the file (iron rule: degrade gracefully).
			// Occurrences already degrades a bad rule to the base instance, so
			// an error here is unexpected — but guard anyway.
			occs, err := master.occurrences(from, to, budget)
			if err != nil {
				continue
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
