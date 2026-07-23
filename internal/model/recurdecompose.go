package model

import (
	"time"

	"github.com/teambition/rrule-go"
)

// RecurSpecFromRule decomposes a parsed RRULE anchored at `anchor` (the
// component's DTSTART/DUE) into a RecurSpec, reporting ok=false for any rule
// outside the editable vocabulary or one that contradicts its own anchor. A
// rejected rule stays "Custom (kept)" and is preserved byte-for-byte rather than
// re-serialized from a lossy spec.
//
// It is deliberately conservative: BYSETPOS, sub-daily frequencies, BYYEARDAY /
// BYWEEKNO / BYHOUR / BYMINUTE / BYSECOND / BYEASTER, a non-Monday WKST, an
// nth-ordinal on a weekly rule, a monthly rule whose BYMONTHDAY or nth-weekday
// disagrees with the anchor, and a yearly rule whose BYMONTH/BYMONTHDAY disagrees
// with the anchor all return ok=false. Monthly by day-of-month and yearly carry
// no stored day/month — the anchor supplies them (matching ROption, which emits
// a bare FREQ for those).
func RecurSpecFromRule(option *rrule.ROption, anchor time.Time) (RecurSpec, bool) {
	if option == nil {
		return RecurSpec{}, false
	}
	// BY* dimensions the vocabulary can't express, and a non-default week start.
	if len(option.Bysetpos) > 0 || len(option.Byyearday) > 0 || len(option.Byweekno) > 0 ||
		len(option.Byhour) > 0 || len(option.Byminute) > 0 || len(option.Bysecond) > 0 ||
		len(option.Byeaster) > 0 {
		return RecurSpec{}, false
	}
	if option.Wkst != rrule.MO {
		return RecurSpec{}, false
	}

	var spec RecurSpec
	switch option.Freq {
	case rrule.DAILY:
		spec.Freq = FreqDaily
	case rrule.WEEKLY:
		spec.Freq = FreqWeekly
	case rrule.MONTHLY:
		spec.Freq = FreqMonthly
	case rrule.YEARLY:
		spec.Freq = FreqYearly
	default: // HOURLY / MINUTELY / SECONDLY
		return RecurSpec{}, false
	}
	if option.Interval > 1 {
		spec.Interval = option.Interval
	}
	// End condition — at most one. rrule's parser sets both fields independently, so
	// a malformed COUNT+UNTIL rule is possible; the spec models only one.
	if option.Count > 0 {
		spec.Count = option.Count
	}
	if !option.Until.IsZero() {
		u := option.Until.UTC().Truncate(time.Second)
		spec.Until = &u
	}
	if spec.Count > 0 && spec.Until != nil {
		return RecurSpec{}, false
	}

	switch spec.Freq {
	case FreqDaily:
		if len(option.Byweekday) > 0 || len(option.Bymonthday) > 0 || len(option.Bymonth) > 0 {
			return RecurSpec{}, false
		}
	case FreqWeekly:
		if len(option.Bymonthday) > 0 || len(option.Bymonth) > 0 {
			return RecurSpec{}, false
		}
		for _, w := range option.Byweekday {
			if w.N() != 0 { // an nth-ordinal weekday is not a plain weekly set
				return RecurSpec{}, false
			}
			spec.Weekdays = append(spec.Weekdays, rruleToWeekday(w))
		}
	case FreqMonthly:
		return decodeMonthly(option, anchor, spec)
	case FreqYearly:
		return decodeYearly(option, anchor, spec)
	}
	return spec, true
}

// decodeMonthly resolves a MONTHLY rule to by-day-of-month or by-nth-weekday,
// requiring any BYMONTHDAY / nth-weekday to agree with the anchor (Google derives
// both from the start date, so a disagreeing rule can't be seeded faithfully).
func decodeMonthly(option *rrule.ROption, anchor time.Time, spec RecurSpec) (RecurSpec, bool) {
	if len(option.Bymonth) > 0 {
		return RecurSpec{}, false
	}
	hasMonthday := len(option.Bymonthday) > 0
	hasByday := len(option.Byweekday) > 0
	switch {
	case !hasMonthday && !hasByday:
		return spec, true // bare monthly = by day-of-month (the anchor's day)
	case hasMonthday && !hasByday:
		if len(option.Bymonthday) != 1 || option.Bymonthday[0] != anchor.Day() {
			return RecurSpec{}, false
		}
		return spec, true
	case hasByday && !hasMonthday:
		if len(option.Byweekday) != 1 {
			return RecurSpec{}, false
		}
		w := option.Byweekday[0]
		n := w.N()
		if n == 0 {
			return RecurSpec{}, false // MONTHLY;BYDAY=TU (every Tuesday) is not in the vocabulary
		}
		wd := rruleToWeekday(w)
		if wd != anchor.Weekday() || !nthMatchesAnchor(n, anchor) {
			return RecurSpec{}, false
		}
		spec.MonthlyNth = n
		spec.MonthlyWeekday = wd
		return spec, true
	default: // both BYMONTHDAY and BYDAY
		return RecurSpec{}, false
	}
}

// decodeYearly resolves a YEARLY rule, which the vocabulary derives entirely from
// the anchor date: it accepts only a bare rule or one whose BYMONTH/BYMONTHDAY
// name the anchor's own month/day. Any BYDAY makes it non-representable.
func decodeYearly(option *rrule.ROption, anchor time.Time, spec RecurSpec) (RecurSpec, bool) {
	if len(option.Byweekday) > 0 {
		return RecurSpec{}, false
	}
	if len(option.Bymonth) > 0 {
		if len(option.Bymonth) != 1 || time.Month(option.Bymonth[0]) != anchor.Month() {
			return RecurSpec{}, false
		}
	}
	if len(option.Bymonthday) > 0 {
		if len(option.Bymonthday) != 1 || option.Bymonthday[0] != anchor.Day() {
			return RecurSpec{}, false
		}
	}
	return spec, true
}

// nthMatchesAnchor reports whether the anchor is the nth (1..4) or last (-1)
// occurrence of its weekday within its month — the check that a monthly
// nth-weekday rule agrees with the start date.
func nthMatchesAnchor(n int, anchor time.Time) bool {
	weekOfMonth := (anchor.Day()-1)/7 + 1
	switch {
	case n >= 1 && n <= 4:
		return weekOfMonth == n
	case n == -1:
		return anchor.Day()+7 > daysInMonth(anchor.Year(), anchor.Month())
	default:
		return false
	}
}

// rruleToWeekday maps an rrule-go weekday (Mon=0 … Sun=6) back to a time.Weekday.
func rruleToWeekday(w rrule.Weekday) time.Weekday {
	switch w.Day() {
	case 0:
		return time.Monday
	case 1:
		return time.Tuesday
	case 2:
		return time.Wednesday
	case 3:
		return time.Thursday
	case 4:
		return time.Friday
	case 5:
		return time.Saturday
	default:
		return time.Sunday
	}
}
