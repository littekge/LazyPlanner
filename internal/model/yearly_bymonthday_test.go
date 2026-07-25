package model

import (
	"testing"
	"time"

	"github.com/teambition/rrule-go"
)

// yearlyBMDEvent decodes a one-VEVENT calendar with the given DTSTART + RRULE.
func yearlyBMDEvent(t *testing.T, dtstart, rule string) *Event {
	t.Helper()
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:y@t\r\nSUMMARY:Yearly BYMONTHDAY\r\nDTSTAMP:20260701T000000Z\r\n" +
		"DTSTART:" + dtstart + "\r\nDTEND:" + dtstart + "\r\nRRULE:" + rule +
		"\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	obj := decodeForTest(t, ics)
	for _, e := range obj.Events {
		if e.UID == "y@t" {
			return e
		}
	}
	t.Fatal("event not decoded")
	return nil
}

// TestYearlyByMonthdayWithoutBymonth guards the pass-23 HIGH: a YEARLY rule
// carrying BYMONTHDAY but no BYMONTH expands per RFC 5545 to that day of EVERY
// month (12x/year), so it is outside the once-a-year editable vocabulary and must
// decompose to ok=false — otherwise a grab day-move re-anchors DTSTART away from
// the untouched BYMONTHDAY and the event vanishes from the calendar.
func TestYearlyByMonthdayWithoutBymonth(t *testing.T) {
	const dtstart = "20260715T090000Z"
	const rule = "FREQ=YEARLY;BYMONTHDAY=15"
	anchor := time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC)

	// (0) Ground truth: the rule fires monthly, not yearly.
	ev := yearlyBMDEvent(t, dtstart, rule)
	occ, err := ev.Occurrences(
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 7, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Occurrences: %v", err)
	}
	if len(occ) != 12 {
		t.Fatalf("premise: got %d occurrences over one year, want 12", len(occ))
	}

	// (1) The decomposer must decline it.
	opt, err := rrule.StrToROption(rule)
	if err != nil {
		t.Fatalf("StrToROption: %v", err)
	}
	if spec, ok := RecurSpecFromRule(opt, anchor); ok {
		t.Errorf("RecurSpecFromRule(%q) = ok (spec %+v), want unrepresentable: "+
			"no BYMONTH means it fires %d times/year, not yearly", rule, spec, len(occ))
	}

	// (2) Declining it is what keeps the summary honest — no "Yearly on Jul 15".
	if got := RecurrenceSummary(ev.Raw, anchor, time.UTC); got == "Yearly on Jul 15" {
		t.Errorf("RecurrenceSummary = %q, but the series fires %d times/year", got, len(occ))
	}

	// (3) A grab day-move must be blocked, not silently re-anchored.
	newStart := anchor.AddDate(0, 0, 1)
	if recur, blocked := ReanchoredRecurrence(ev, newStart); !blocked {
		t.Errorf("ReanchoredRecurrence(+1 day) = (%v, blocked=false); a day-move on this "+
			"rule must be blocked so the anchor never contradicts BYMONTHDAY", recur)
	}

	// (4) The consequence the block prevents: had the move gone through, DTSTART
	// would contradict BYMONTHDAY=15 and July would hold no occurrence at all.
	moved := yearlyBMDEvent(t, "20260716T090000Z", rule)
	julOcc, err := moved.Occurrences(
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Occurrences (moved): %v", err)
	}
	if len(julOcc) != 0 {
		t.Fatalf("premise: a moved DTSTART unexpectedly still fires in July (%d)", len(julOcc))
	}
}

// TestYearlyByMonthRuleStillRepresentable is the other side of the class: a
// genuinely yearly rule (BYMONTHDAY qualified by BYMONTH, both agreeing with the
// anchor) must still decode, and the spec must still serialize→decompose to
// identity — the fix must not over-reject.
func TestYearlyByMonthRuleStillRepresentable(t *testing.T) {
	anchor := time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC)
	const rule = "FREQ=YEARLY;BYMONTH=7;BYMONTHDAY=20"

	opt, err := rrule.StrToROption(rule)
	if err != nil {
		t.Fatalf("StrToROption: %v", err)
	}
	spec, ok := RecurSpecFromRule(opt, anchor)
	if !ok {
		t.Fatalf("RecurSpecFromRule(%q) not ok, want representable", rule)
	}
	if want := (RecurSpec{Freq: FreqYearly}); !specEqual(spec, want) {
		t.Errorf("decomposed %q to %+v, want %+v", rule, spec, want)
	}
	// Identity: the decomposed spec re-serializes and re-decomposes to itself.
	str := spec.ROption().RRuleString()
	back, err := rrule.StrToROption(str)
	if err != nil {
		t.Fatalf("StrToROption(%q): %v", str, err)
	}
	again, ok := RecurSpecFromRule(back, anchor)
	if !ok || !specEqual(again, spec) {
		t.Errorf("round-trip via %q: got (%+v, ok=%v), want %+v", str, again, ok, spec)
	}

	// A single yearly occurrence per year, unlike the BYMONTH-less shape above.
	ev := yearlyBMDEvent(t, "20260720T090000Z", rule)
	occ, err := ev.Occurrences(
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 7, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Occurrences: %v", err)
	}
	if len(occ) != 1 {
		t.Errorf("got %d occurrences over one year, want 1", len(occ))
	}
}

// TestRejectedRuleBytesPreserved is the point of ok=false: a declined rule is
// kept byte-for-byte rather than re-serialized from a lossy spec.
func TestRejectedRuleBytesPreserved(t *testing.T) {
	const rule = "FREQ=YEARLY;BYMONTHDAY=15"
	ev := yearlyBMDEvent(t, "20260715T090000Z", rule)

	if _, blocked := ReanchoredRecurrence(ev, ev.Start.AddDate(0, 0, 1)); !blocked {
		t.Fatalf("ReanchoredRecurrence: want blocked for a kept custom rule")
	}
	prop := ev.Raw.Props.Get("RRULE")
	if prop == nil {
		t.Fatal("RRULE property missing after decomposition")
	}
	if prop.Value != rule {
		t.Errorf("RRULE bytes = %q, want %q untouched", prop.Value, rule)
	}
}
