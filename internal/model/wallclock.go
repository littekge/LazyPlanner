package model

import (
	"time"

	"github.com/teambition/rrule-go"
)

// RFC 5545 §3.8.5.3 generates a recurrence set from the anchor's *local* time, so
// the instants a rule produces are wall clocks first and absolute times only
// afterwards. rrule-go blurs the two: it enumerates each period's day as
// `firstyday.AddDate(0, 0, i)` where firstyday is January 1 **midnight** in the
// anchor's zone, then re-stamps the anchor's clock onto that date. When the target
// day's local midnight does not exist — a DST transition landing exactly on
// 00:00 — AddDate normalizes backwards into the previous day, so the generated
// date is the previous day and the instant duplicates the previous occurrence.
// rrule-go's own Set.Iterator then drops it as a duplicate and the day vanishes
// from the series entirely, regardless of the anchor's time of day (firstyday is
// always midnight) and regardless of frequency (every FREQ shares that
// enumeration).
//
// Sweeping the IANA database found 11 zones that lose a day this way — Havana,
// Santiago, Coyhaique, Punta_Arenas, Palmer, Azores, Scoresbysund, Sao_Paulo,
// Asuncion, Campo_Grande, Cuiaba — and it is not merely cosmetic: completing a
// recurring todo advances its DUE past the missing day and pushes that hole to the
// server.
//
// The fix is to honour the RFC's own order of operations: iterate the rule in
// wall-clock space, where a day can never be missing, and resolve each generated
// wall clock into the real zone as it is yielded. Vendored code must not be
// hand-edited, so this lives at our call boundary alongside the existing panic and
// step-bound guards.

// wallClockUTC re-stamps t's wall clock in UTC, a zone with no transitions and so
// no missing days. It is a representation change, not a conversion: the returned
// instant names the same calendar date and clock reading, and is meaningful only
// as wall-clock arithmetic input.
func wallClockUTC(t time.Time) time.Time {
	y, m, d := t.Date()
	h, min, s := t.Clock()
	return time.Date(y, m, d, h, min, s, t.Nanosecond(), time.UTC)
}

// resolveWallClock maps a generated wall clock back into loc.
//
// It deliberately reproduces time.Date's behaviour — including its backwards
// normalization of a nonexistent clock reading — whenever that keeps the
// occurrence on its own calendar day, so every zone that expands correctly today
// keeps byte-identical instants. Only when normalization would move the
// occurrence to a *different day* does it snap forward instead, to the first
// instant of the intended day that exists. A DST gap may shift an occurrence's
// time, as it already does; it must never move it to another day.
func resolveWallClock(w time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	y, mo, d := w.Date()
	h, min, s := w.Clock()
	t := time.Date(y, mo, d, h, min, s, w.Nanosecond(), loc)
	if ty, tm, td := t.Date(); ty == y && tm == mo && td == d {
		return t
	}
	if snapped, ok := gapTransition(w, loc); ok {
		return snapped
	}
	// Unbracketable (a zone whose offsets moved more than once around w): keep
	// time.Date's reading rather than guess. No worse than not having this path.
	return t
}

// gapTransition returns the instant at which loc's offset jumps forward over the
// wall clock w, i.e. the first instant of w's day that exists when w itself does
// not. It brackets the transition using the offsets in effect a day either side of
// w and bisects to second resolution — the stdlib exposes no transition table, and
// probing fixed candidate offsets would miss the sub-hour jumps some zones use.
// ok is false when the bracket does not hold a single forward jump, so the caller
// can fall back rather than return a fabricated instant.
func gapTransition(w time.Time, loc *time.Location) (time.Time, bool) {
	y, mo, d := w.Date()
	h, min, s := w.Clock()
	u := time.Date(y, mo, d, h, min, s, w.Nanosecond(), time.UTC)

	_, before := u.Add(-24 * time.Hour).In(loc).Zone()
	_, after := u.Add(24 * time.Hour).In(loc).Zone()
	if after <= before {
		return time.Time{}, false
	}
	// Reading w with the LARGER (post-transition) offset lands before the jump;
	// reading it with the smaller one lands after. The transition is in (lo, hi].
	lo := u.Add(-time.Duration(after) * time.Second)
	hi := u.Add(-time.Duration(before) * time.Second)
	if _, o := lo.In(loc).Zone(); o != before {
		return time.Time{}, false
	}
	if _, o := hi.In(loc).Zone(); o != after {
		return time.Time{}, false
	}
	for hi.Sub(lo) > time.Second {
		mid := lo.Add(hi.Sub(lo) / 2)
		if _, o := mid.In(loc).Zone(); o == after {
			hi = mid
		} else {
			lo = mid
		}
	}
	return hi.In(loc), true
}

// wallClockSet is a recurrence set whose every instant — anchor, RDATEs, EXDATEs
// and the RRULE's UNTIL bound — is authored in wall-clock space, together with the
// zone its generated instants resolve back into. Keeping the rule's own internal
// comparisons (UNTIL, "not before DTSTART", EXDATE equality) in the same space as
// the instants it generates is what makes the translation total rather than
// piecemeal.
type wallClockSet struct {
	set *rrule.Set
	loc *time.Location
}

// newWallClockSet returns an empty set resolving into loc.
func newWallClockSet(loc *time.Location) *wallClockSet {
	if loc == nil {
		loc = time.Local
	}
	return &wallClockSet{set: &rrule.Set{}, loc: loc}
}

// dtStart sets the set's anchor.
func (s *wallClockSet) dtStart(t time.Time) { s.set.DTStart(wallClockUTC(t.In(s.loc))) }

// rDate adds an explicit extra instant.
func (s *wallClockSet) rDate(t time.Time) { s.set.RDate(wallClockUTC(t.In(s.loc))) }

// exDate excludes an instant. Converting through loc first keeps a UTC-serialized
// EXDATE matching the TZID-anchored instant it names, exactly as instant equality
// did before.
func (s *wallClockSet) exDate(t time.Time) { s.set.ExDate(wallClockUTC(t.In(s.loc))) }

// rRule installs o anchored at anchor, translating both the anchor and any UNTIL
// bound into wall-clock space.
func (s *wallClockSet) rRule(o rrule.ROption, anchor time.Time) error {
	o.Dtstart = wallClockUTC(anchor.In(s.loc))
	if !o.Until.IsZero() {
		o.Until = wallClockUTC(o.Until.In(s.loc))
	}
	rule, err := rrule.NewRRule(o)
	if err != nil {
		return err
	}
	s.set.RRule(rule)
	return nil
}

// resolve maps one generated wall clock back into the set's zone.
func (s *wallClockSet) resolve(w time.Time) time.Time { return resolveWallClock(w, s.loc) }
