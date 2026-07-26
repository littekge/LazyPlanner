package model

import (
	"strings"
	"time"

	"github.com/emersion/go-ical"
)

// icalDateTimeLocal is the RFC 5545 "floating" date-time layout (no zone
// designator). It is used only on the recovery path below.
const icalDateTimeLocal = "20060102T150405"

// resolveDateTime parses an iCal date/date-time property into an absolute time,
// staying robust to time zones Go cannot load. It first defers to go-ical, which
// handles UTC, date-only, and IANA-TZID values. go-ical fails hard when a TZID
// is not an IANA zone (Outlook/Windows zone names like "Eastern Standard Time",
// or a custom VTIMEZONE label); rather than let that drop the whole item, this
// maps common Windows zone names to IANA and, failing that, interprets the value
// as floating time in loc. The item is thus never lost — at worst an unmapped
// exotic zone is off by its UTC offset until corrected.
func resolveDateTime(prop *ical.Prop, loc *time.Location) (time.Time, error) {
	if loc == nil {
		loc = time.Local
	}
	if t, err := prop.DateTime(loc); err == nil {
		return gapSafeReading(prop, t), nil
	}

	// go-ical failed. If there is no TZID, the value itself is malformed — there
	// is nothing to recover, so report the original failure.
	tzid := prop.Params.Get(ical.ParamTimezoneID)
	if tzid == "" {
		_, err := prop.DateTime(loc)
		return time.Time{}, err
	}

	if iana := windowsToIANA(tzid); iana != "" {
		if z, err := time.LoadLocation(iana); err == nil {
			if t, err := parseWallClockIn(icalDateTimeLocal, prop.Value, z); err == nil {
				return t, nil
			}
		}
	} else if z, err := time.LoadLocation(tzid); err == nil {
		// The TZID is itself a valid IANA zone that go-ical's DateTime rejected
		// for a reason other than the zone (e.g. a value-type param it can't
		// handle). Zone the wall-clock value directly rather than dropping it to
		// the floating fallback below — which would silently mis-zone it by the
		// TZID's UTC offset. Keeps the IANA path symmetric with the Windows one.
		if t, err := parseWallClockIn(icalDateTimeLocal, prop.Value, z); err == nil {
			return t, nil
		}
	}

	// Last resort: keep the item by treating the wall-clock value as floating.
	if t, err := parseWallClockIn(icalDateTimeLocal, prop.Value, loc); err == nil {
		return t, nil
	}

	_, err := prop.DateTime(loc)
	return time.Time{}, err
}

// gapSafeReading re-resolves a zone-dependent value that Go's parser normalized
// out of the calendar day it names.
//
// A DATE value names a day and midnight is its instant; a zone-less DATE-TIME
// names a wall clock. In the zones whose DST transition lands ON 00:00 that
// midnight does not exist, and time.ParseInLocation normalizes it *backwards*
// into the previous day — so an all-day item due 2026-03-08 in America/Havana
// reads as due 2026-03-07, one day before the date written in the file. The
// recurrence fix in wallclock.go makes the app *write* the right day; without
// this it would still display and act on the wrong one.
//
// The zone comes from t.Location() — whatever go-ical resolved — so go-ical's
// TZID-vs-loc choice is never duplicated here, only its RFC 5545 value-type
// reading, and only to recover the nominal wall clock the normalization lost. A
// UTC-suffixed value names an absolute instant with no wall clock at risk.
func gapSafeReading(prop *ical.Prop, t time.Time) time.Time {
	w, ok := nominalWallClock(prop.Value)
	if !ok {
		return t
	}
	return resolveWallClock(w, t.Location())
}

// nominalWallClock returns an iCal DATE or floating DATE-TIME value as the wall
// clock it names, stamped in UTC for resolveWallClock. ok is false for a
// UTC-suffixed value (already absolute) or any shape neither layout matches, so
// the caller keeps the reading it already has rather than guessing.
func nominalWallClock(value string) (time.Time, bool) {
	if strings.HasSuffix(value, "Z") {
		return time.Time{}, false
	}
	for _, layout := range []string{dateOnlyLayout, icalDateTimeLocal} {
		if len(value) != len(layout) {
			continue
		}
		if w, err := time.Parse(layout, value); err == nil {
			return w, true
		}
	}
	return time.Time{}, false
}

// parseWallClockIn parses a zone-less iCal value as a wall clock and resolves it
// in z gap-safely — the recovery-path twin of gapSafeReading.
func parseWallClockIn(layout, value string, z *time.Location) (time.Time, error) {
	w, err := time.Parse(layout, value)
	if err != nil {
		return time.Time{}, err
	}
	return resolveWallClock(w, z), nil
}

// resolveDateTimeValues resolves an RDATE/EXDATE property that may carry a
// comma-separated list of values on a single line (RFC 5545 permits this) into
// one absolute time per value. Without this, go-ical's single-value DateTime
// infers the value type from the whole line's length, so a multi-valued line
// matches no date/date-time layout and errors — collapsing the recurrence set to
// its base instance. A VALUE=PERIOD element ("start/end" or "start/duration")
// contributes its start instant. Each value inherits the property's TZID/VALUE
// params, so a Windows/Outlook TZID recovers the same way a single value does.
func resolveDateTimeValues(prop *ical.Prop, loc *time.Location) ([]time.Time, error) {
	parts := strings.Split(prop.Value, ",")
	out := make([]time.Time, 0, len(parts))
	for _, part := range parts {
		sub := *prop
		sub.Value = periodStart(part)
		// periodStart has reduced any PERIOD element to a plain date-time, so a
		// lingering VALUE=PERIOD param is now stale: go-ical's DateTime has no
		// period case and rejects the prop, which would drop an otherwise-valid
		// IANA-TZID value all the way to the floating fallback (mis-zoned by its
		// offset). Clone the params first — the shallow struct copy shares prop's
		// map — then drop the stale param so the reduced value re-parses cleanly.
		if sub.Params.Get(ical.ParamValue) == string(ical.ValuePeriod) {
			sub.Params = cloneParams(prop.Params)
			sub.Params.Del(ical.ParamValue)
		}
		t, err := resolveDateTime(&sub, loc)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// cloneParams returns a deep copy of an iCal params map so a per-value edit
// (dropping a stale VALUE param) can't mutate the shared original.
func cloneParams(p ical.Params) ical.Params {
	out := make(ical.Params, len(p))
	for k, v := range p {
		out[k] = append([]string(nil), v...)
	}
	return out
}

// periodStart returns the start instant of an RFC 5545 PERIOD value
// ("start/end" or "start/duration"); for a plain date-time value it returns the
// value unchanged. Only the start matters when expanding a recurrence set.
func periodStart(v string) string {
	if i := strings.IndexByte(v, '/'); i >= 0 {
		return v[:i]
	}
	return v
}
