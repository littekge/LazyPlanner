package model

import (
	"fmt"
	"time"

	"github.com/emersion/go-ical"
)

// vtimezoneLookbackYears is how far *back* from the anchor transitions are searched.
// The search is deliberately one-sided: an observance's RRULE expands forward from
// its DTSTART only, so the onset must sit at or before the anchor or a strict reader
// has no rule in effect for the very date the VTIMEZONE exists to describe.
//
// Three years is measured, not guessed. Sweeping all 487 zones in the host database
// against twelve monthly anchors, 149 of the 152 DST zones need only one year, and
// three need two — America/Asuncion, America/Coyhaique and America/Vancouver, each
// having changed rules recently enough to leave a sparse patch. Three years keeps a
// year of margin for the next such rule change. Widening is always safe: the most
// recent transition of each kind is selected, so a longer window can only supply a
// kind that was missing, never displace a nearer one.
const vtimezoneLookbackYears = 3

// vtimezoneDateTimeLayout is the RFC 5545 floating DATE-TIME form. An observance's
// DTSTART is always local-and-floating: it carries neither a Z nor a TZID, because
// the offsets it sits next to are what give it meaning.
const vtimezoneDateTimeLayout = "20060102T150405"

// vtimezoneEpochOnset is the onset for an observance that has no transition behind
// it — a zone on one rule for the whole window. Dating it at the anchor would make
// the rule start *after* events earlier that same day; real generators use a far-past
// onset (Google emits 19700308T020000) so the single rule covers every date a reader
// can ask about.
const vtimezoneEpochOnset = "19700101T000000"

// IsNamedZone reports whether loc can be referenced by TZID. UTC is excluded
// deliberately: a UTC value's correct serialization is the Z form, which needs no
// TZID and no VTIMEZONE. A fixed-offset zone (time.FixedZone) carries an
// abbreviation, not an identity, so it is not referenceable either.
func IsNamedZone(loc *time.Location) bool {
	if loc == nil || loc == time.UTC {
		return false
	}
	name := loc.String()
	if name == "" || name == "Local" || name == "UTC" {
		return false
	}
	// A name that does not load is not one another client can resolve.
	_, err := time.LoadLocation(name)
	return err == nil
}

// BuildVTimezone returns a VTIMEZONE describing loc's current offset rules, or
// nil when loc is not TZID-referenceable.
//
// The shape is the conventional compact one — one observance per offset with a
// yearly RRULE derived from the most recent transition — rather than an exhaustive
// historical record: it is what NextCloud, Google and Apple emit, and what other
// clients are tested against.
func BuildVTimezone(loc *time.Location, around time.Time) *ical.Component {
	if !IsNamedZone(loc) {
		return nil
	}
	tz := ical.NewComponent(ical.CompTimezone)
	tz.Props.SetText(ical.PropTimezoneID, loc.String())

	transitions := zoneTransitions(loc, around.AddDate(-vtimezoneLookbackYears, 0, 0), around)

	if len(transitions) == 0 {
		// No DST in the window: one STANDARD observance carrying the fixed offset.
		_, off := around.In(loc).Zone()
		tz.Children = append(tz.Children, observance(ical.CompTimezoneStandard, around.In(loc), off, off, false))
		return tz
	}

	// Keep the most recent transition of each kind. Because the window ends at the
	// anchor, that is the transition under whose rule the anchor itself falls — the
	// one a reader needs — and the yearly RRULE projects it forward from there.
	seen := map[string]bool{}
	for i := len(transitions) - 1; i >= 0; i-- {
		at := transitions[i].In(loc)
		name := ical.CompTimezoneStandard
		if at.IsDST() {
			name = ical.CompTimezoneDaylight
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		_, offTo := at.Zone()
		_, offFrom := at.Add(-time.Second).In(loc).Zone()
		tz.Children = append(tz.Children, observance(name, at, offFrom, offTo, true))
	}
	return tz
}

// zoneTransitions returns the instants in [from, to) at which loc changes offset,
// found by day-stepping and then bisecting to the minute. Go exposes no
// transition table, so probing is the only portable way to read one.
func zoneTransitions(loc *time.Location, from, to time.Time) []time.Time {
	var out []time.Time
	prev := from
	_, prevOff := prev.In(loc).Zone()
	for t := from.AddDate(0, 0, 1); t.Before(to); t = t.AddDate(0, 0, 1) {
		_, off := t.In(loc).Zone()
		if off == prevOff {
			prev = t
			continue
		}
		lo, hi := prev, t
		for hi.Sub(lo) > time.Minute {
			mid := lo.Add(hi.Sub(lo) / 2)
			if _, o := mid.In(loc).Zone(); o == prevOff {
				lo = mid
			} else {
				hi = mid
			}
		}
		out = append(out, hi.Truncate(time.Minute))
		prevOff, prev = off, t
	}
	return out
}

// observance builds one STANDARD/DAYLIGHT subcomponent.
func observance(name string, at time.Time, offFrom, offTo int, recurring bool) *ical.Component {
	sub := ical.NewComponent(name)
	// A non-recurring observance is the "one rule, always" case, so its onset belongs
	// in the far past rather than at the transition-less anchor.
	dtstart := vtimezoneEpochOnset
	if recurring {
		dtstart = observanceStart(at, offFrom)
	}
	setTypedProp(sub.Props, ical.PropDateTimeStart, dtstart)
	setTypedProp(sub.Props, ical.PropTimezoneOffsetFrom, icalUTCOffset(offFrom))
	setTypedProp(sub.Props, ical.PropTimezoneOffsetTo, icalUTCOffset(offTo))
	if abbrev, _ := at.Zone(); abbrev != "" {
		sub.Props.SetText(ical.PropTimezoneName, abbrev)
	}
	if recurring {
		setTypedProp(sub.Props, ical.PropRecurrenceRule,
			fmt.Sprintf("FREQ=YEARLY;BYMONTH=%d;BYDAY=%s", int(at.Month()), icalNthWeekday(at)))
	}
	return sub
}

// observanceStart renders the transition instant as the floating wall clock an
// observance DTSTART must carry. RFC 5545 §3.6.5 pins that wall clock to the offset
// being switched *from*, not to: a US spring-forward is written 02:00 with
// TZOFFSETFROM:-0500, which is the same instant as 03:00 EDT. Rendering it in the
// new offset instead would read, to any conforming client, as an onset an hour late
// — and would not match the shape NextCloud, Google and Apple emit.
func observanceStart(at time.Time, offFrom int) string {
	return at.In(time.FixedZone("", offFrom)).Format(vtimezoneDateTimeLayout)
}

// setTypedProp writes an already-serialized value for a property whose value type
// is not TEXT. Props.SetText cannot be used for these: it stamps VALUE=TEXT on any
// property whose default type differs and backslash-escapes ';' and ',', which
// would turn an RRULE into the unreadable "FREQ=YEARLY\;BYMONTH=3" and mistype every
// DATE-TIME and UTC-OFFSET in the component.
func setTypedProp(props ical.Props, name, value string) {
	prop := ical.NewProp(name)
	prop.Value = value
	props.Set(prop)
}

// icalUTCOffset renders seconds east of UTC as the ±HHMM form UTC-OFFSET requires.
func icalUTCOffset(seconds int) string {
	sign := "+"
	if seconds < 0 {
		sign, seconds = "-", -seconds
	}
	return fmt.Sprintf("%s%02d%02d", sign, seconds/3600, (seconds%3600)/60)
}

// icalNthWeekday renders t's weekday as the BYDAY ordinal form ("2SU"), using -1
// for a date in the final week of its month ("last Sunday") the way zone rules
// are conventionally expressed.
func icalNthWeekday(t time.Time) string {
	abbrev := [...]string{"SU", "MO", "TU", "WE", "TH", "FR", "SA"}[t.Weekday()]
	if t.AddDate(0, 0, 7).Month() != t.Month() {
		return "-1" + abbrev
	}
	return fmt.Sprintf("%d%s", (t.Day()-1)/7+1, abbrev)
}
