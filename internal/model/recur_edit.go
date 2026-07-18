package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/teambition/rrule-go"
)

// Recurrence-editing primitives implementing the three scopes — this occurrence,
// this-and-future, and all — for VEVENTs and VTODOs. "All" is just the existing
// EditEvent/EditTodo on the master; the functions here handle the other two.
//
// The scopes map to iCalendar as follows:
//   - this occurrence  → a RECURRENCE-ID override component (events), or a
//     detached standalone task + advance (todos, orchestrated by the UI)
//   - this and future  → cap the master's RRULE with UNTIL and spawn a new series
//   - all              → edit the master component (EditEvent/EditTodo)
//
// Every function clones its input object first (never mutating the store snapshot)
// and re-parses so the typed fields match the edited raw components.

// masterComponent returns the VEVENT/VTODO with uid that is NOT a RECURRENCE-ID
// override (the series master), or nil.
func masterComponent(cal *ical.Calendar, uid string) *ical.Component {
	for _, c := range cal.Children {
		if c.Name != ical.CompEvent && c.Name != ical.CompToDo {
			continue
		}
		if text(c.Props, ical.PropUID) == uid && c.Props.Get(ical.PropRecurrenceID) == nil {
			return c
		}
	}
	return nil
}

// componentAnchor returns the component's recurrence anchor property name and its
// parsed time: DTSTART when present, else DUE (a VTODO may recur on its due date
// with no DTSTART). ok is false when neither is present.
func componentAnchor(comp *ical.Component, loc *time.Location) (name string, t time.Time, allDay, ok bool) {
	for _, n := range []string{ical.PropDateTimeStart, ical.PropDue} {
		if prop := comp.Props.Get(n); prop != nil {
			if v, err := resolveDateTime(prop, loc); err == nil {
				return n, v, isDateOnly(prop), true
			}
		}
	}
	return "", time.Time{}, false, false
}

// componentRecurrenceSet builds the rrule.Set for comp anchored at the given
// instant — the write-side twin of Event.recurrenceSet, usable for a VTODO too.
func componentRecurrenceSet(comp *ical.Component, anchor time.Time) (*rrule.Set, error) {
	set := &rrule.Set{}
	set.DTStart(anchor)

	roption, err := comp.Props.RecurrenceRule()
	if err != nil {
		return nil, fmt.Errorf("parsing RRULE: %w", err)
	}
	if roption != nil {
		roption.Dtstart = anchor
		rule, err := rrule.NewRRule(*roption)
		if err != nil {
			return nil, fmt.Errorf("building recurrence: %w", err)
		}
		set.RRule(rule)
	} else {
		set.RDate(anchor)
	}
	loc := anchor.Location()
	for _, prop := range comp.Props.Values(ical.PropRecurrenceDates) {
		if dt, err := resolveDateTime(&prop, loc); err == nil {
			set.RDate(dt)
		}
	}
	for _, prop := range comp.Props.Values(ical.PropExceptionDates) {
		if dt, err := resolveDateTime(&prop, loc); err == nil {
			set.ExDate(dt)
		}
	}
	return set, nil
}

// nextInstantAfter returns the recurrence instant strictly after `after`, or the
// zero time when the series has no further occurrence (COUNT/UNTIL exhausted).
func nextInstantAfter(comp *ical.Component, anchor, after time.Time) (time.Time, error) {
	set, err := componentRecurrenceSet(comp, anchor)
	if err != nil {
		return time.Time{}, err
	}
	// safeAfter (not set.After) so a degenerate rule — e.g. a near-zero anchor
	// year that panics rrule-go's calcDaySet — degrades to "no next occurrence"
	// instead of crashing the app on complete/grab/split (iron rule). The read
	// path is already guarded the same way via safeBetween.
	t, ok := safeAfter(set, after, false)
	if !ok {
		return time.Time{}, nil
	}
	return t, nil
}

// sameInstant compares two recurrence instants at second resolution (the
// granularity iCal stores), so wall-clock/UTC representations of the same slot
// match regardless of location.
func sameInstant(a, b time.Time) bool { return a.Unix() == b.Unix() }

// cloneProp copies an iCal property, deep-copying its parameter map so the copy
// never aliases the source's params.
func cloneProp(p ical.Prop) ical.Prop {
	np := p // copy the struct
	if p.Params != nil {
		np.Params = make(ical.Params, len(p.Params))
		for k, v := range p.Params {
			np.Params[k] = v
		}
	}
	return np
}

// deepCopyComponent returns a full deep copy of comp — every property (with a
// copied param map) and, recursively, every child component. Used when a
// recurrence primitive carries a component forward (a VALARM, or a whole
// RECURRENCE-ID override) so nothing the app doesn't model is lost (iron rule).
func deepCopyComponent(comp *ical.Component) *ical.Component {
	c := ical.NewComponent(comp.Name)
	for name, props := range comp.Props {
		cp := make([]ical.Prop, 0, len(props))
		for _, p := range props {
			cp = append(cp, cloneProp(p))
		}
		c.Props[name] = cp
	}
	c.Children = cloneChildren(comp)
	return c
}

// cloneChildren deep-copies a component's child components (VALARM, and any other
// nested component the app doesn't model). A recurrence primitive that hand-builds
// a new component from a master must carry these over rather than silently
// dropping them — the iron rule (never lose data the app doesn't understand).
func cloneChildren(src *ical.Component) []*ical.Component {
	if len(src.Children) == 0 {
		return nil
	}
	out := make([]*ical.Component, 0, len(src.Children))
	for _, child := range src.Children {
		out = append(out, deepCopyComponent(child))
	}
	return out
}

// cloneOverrideComponent deep-copies a master component into a new component of the
// same kind, dropping the series-level recurrence properties (RRULE/RDATE/EXDATE)
// so the copy describes a single instance. Params maps and child components
// (VALARM) are copied too, so the override neither aliases the master's props nor
// loses its reminders (iron rule).
func cloneOverrideComponent(master *ical.Component) *ical.Component {
	c := ical.NewComponent(master.Name)
	for name, props := range master.Props {
		switch name {
		case ical.PropRecurrenceRule, ical.PropRecurrenceDates, ical.PropExceptionDates, ical.PropRecurrenceID:
			continue
		}
		cp := make([]ical.Prop, 0, len(props))
		for _, p := range props {
			cp = append(cp, cloneProp(p))
		}
		c.Props[name] = cp
	}
	c.Children = cloneChildren(master)
	return c
}

// FindOverride returns the RECURRENCE-ID override event for uid at instant rid, or
// nil when none exists (the instance still comes from the master). Used by grab to
// read a single occurrence's current position across nudges.
func (p *Parsed) FindOverride(uid string, rid time.Time) *Event {
	for _, ev := range p.Events {
		if ev.UID != uid {
			continue
		}
		if t, ok := recurrenceID(ev); ok && sameInstant(t, rid) {
			return ev
		}
	}
	return nil
}

// AddOccurrenceOverride implements "edit this occurrence" for a recurring event:
// it adds (or updates, if one already exists) a RECURRENCE-ID override for the
// instance at occ, applies mutate (the field edits), and leaves the master series
// intact. The override shares the master's UID and lives in the same object.
func AddOccurrenceOverride(obj *Parsed, uid string, occ time.Time, allDay bool, mutate func(*ical.Component), now time.Time, loc *time.Location) (*Parsed, error) {
	if loc == nil {
		loc = time.Local
	}
	clone, err := obj.clone(loc)
	if err != nil {
		return nil, err
	}
	master := masterComponent(clone.Calendar, uid)
	if master == nil {
		return nil, fmt.Errorf("model: no recurring master with UID %q", uid)
	}

	var override *ical.Component
	for _, c := range clone.Calendar.Children {
		if text(c.Props, ical.PropUID) != uid {
			continue
		}
		if rid := c.Props.Get(ical.PropRecurrenceID); rid != nil {
			if t, err := resolveDateTime(rid, loc); err == nil && sameInstant(t, occ) {
				override = c
				break
			}
		}
	}
	if override == nil {
		override = cloneOverrideComponent(master)
		setDateOrTime(override, ical.PropRecurrenceID, occ, allDay)
		clone.Calendar.Children = append(clone.Calendar.Children, override)
	}
	mutate(override)
	return Parse(clone.Calendar, loc)
}

// AddException implements "delete this occurrence" for a recurring event: it adds
// an EXDATE for occ to the master (suppressing that instance) and removes any
// RECURRENCE-ID override that targeted it.
func AddException(obj *Parsed, uid string, occ time.Time, allDay bool, now time.Time, loc *time.Location) (*Parsed, error) {
	if loc == nil {
		loc = time.Local
	}
	clone, err := obj.clone(loc)
	if err != nil {
		return nil, err
	}
	master := masterComponent(clone.Calendar, uid)
	if master == nil {
		return nil, fmt.Errorf("model: no recurring master with UID %q", uid)
	}

	ex := newDateOrTimeProp(ical.PropExceptionDates, occ, allDay)
	master.Props[ical.PropExceptionDates] = append(master.Props[ical.PropExceptionDates], *ex)
	touch(master, now)

	kept := clone.Calendar.Children[:0]
	for _, c := range clone.Calendar.Children {
		if text(c.Props, ical.PropUID) == uid {
			if rid := c.Props.Get(ical.PropRecurrenceID); rid != nil {
				if t, err := resolveDateTime(rid, loc); err == nil && sameInstant(t, occ) {
					continue // drop the override for the deleted instance
				}
			}
		}
		kept = append(kept, c)
	}
	clone.Calendar.Children = kept
	return Parse(clone.Calendar, loc)
}

// dateOnlyUntil rewrites an RRULE string's UNTIL value from a DATE-TIME
// (YYYYMMDDThhmmssZ) to a DATE (YYYYMMDD) — used when capping an all-day series,
// where RFC 5545 requires UNTIL to match the date-only DTSTART value type.
func dateOnlyUntil(rule string) string {
	parts := strings.Split(rule, ";")
	for i, part := range parts {
		if !strings.HasPrefix(part, "UNTIL=") {
			continue
		}
		val := part[len("UNTIL="):]
		if idx := strings.IndexByte(val, 'T'); idx == 8 { // YYYYMMDD then 'T'
			parts[i] = "UNTIL=" + val[:idx]
		}
	}
	return strings.Join(parts, ";")
}

// filterRDates rewrites comp's RDATE properties to keep only the values whose
// resolved instant satisfies keep — used to partition a series' RDATEs across a
// this-and-future split so a one-off RDATE lands in exactly one half, not both.
// RDATE lines may be comma-multi-valued (RFC 5545); each value is judged
// individually and the original value text is preserved so a VALUE=PERIOD element
// round-trips unchanged. A value that cannot be resolved is kept — never silently
// drop data we can't interpret (iron rule).
func filterRDates(comp *ical.Component, keep func(time.Time) bool, loc *time.Location) {
	props := comp.Props[ical.PropRecurrenceDates]
	if len(props) == 0 {
		return
	}
	var out []ical.Prop
	for _, p := range props {
		var keptVals []string
		for _, v := range strings.Split(p.Value, ",") {
			probe := cloneProp(p)
			probe.Value = periodStart(v)
			if t, err := resolveDateTime(&probe, loc); err != nil || keep(t) {
				keptVals = append(keptVals, v)
			}
		}
		if len(keptVals) > 0 {
			np := cloneProp(p)
			np.Value = strings.Join(keptVals, ",")
			out = append(out, np)
		}
	}
	if len(out) == 0 {
		delete(comp.Props, ical.PropRecurrenceDates)
	} else {
		comp.Props[ical.PropRecurrenceDates] = out
	}
}

// CapSeries caps the master's recurrence at `until` (inclusive), so the series
// ends there — the master half of a this-and-future split, and the whole of a
// this-and-future delete. Any COUNT is dropped (UNTIL replaces it) and overrides
// after `until` are removed.
func CapSeries(obj *Parsed, uid string, until time.Time, now time.Time, loc *time.Location) (*Parsed, error) {
	if loc == nil {
		loc = time.Local
	}
	clone, err := obj.clone(loc)
	if err != nil {
		return nil, err
	}
	master := masterComponent(clone.Calendar, uid)
	if master == nil {
		return nil, fmt.Errorf("model: no recurring master with UID %q", uid)
	}
	roption, err := master.Props.RecurrenceRule()
	if err != nil || roption == nil {
		return nil, fmt.Errorf("model: %q has no RRULE to cap", uid)
	}
	roption.Until = until.UTC().Truncate(time.Second)
	roption.Count = 0 // UNTIL and COUNT are mutually exclusive
	master.Props.SetRecurrenceRule(roption)
	// rrule-go always renders UNTIL as a DATE-TIME, but RFC 5545 §3.3.10 requires
	// UNTIL's value type to match DTSTART — so an all-day (VALUE=DATE) master needs
	// a DATE UNTIL, else a strict server or another client may reject the object.
	if dtstart := master.Props.Get(ical.PropDateTimeStart); dtstart != nil && isDateOnly(dtstart) {
		if rp := master.Props.Get(ical.PropRecurrenceRule); rp != nil {
			rp.Value = dateOnlyUntil(rp.Value)
		}
	}
	// UNTIL bounds only the RRULE generator, not RDATEs (rrule-go's Set.Iterator
	// merges RDATEs independent of UNTIL), so a trailing RDATE after the cut would
	// still be emitted by the capped past half — and again by the future series,
	// which keeps it (NewSeriesFrom) — a duplicated occurrence and the iron-rule
	// hazard of one unmodeled property becoming two live instances. Keep only
	// RDATEs at or before the cut.
	filterRDates(master, func(t time.Time) bool { return !t.After(until) }, loc)
	touch(master, now)

	kept := clone.Calendar.Children[:0]
	for _, c := range clone.Calendar.Children {
		if text(c.Props, ical.PropUID) == uid {
			if rid := c.Props.Get(ical.PropRecurrenceID); rid != nil {
				if t, err := resolveDateTime(rid, loc); err == nil && t.After(until) {
					continue
				}
			}
		}
		kept = append(kept, c)
	}
	clone.Calendar.Children = kept
	return Parse(clone.Calendar, loc)
}

// NewSeriesFrom builds a fresh single-component object (a new resource) cloned
// from the master of uid in obj, re-keyed to a new UID, with mutate applied — the
// future half of a this-and-future split at occ. It keeps the master's RRULE,
// preserving both bounds exactly: an absolute UNTIL is unchanged (the series' end
// date doesn't move when it's split), and a COUNT is reduced by the number of
// occurrences that stay with the capped master (those before occ), so a
// COUNT-bounded series splits into two halves that sum to the original count.
func NewSeriesFrom(obj *Parsed, uid string, occ time.Time, mutate func(*ical.Component), now time.Time, loc *time.Location) (*Parsed, error) {
	if loc == nil {
		loc = time.Local
	}
	src, err := obj.clone(loc)
	if err != nil {
		return nil, err
	}
	master := masterComponent(src.Calendar, uid)
	if master == nil {
		return nil, fmt.Errorf("model: no recurring master with UID %q", uid)
	}
	pastCount := rruleIterationsBefore(master, occ, loc)

	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropVersion, icalVersion)
	cal.Props.SetText(ical.PropProductID, ProductID)

	comp := ical.NewComponent(master.Name)
	for name, props := range master.Props {
		if name == ical.PropRecurrenceID {
			continue
		}
		cp := make([]ical.Prop, 0, len(props))
		for _, p := range props {
			cp = append(cp, cloneProp(p))
		}
		comp.Props[name] = cp
	}
	// Carry the master's child components (VALARM) into the future series, so a
	// this-and-future split doesn't strip reminders from every future occurrence
	// (iron rule).
	comp.Children = cloneChildren(master)
	// Keep only RDATEs at or after the split point: a pre-split RDATE belongs to
	// the capped past half, and copying it here would make the future series emit
	// an extra instant before its own start. Paired with CapSeries dropping
	// post-cut RDATEs, this partitions each RDATE into exactly one half.
	filterRDates(comp, func(t time.Time) bool { return !t.Before(occ) }, loc)
	newUID := NewUID()
	comp.Props.SetText(ical.PropUID, newUID)
	// Preserve the rule's bound: keep an absolute UNTIL as-is, and reduce a COUNT by
	// the occurrences that remain with the capped master (before occ), so the two
	// halves together still yield the original number of occurrences.
	if roption, err := comp.Props.RecurrenceRule(); err == nil && roption != nil {
		if roption.Count > 0 {
			remaining := roption.Count - pastCount
			if remaining < 1 {
				remaining = 1
			}
			roption.Count = remaining
		}
		comp.Props.SetRecurrenceRule(roption)
	}
	setDateTimeUTC(comp, ical.PropCreated, now)
	cal.Children = append(cal.Children, comp)

	// Carry forward any RECURRENCE-ID override strictly after the split point,
	// re-keyed to the new series' UID. CapSeries drops these from the past half and
	// the new series is built from the master alone, so without this a customized
	// future occurrence would be lost from both halves (iron rule). The occurrence
	// at occ itself is redefined by mutate below, so its old override (if any) is
	// intentionally not carried.
	for _, c := range src.Calendar.Children {
		if text(c.Props, ical.PropUID) != uid {
			continue
		}
		rid := c.Props.Get(ical.PropRecurrenceID)
		if rid == nil {
			continue
		}
		if t, err := resolveDateTime(rid, loc); err != nil || t.Unix() <= occ.Unix() {
			continue
		}
		ov := deepCopyComponent(c)
		ov.Props.SetText(ical.PropUID, newUID)
		cal.Children = append(cal.Children, ov)
	}

	mutate(comp)
	return Parse(cal, loc)
}

// rruleIterationsBefore counts the master's RRULE iterations strictly before occ
// — the COUNT units consumed by the capped past half of a split, used to reduce
// the future half's COUNT. It builds a set from the RRULE alone, excluding EXDATE
// and RDATE deliberately: RFC 5545 COUNT bounds the RRULE generator, so an
// EXDATE'd instance still consumes COUNT (counting the EXDATE-filtered visible
// set instead would undercount the past half and leave the future COUNT one too
// high, appending a phantom trailing occurrence), and an RDATE is an explicit
// extra instant that COUNT does not bound. Returns 0 when the component isn't
// dated or has no rule.
func rruleIterationsBefore(master *ical.Component, occ time.Time, loc *time.Location) int {
	_, anchor, _, ok := componentAnchor(master, loc)
	if !ok {
		return 0
	}
	roption, err := master.Props.RecurrenceRule()
	if err != nil || roption == nil {
		return 0
	}
	roption.Dtstart = anchor
	rule, err := rrule.NewRRule(*roption)
	if err != nil {
		return 0
	}
	set := &rrule.Set{}
	set.DTStart(anchor)
	set.RRule(rule)
	// safeBetween (not set.Between) so a degenerate rule degrades to 0 rather than
	// panicking during a this-and-future split (iron rule).
	starts, ok := safeBetween(set, anchor, occ)
	if !ok {
		return 0
	}
	n := 0
	for _, t := range starts {
		if t.Before(occ) {
			n++
		}
	}
	return n
}

// EditEventOccurrence is the draft-based "edit this occurrence" for an event: it
// overrides just the instance at occ with d (leaving the series intact).
func EditEventOccurrence(obj *Parsed, uid string, occ time.Time, allDay bool, d EventDraft, now time.Time, loc *time.Location) (*Parsed, error) {
	return AddOccurrenceOverride(obj, uid, occ, allDay, func(c *ical.Component) {
		applyEvent(c, d, now)
	}, now, loc)
}

// SplitEvent is the draft-based "edit this and future" for an event: it caps the
// master just before occ and returns both the capped master (same resource) and a
// new-UID future series carrying d (a new resource). The caller writes both.
func SplitEvent(obj *Parsed, uid string, occ time.Time, d EventDraft, now time.Time, loc *time.Location) (capped, future *Parsed, err error) {
	capped, err = CapSeries(obj, uid, occ.Add(-time.Second), now, loc)
	if err != nil {
		return nil, nil, err
	}
	future, err = NewSeriesFrom(obj, uid, occ, func(c *ical.Component) {
		applyEvent(c, d, now)
	}, now, loc)
	if err != nil {
		return nil, nil, err
	}
	return capped, future, nil
}

// AdvanceRecurringTodo rolls a recurring todo forward to its next occurrence,
// implementing "complete one occurrence" the NextCloud/RFC way: DTSTART and DUE
// move to the next instant (keeping their offset), COUNT decrements, and when the
// series is exhausted the todo is marked completed instead. done reports whether
// the whole series was completed (no further occurrence).
// A recurring todo shows a single live instance and advances one occurrence on
// completion (NextCloud-style); when the series is exhausted it is marked done
// instead. done reports whether the whole series completed.
func AdvanceRecurringTodo(obj *Parsed, uid string, now time.Time, loc *time.Location) (result *Parsed, done bool, err error) {
	if loc == nil {
		loc = time.Local
	}
	clone, err := obj.clone(loc)
	if err != nil {
		return nil, false, err
	}
	comp := findComponent(clone.Calendar, uid)
	if comp == nil {
		return nil, false, fmt.Errorf("model: no todo with UID %q", uid)
	}
	anchorName, anchor, allDay, ok := componentAnchor(comp, loc)
	if !ok {
		return nil, false, fmt.Errorf("model: todo %q has no DTSTART/DUE to advance", uid)
	}

	roption, _ := comp.Props.RecurrenceRule()
	// Ask the full recurrence set (RRULE + RDATE - EXDATE) for the next instant
	// rather than short-circuiting on COUNT==1: a COUNT=1 rule that also carries an
	// RDATE still has a further occurrence, and marking it done on COUNT alone would
	// complete it one occurrence early.
	next, err := nextInstantAfter(comp, anchor, anchor)
	if err != nil {
		return nil, false, err
	}
	if exhausted := next.IsZero(); exhausted {
		setCompleted(comp, true, now)
		touch(comp, now)
		out, perr := Parse(clone.Calendar, loc)
		return out, true, perr
	}

	// Roll the anchor (and the paired DTSTART/DUE, preserving their offset) forward.
	delta := next.Sub(anchor)
	rollProp := func(name string) {
		if prop := comp.Props.Get(name); prop != nil {
			if t, err := resolveDateTime(prop, loc); err == nil {
				setDateOrTime(comp, name, t.Add(delta), isDateOnly(prop))
			}
		}
	}
	rollProp(ical.PropDateTimeStart)
	rollProp(ical.PropDue)
	if comp.Props.Get(ical.PropDateTimeStart) == nil && comp.Props.Get(ical.PropDue) == nil {
		// Neither paired prop existed except the anchor we found; set it directly.
		setDateOrTime(comp, anchorName, next, allDay)
	}
	if roption != nil && roption.Count > 1 {
		roption.Count--
		comp.Props.SetRecurrenceRule(roption)
	}
	// A rolled-forward instance is freshly not-done.
	setCompleted(comp, false, now)
	touch(comp, now)
	out, perr := Parse(clone.Calendar, loc)
	return out, false, perr
}

// DetachTodoOccurrence builds a standalone one-off task from the current instance
// of the recurring todo carrying uid: it clones the original component so every
// unmodeled property is preserved (VALARM, ATTACH, URL, X-, non-PARENT
// RELATED-TO — the iron rule), strips the recurrence properties (RRULE / RDATE /
// EXDATE / RECURRENCE-ID) so it is a plain task, assigns a fresh UID, and applies
// the edited fields d. It returns the new standalone object and its UID; obj and
// the series are untouched (the caller advances the series separately). This is
// the todo analogue of the event override's clone-and-mutate — building from
// NewTodoObject instead would drop everything the form doesn't model.
func DetachTodoOccurrence(obj *Parsed, uid string, d TodoDraft, now time.Time, loc *time.Location) (*Parsed, string, error) {
	if loc == nil {
		loc = time.Local
	}
	src, err := obj.clone(loc)
	if err != nil {
		return nil, "", err
	}
	master := findComponent(src.Calendar, uid)
	if master == nil {
		return nil, "", fmt.Errorf("model: no todo with UID %q", uid)
	}

	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropVersion, icalVersion)
	cal.Props.SetText(ical.PropProductID, ProductID)

	one := ical.NewComponent(master.Name)
	for name, props := range master.Props {
		switch name {
		case ical.PropRecurrenceRule, ical.PropRecurrenceDates, ical.PropExceptionDates, ical.PropRecurrenceID:
			continue
		}
		cp := make([]ical.Prop, 0, len(props))
		for _, p := range props {
			cp = append(cp, cloneProp(p))
		}
		one.Props[name] = cp
	}
	one.Children = cloneChildren(master)
	newUID := NewUID()
	one.Props.SetText(ical.PropUID, newUID)
	setDateTimeUTC(one, ical.PropCreated, now)
	setCompleted(one, d.Completed, now)
	applyTodo(one, d, now)
	cal.Children = append(cal.Children, one)

	out, err := Parse(cal, loc)
	if err != nil {
		return nil, "", err
	}
	return out, newUID, nil
}
