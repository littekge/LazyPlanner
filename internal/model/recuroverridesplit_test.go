package model

import (
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

// TestSplitPartitionsOverridesAcrossTheSplitInstant pins ALL THREE positions an
// existing RECURRENCE-ID override can occupy relative to a this-and-future split
// point, because NewSeriesFrom's carry-forward filter is a boundary comparison
// (`t.Unix() <= occ.Unix()` — skip) and a one-sided test leaves the comparison
// free to flip silently:
//
//   - strictly BEFORE occ  → stays with the capped past series.
//   - exactly AT occ       → dropped from BOTH halves. This is the boundary the
//     mutation canary escaped through: the split-point occurrence is redefined by
//     the caller's draft, so carrying its stale override into the new series
//     re-keys the pre-split customization to the new UID, where it OUTRANKS the
//     master's freshly-mutated instance (eventOccurrences lets an override replace
//     the master's slot) — the user's edit silently vanishes and the old
//     customization resurrects.
//   - strictly AFTER occ   → carried into the new series, re-keyed to its UID
//     (also covered on its own by TestSplitCarriesFutureOverride).
func TestSplitPartitionsOverridesAcrossTheSplitInstant(t *testing.T) {
	const uid = "split-boundary"
	before := time.Date(2026, 1, 6, 9, 0, 0, 0, time.UTC)
	occ := time.Date(2026, 1, 13, 9, 0, 0, 0, time.UTC)
	after := time.Date(2026, 1, 27, 9, 0, 0, 0, time.UTC)

	override := func(rid time.Time, summary string) string {
		stamp := rid.Format("20060102T150405Z")
		return "BEGIN:VEVENT\r\nUID:" + uid + "\r\nDTSTAMP:20260101T000000Z\r\n" +
			"RECURRENCE-ID:" + stamp + "\r\nDTSTART:" + stamp + "\r\n" +
			"DTEND:" + rid.Add(time.Hour).Format("20060102T150405Z") + "\r\n" +
			"SUMMARY:" + summary + "\r\nEND:VEVENT\r\n"
	}
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:" + uid + "\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART:20260106T090000Z\r\nDTEND:20260106T100000Z\r\n" +
		"RRULE:FREQ=WEEKLY\r\nSUMMARY:base\r\nEND:VEVENT\r\n" +
		override(before, "custom-before") +
		override(occ, "custom-at-split") +
		override(after, "custom-after") +
		"END:VCALENDAR\r\n"

	obj, err := Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	// The UI seeds the form from the occurrence, so the draft's start IS the split
	// point (internal/ui/recur_edit.go, scopeFuture).
	draft := EventDraft{Summary: "edited", Start: occ, End: occ.Add(time.Hour)}
	capped, future, err := SplitEvent(obj, uid, occ, draft, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.UTC)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(future.Events) == 0 {
		t.Fatal("no future series")
	}
	newUID := future.Events[0].UID

	// Past half: keeps the pre-split override, and only that one.
	if ov := overrideSummaryAt(capped.Calendar, before); ov != "custom-before" {
		t.Errorf("capped past series override at %v = %q, want %q (pre-split customization lost)",
			before, ov, "custom-before")
	}
	for _, rid := range []time.Time{occ, after} {
		if ov := overrideSummaryAt(capped.Calendar, rid); ov != "" {
			t.Errorf("capped past series still carries an override at %v (%q); CapSeries must drop overrides after the cut", rid, ov)
		}
	}

	// Future half: the after-override is carried and re-keyed; the AT-occ override
	// is dropped (the draft redefines that instance).
	if ov := overrideSummaryAt(future.Calendar, after); ov != "custom-after" {
		t.Errorf("future series override at %v = %q, want %q (post-split customization lost)",
			after, ov, "custom-after")
	}
	if uidGot := overrideUIDAt(future.Calendar, after); uidGot != newUID {
		t.Errorf("carried override UID = %q, want the new series UID %q", uidGot, newUID)
	}
	if ov := overrideSummaryAt(future.Calendar, occ); ov != "" {
		t.Errorf("future series carried the override AT the split point (%q); it must be dropped — the draft redefines that occurrence", ov)
	}

	// The observable consequence: the split-point occurrence must expand with the
	// DRAFT's summary, not the stale override's.
	occs, err := future.EventOccurrences(occ.Add(-time.Hour), occ.Add(time.Hour))
	if err != nil {
		t.Fatalf("expand future: %v", err)
	}
	if len(occs) != 1 {
		t.Fatalf("future series yielded %d occurrences at the split point, want 1", len(occs))
	}
	if got := occs[0].Event.Summary; got != "edited" {
		t.Errorf("split-point occurrence SUMMARY = %q, want %q (the user's edit was overridden by the stale pre-split customization)", got, "edited")
	}
}

// overrideSummaryAt returns the SUMMARY of the component whose RECURRENCE-ID is
// rid, or "" when no such override exists.
func overrideSummaryAt(cal *ical.Calendar, rid time.Time) string {
	if c := overrideAt(cal, rid); c != nil {
		return text(c.Props, ical.PropSummary)
	}
	return ""
}

// overrideUIDAt returns the UID of the component whose RECURRENCE-ID is rid, or
// "" when no such override exists.
func overrideUIDAt(cal *ical.Calendar, rid time.Time) string {
	if c := overrideAt(cal, rid); c != nil {
		return text(c.Props, ical.PropUID)
	}
	return ""
}

func overrideAt(cal *ical.Calendar, rid time.Time) *ical.Component {
	for _, c := range cal.Children {
		p := c.Props.Get(ical.PropRecurrenceID)
		if p == nil {
			continue
		}
		if t, err := resolveDateTime(p, time.UTC); err == nil && t.Unix() == rid.Unix() {
			return c
		}
	}
	return nil
}

// TestSplitCarriesFutureOverride guards H5: a RECURRENCE-ID override on an
// occurrence after the split point must survive a this-and-future split (carried
// into the new series, re-keyed to its UID) instead of being dropped from both
// halves.
func TestSplitCarriesFutureOverride(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
		// master: weekly from Jan 6
		"BEGIN:VEVENT\r\nUID:split-1\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART:20260106T090000Z\r\nDTEND:20260106T100000Z\r\n" +
		"RRULE:FREQ=WEEKLY\r\nSUMMARY:base\r\nEND:VEVENT\r\n" +
		// override: the Jan 20 occurrence is customized
		"BEGIN:VEVENT\r\nUID:split-1\r\nDTSTAMP:20260101T000000Z\r\n" +
		"RECURRENCE-ID:20260120T090000Z\r\n" +
		"DTSTART:20260120T090000Z\r\nDTEND:20260120T100000Z\r\nSUMMARY:custom\r\nEND:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	obj, err := Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Split "this & future" from the Jan 13 occurrence (before the Jan 20 override).
	occ := time.Date(2026, 1, 13, 9, 0, 0, 0, time.UTC)
	_, future, err := SplitEvent(obj, "split-1", occ, EventDraft{Summary: "future-base"}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.UTC)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(future.Events) == 0 {
		t.Fatal("no future series")
	}
	newUID := future.Events[0].UID

	var found bool
	for _, c := range future.Calendar.Children {
		rid := c.Props.Get(ical.PropRecurrenceID)
		if rid == nil {
			continue
		}
		ridTime, err := resolveDateTime(rid, time.UTC)
		if err != nil || ridTime.Unix() != time.Date(2026, 1, 20, 9, 0, 0, 0, time.UTC).Unix() {
			continue
		}
		found = true
		if got := text(c.Props, ical.PropUID); got != newUID {
			t.Errorf("carried override UID = %q, want the new series UID %q", got, newUID)
		}
		if got := text(c.Props, ical.PropSummary); got != "custom" {
			t.Errorf("carried override SUMMARY = %q, want %q (customization lost)", got, "custom")
		}
	}
	if !found {
		t.Error("future override was dropped by the split (lost from both halves)")
	}
}
