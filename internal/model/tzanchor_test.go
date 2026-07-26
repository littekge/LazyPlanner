package model

import (
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

func mustZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("zone %q unavailable: %v", name, err)
	}
	return loc
}

func propOf(t *testing.T, obj *Parsed, name string) *ical.Prop {
	t.Helper()
	for _, c := range obj.Calendar.Children {
		if c.Name == ical.CompEvent || c.Name == ical.CompToDo {
			return c.Props.Get(name)
		}
	}
	t.Fatalf("no item component carrying %s", name)
	return nil
}

// Creating a recurring event anchors it in the user's zone, so the rule the app
// derived from the local weekday is evaluated against that same weekday.
func TestNewRecurringEventAnchorsWithTZID(t *testing.T) {
	ny := mustZone(t, "America/New_York")
	start := time.Date(2026, 8, 25, 20, 0, 0, 0, ny) // Tuesday 8pm
	obj, err := NewEventObject(EventDraft{
		Summary: "Evening sync",
		Start:   start,
		End:     start.Add(time.Hour),
		Recur:   &RecurSpec{Freq: FreqWeekly, Weekdays: []time.Weekday{time.Tuesday}},
	}, start)
	if err != nil {
		t.Fatal(err)
	}
	dtstart := propOf(t, obj, ical.PropDateTimeStart)
	if got := dtstart.Params.Get(ical.ParamTimezoneID); got != "America/New_York" {
		t.Errorf("DTSTART TZID = %q, want America/New_York", got)
	}
	if got := dtstart.Value; got != "20260825T200000" {
		t.Errorf("DTSTART = %q, want the local wall clock 20260825T200000", got)
	}

	// The whole point: the series must fall on Tuesdays and hold 20:00 across DST.
	raw, err := obj.Encode()
	if err != nil {
		t.Fatal(err)
	}
	reparsed, err := Decode(raw, ny)
	if err != nil {
		t.Fatal(err)
	}
	ev := reparsed.Events[0]
	occs, err := ev.Occurrences(time.Date(2026, 8, 20, 0, 0, 0, 0, ny), time.Date(2026, 11, 20, 0, 0, 0, 0, ny))
	if err != nil {
		t.Fatal(err)
	}
	if len(occs) == 0 {
		t.Fatal("no occurrences")
	}
	if !occs[0].Start.Equal(start) {
		t.Errorf("first occurrence = %v, want the event's own start %v", occs[0].Start, start)
	}
	for _, o := range occs {
		local := o.Start.In(ny)
		if local.Weekday() != time.Tuesday {
			t.Errorf("occurrence %v is a %v, want Tuesday", local, local.Weekday())
		}
		if local.Hour() != 20 {
			t.Errorf("occurrence %v drifted off 20:00", local)
		}
	}
}

// A VTIMEZONE for the referenced zone travels with the object.
func TestNewRecurringEventCarriesVTimezone(t *testing.T) {
	ny := mustZone(t, "America/New_York")
	start := time.Date(2026, 8, 25, 20, 0, 0, 0, ny)
	obj, err := NewEventObject(EventDraft{
		Summary: "Evening sync", Start: start, End: start.Add(time.Hour),
		Recur: &RecurSpec{Freq: FreqWeekly, Weekdays: []time.Weekday{time.Tuesday}},
	}, start)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, c := range obj.Calendar.Children {
		if c.Name == ical.CompTimezone && c.Props.Get(ical.PropTimezoneID).Value == "America/New_York" {
			found = true
		}
	}
	if !found {
		t.Error("no VTIMEZONE for the referenced TZID")
	}
}

// A NON-recurring event keeps the UTC form: no TZID, no VTIMEZONE, no churn.
func TestNonRecurringEventStaysUTC(t *testing.T) {
	ny := mustZone(t, "America/New_York")
	start := time.Date(2026, 8, 25, 20, 0, 0, 0, ny)
	obj, err := NewEventObject(EventDraft{Summary: "One-off", Start: start, End: start.Add(time.Hour)}, start)
	if err != nil {
		t.Fatal(err)
	}
	dtstart := propOf(t, obj, ical.PropDateTimeStart)
	if tzid := dtstart.Params.Get(ical.ParamTimezoneID); tzid != "" {
		t.Errorf("non-recurring DTSTART carries TZID %q", tzid)
	}
	if !strings.HasSuffix(dtstart.Value, "Z") {
		t.Errorf("non-recurring DTSTART = %q, want the UTC Z form", dtstart.Value)
	}
	for _, c := range obj.Calendar.Children {
		if c.Name == ical.CompTimezone {
			t.Errorf("non-recurring object carries a VTIMEZONE %q", c.Props.Get(ical.PropTimezoneID).Value)
		}
	}
}

// THE SAFETY PROPERTY: editing a recurring item WITHOUT authoring a rule must not
// re-anchor it — re-interpreting BY* against a new zone would move the series.
func TestEditWithoutRuleChangeKeepsAnchorForm(t *testing.T) {
	berlin := mustZone(t, "Europe/Berlin")
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\nBEGIN:VEVENT\r\nUID:keep-1\r\n" +
		"DTSTAMP:20260101T000000Z\r\nDTSTART:20260824T230000Z\r\nDTEND:20260825T000000Z\r\n" +
		"RRULE:FREQ=WEEKLY;BYDAY=MO\r\nSUMMARY:old\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	obj, err := Decode([]byte(ics), berlin)
	if err != nil {
		t.Fatal(err)
	}
	before, err := obj.Events[0].Occurrences(
		time.Date(2026, 8, 20, 0, 0, 0, 0, berlin), time.Date(2026, 9, 20, 0, 0, 0, 0, berlin))
	if err != nil {
		t.Fatal(err)
	}

	// A summary-only edit: Recur is nil, so the rule is untouched.
	//
	// The instants are re-expressed .In(berlin) because that is what production
	// hands applyEvent: readEventDraft parses the form's date/time fields with
	// a.loc, so an edited draft ALWAYS carries the user's zone regardless of how
	// the stored value was serialized. Passing obj.Events[0].Start unchanged would
	// carry time.UTC (go-ical decodes a Z value into time.UTC), which IsNamedZone
	// rejects — and the test would then pass even with the recur gate removed.
	edited, err := EditEvent(obj, "keep-1", EventDraft{
		Summary: "renamed",
		Start:   obj.Events[0].Start.In(berlin),
		End:     obj.Events[0].End.In(berlin),
	}, time.Now(), berlin)
	if err != nil {
		t.Fatal(err)
	}
	dtstart := propOf(t, edited, ical.PropDateTimeStart)
	if tzid := dtstart.Params.Get(ical.ParamTimezoneID); tzid != "" {
		t.Errorf("untouched rule was re-anchored to TZID %q", tzid)
	}
	after, err := edited.Events[0].Occurrences(
		time.Date(2026, 8, 20, 0, 0, 0, 0, berlin), time.Date(2026, 9, 20, 0, 0, 0, 0, berlin))
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("occurrence count changed: %d → %d", len(before), len(after))
	}
	for i := range before {
		if !before[i].Start.Equal(after[i].Start) {
			t.Errorf("occurrence %d moved: %v → %v", i, before[i].Start, after[i].Start)
		}
	}
}

// A recurring TODO's DUE is its anchor and gets the same treatment.
func TestNewRecurringTodoAnchorsDueWithTZID(t *testing.T) {
	ny := mustZone(t, "America/New_York")
	due := time.Date(2026, 8, 25, 20, 0, 0, 0, ny)
	obj := NewTodoObject(TodoDraft{
		Summary: "Water plants", HasDue: true, Due: due,
		Recur: &RecurSpec{Freq: FreqWeekly, Weekdays: []time.Weekday{time.Tuesday}},
	}, due)
	if got := propOf(t, obj, ical.PropDue).Params.Get(ical.ParamTimezoneID); got != "America/New_York" {
		t.Errorf("DUE TZID = %q, want America/New_York", got)
	}
}

// newTZIDWeeklyEvent builds the object the other anchor writers operate on: a
// weekly Tuesday series anchored with a TZID rather than the UTC Z form.
func newTZIDWeeklyEvent(t *testing.T, ny *time.Location) (*Parsed, string, time.Time) {
	t.Helper()
	start := time.Date(2026, 8, 25, 20, 0, 0, 0, ny)
	obj, err := NewEventObject(EventDraft{
		Summary: "Evening sync", Start: start, End: start.Add(time.Hour),
		Recur: &RecurSpec{Freq: FreqWeekly, Weekdays: []time.Weekday{time.Tuesday}},
	}, start)
	if err != nil {
		t.Fatal(err)
	}
	if got := propOf(t, obj, ical.PropDateTimeStart).Params.Get(ical.ParamTimezoneID); got == "" {
		t.Fatal("fixture is not TZID-anchored; the rest of the test would be vacuous")
	}
	return obj, obj.Events[0].UID, start
}

// An EXDATE must be written in the master anchor's zone, not flattened to UTC:
// a value type / zone that disagrees with DTSTART names a different instant, so
// the deleted occurrence comes back.
func TestAddExceptionKeepsMasterAnchorZone(t *testing.T) {
	ny := mustZone(t, "America/New_York")
	obj, uid, start := newTZIDWeeklyEvent(t, ny)
	victim := start.AddDate(0, 0, 7) // the second Tuesday

	out, err := AddException(obj, uid, victim, false, start, ny)
	if err != nil {
		t.Fatal(err)
	}
	ex := propOf(t, out, ical.PropExceptionDates)
	if got := ex.Params.Get(ical.ParamTimezoneID); got != "America/New_York" {
		t.Errorf("EXDATE TZID = %q, want America/New_York (value %q)", got, ex.Value)
	}

	// Round-trip, because a mismatched EXDATE only fails once re-read.
	raw, err := out.Encode()
	if err != nil {
		t.Fatal(err)
	}
	reparsed, err := Decode(raw, ny)
	if err != nil {
		t.Fatal(err)
	}
	occs, err := reparsed.Events[0].Occurrences(start.AddDate(0, 0, -1), start.AddDate(0, 0, 30))
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range occs {
		if o.Start.Equal(victim) {
			t.Fatalf("the EXDATE'd occurrence %v is still in the series", victim)
		}
	}
}

// A RECURRENCE-ID must likewise match the master's DTSTART form, or the override
// targets no instance and the object grows a phantom duplicate.
func TestAddOccurrenceOverrideKeepsMasterAnchorZone(t *testing.T) {
	ny := mustZone(t, "America/New_York")
	obj, uid, start := newTZIDWeeklyEvent(t, ny)
	target := start.AddDate(0, 0, 7)

	out, err := AddOccurrenceOverride(obj, uid, target, false, func(c *ical.Component) {
		c.Props.SetText(ical.PropSummary, "moved")
	}, start, ny)
	if err != nil {
		t.Fatal(err)
	}

	var rid *ical.Prop
	for _, c := range out.Calendar.Children {
		if p := c.Props.Get(ical.PropRecurrenceID); p != nil {
			rid = p
		}
	}
	if rid == nil {
		t.Fatal("no override component was added")
	}
	if got := rid.Params.Get(ical.ParamTimezoneID); got != "America/New_York" {
		t.Errorf("RECURRENCE-ID TZID = %q, want America/New_York (value %q)", got, rid.Value)
	}

	// The override must be recognised as the SAME instance on a second call, not
	// duplicated — which is exactly what a zone mismatch would cause.
	again, err := AddOccurrenceOverride(out, uid, target, false, func(c *ical.Component) {
		c.Props.SetText(ical.PropSummary, "moved twice")
	}, start, ny)
	if err != nil {
		t.Fatal(err)
	}
	overrides := 0
	for _, c := range again.Calendar.Children {
		if c.Props.Get(ical.PropRecurrenceID) != nil {
			overrides++
		}
	}
	if overrides != 1 {
		t.Errorf("override components = %d, want 1 (the second edit re-targeted the same instance)", overrides)
	}
}

// Completing a recurring todo rolls its DUE forward; the roll must keep the TZID
// form, or every completion silently re-anchors the rule into UTC and walks the
// series onto the wrong weekday.
func TestAdvanceRecurringTodoKeepsAnchorZone(t *testing.T) {
	ny := mustZone(t, "America/New_York")
	due := time.Date(2026, 8, 25, 20, 0, 0, 0, ny) // Tuesday
	obj := NewTodoObject(TodoDraft{
		Summary: "Water plants", HasDue: true, Due: due,
		Recur: &RecurSpec{Freq: FreqWeekly, Weekdays: []time.Weekday{time.Tuesday}},
	}, due)
	if propOf(t, obj, ical.PropDue).Params.Get(ical.ParamTimezoneID) == "" {
		t.Fatal("fixture is not TZID-anchored; the rest of the test would be vacuous")
	}

	// Advance across the November DST boundary, where a UTC re-anchor also shifts
	// the wall clock by an hour.
	cur := obj
	for i := 0; i < 12; i++ {
		out, done, err := AdvanceRecurringTodo(cur, cur.Todos[0].UID, due, ny)
		if err != nil {
			t.Fatalf("advance %d: %v", i, err)
		}
		if done {
			t.Fatalf("series reported exhausted at step %d; it has no COUNT/UNTIL", i)
		}
		p := propOf(t, out, ical.PropDue)
		if got := p.Params.Get(ical.ParamTimezoneID); got != "America/New_York" {
			t.Fatalf("advance %d: DUE TZID = %q, want America/New_York (value %q)", i, got, p.Value)
		}
		next := out.Todos[0].Due.In(ny)
		if next.Weekday() != time.Tuesday || next.Hour() != 20 {
			t.Fatalf("advance %d: DUE = %v, want a Tuesday at 20:00", i, next)
		}
		cur = out
	}
}

// IRON RULE: ensureVTimezone is additive only. A VTIMEZONE the object already
// carries — a server's own definition, possibly a shape we would not have
// emitted — is left byte-identical and never joined by a second definition of
// the same TZID.
func TestExistingVTimezoneIsPreservedNotReplaced(t *testing.T) {
	ny := mustZone(t, "America/New_York")
	// A deliberately non-LazyPlanner shape: one STANDARD observance, no RRULE, a
	// TZURL and an X- property we must not touch.
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//foreign//EN\r\n" +
		"BEGIN:VTIMEZONE\r\nTZID:America/New_York\r\nTZURL:http://example.invalid/ny\r\n" +
		"X-VENDOR-TAG:keep-me\r\nBEGIN:STANDARD\r\nDTSTART:19701101T020000\r\n" +
		"TZOFFSETFROM:-0400\r\nTZOFFSETTO:-0500\r\nEND:STANDARD\r\nEND:VTIMEZONE\r\n" +
		"BEGIN:VEVENT\r\nUID:foreign-1\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART;TZID=America/New_York:20260825T200000\r\n" +
		"DTEND;TZID=America/New_York:20260825T210000\r\nSUMMARY:old\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"
	obj, err := Decode([]byte(ics), ny)
	if err != nil {
		t.Fatal(err)
	}

	// Author a rule — the case that turns on the zoned-anchor path.
	start := time.Date(2026, 8, 25, 20, 0, 0, 0, ny)
	edited, err := EditEvent(obj, "foreign-1", EventDraft{
		Summary: "renamed", Start: start, End: start.Add(time.Hour),
		Recur: &RecurSpec{Freq: FreqWeekly, Weekdays: []time.Weekday{time.Tuesday}},
	}, start, ny)
	if err != nil {
		t.Fatal(err)
	}

	var zones []*ical.Component
	for _, c := range edited.Calendar.Children {
		if c.Name == ical.CompTimezone {
			zones = append(zones, c)
		}
	}
	if len(zones) != 1 {
		t.Fatalf("VTIMEZONE count = %d, want 1 (the existing one, not a duplicate)", len(zones))
	}
	tz := zones[0]
	if got := text(tz.Props, ical.PropTimezoneURL); got != "http://example.invalid/ny" {
		t.Errorf("TZURL = %q; the foreign VTIMEZONE was replaced, not preserved", got)
	}
	if got := text(tz.Props, "X-VENDOR-TAG"); got != "keep-me" {
		t.Errorf("X-VENDOR-TAG = %q, want keep-me", got)
	}
	if len(tz.Children) != 1 || tz.Children[0].Name != ical.CompTimezoneStandard {
		t.Errorf("observances = %d, want the single foreign STANDARD", len(tz.Children))
	}
	if tz.Children[0].Props.Get(ical.PropRecurrenceRule) != nil {
		t.Error("foreign observance gained an RRULE; it was rebuilt rather than preserved")
	}
}

// occurrenceStarts renders a series' starts in loc, for comparing two objects'
// occurrence sets.
func occurrenceStarts(t *testing.T, ev *Event, loc *time.Location, from, to time.Time) []string {
	t.Helper()
	occs, err := ev.Occurrences(from, to)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(occs))
	for _, o := range occs {
		out = append(out, o.Start.In(loc).Format("Mon 2006-01-02 15:04"))
	}
	return out
}

// A rule-authoring edit must anchor in the zone the BY* was derived from, even
// when the component already carries a FOREIGN TZID.
//
// The reviewer's repro: a server-authored DTSTART;TZID=Europe/Berlin:20260826T020000
// is the very same instant as Tuesday 20:00 in New York. A New York user edits it
// and picks the form's own "Weekly on Tue", so BYDAY=TU is derived from the New
// York Tuesday. If the existing Berlin TZID wins, the anchor stays a Berlin
// WEDNESDAY while the rule says TU — and every occurrence fires Monday 20:00 New
// York, with the first occurrence not being the event's own start. That is this
// task's original defect, reproduced on the edit path.
func TestRuleAuthoringEditOverridesForeignAnchorZone(t *testing.T) {
	ny := mustZone(t, "America/New_York")
	berlin := mustZone(t, "Europe/Berlin")

	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//foreign//EN\r\nBEGIN:VEVENT\r\n" +
		"UID:foreign-tz-1\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART;TZID=Europe/Berlin:20260826T020000\r\n" +
		"DTEND;TZID=Europe/Berlin:20260826T030000\r\nSUMMARY:old\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	obj, err := Decode([]byte(ics), berlin)
	if err != nil {
		t.Fatal(err)
	}

	// Sanity: the stored instant IS Tuesday 20:00 in New York, so the user is not
	// moving the event — only choosing a rule.
	start := obj.Events[0].Start.In(ny)
	if start.Weekday() != time.Tuesday || start.Hour() != 20 {
		t.Fatalf("fixture start = %v in NY, want a Tuesday at 20:00", start)
	}

	edited, err := EditEvent(obj, "foreign-tz-1", EventDraft{
		Summary: "renamed", Start: start, End: start.Add(time.Hour),
		Recur: &RecurSpec{Freq: FreqWeekly, Weekdays: []time.Weekday{time.Tuesday}},
	}, start, ny)
	if err != nil {
		t.Fatal(err)
	}

	dtstart := propOf(t, edited, ical.PropDateTimeStart)
	if got := dtstart.Params.Get(ical.ParamTimezoneID); got != "America/New_York" {
		t.Errorf("DTSTART TZID = %q, want America/New_York (the zone BYDAY was derived from)", got)
	}
	if got := dtstart.Value; got != "20260825T200000" {
		t.Errorf("DTSTART = %q, want the New York wall clock 20260825T200000", got)
	}

	raw, err := edited.Encode()
	if err != nil {
		t.Fatal(err)
	}
	reparsed, err := Decode(raw, ny)
	if err != nil {
		t.Fatal(err)
	}
	occs, err := reparsed.Events[0].Occurrences(
		time.Date(2026, 8, 20, 0, 0, 0, 0, ny), time.Date(2026, 11, 20, 0, 0, 0, 0, ny))
	if err != nil {
		t.Fatal(err)
	}
	if len(occs) == 0 {
		t.Fatal("no occurrences")
	}
	if !occs[0].Start.Equal(start) {
		t.Errorf("first occurrence = %v, want the event's own start %v", occs[0].Start, start)
	}
	for _, o := range occs {
		local := o.Start.In(ny)
		if local.Weekday() != time.Tuesday || local.Hour() != 20 {
			t.Errorf("occurrence %v is a %v at %02d:00, want Tuesday 20:00",
				local, local.Weekday(), local.Hour())
		}
	}
}

// The flip side of the branch order: when the rule IS being authored but the
// draft's own zone cannot be named, the existing foreign TZID is still preserved
// rather than flattened to UTC. Destroying the server's zone would buy nothing —
// a UTC anchor agrees with the new BY* no better than the Berlin one — and the
// iron rule says do not churn data we did not author.
func TestRuleAuthoringEditKeepsForeignZoneWhenDraftZoneIsUnnameable(t *testing.T) {
	berlin := mustZone(t, "Europe/Berlin")
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//foreign//EN\r\nBEGIN:VEVENT\r\n" +
		"UID:foreign-tz-2\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART;TZID=Europe/Berlin:20260826T020000\r\n" +
		"DTEND;TZID=Europe/Berlin:20260826T030000\r\nSUMMARY:old\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	obj, err := Decode([]byte(ics), berlin)
	if err != nil {
		t.Fatal(err)
	}
	// A draft carrying time.UTC: IsNamedZone rejects it, so branch 1 cannot fire.
	start := obj.Events[0].Start.UTC()
	edited, err := EditEvent(obj, "foreign-tz-2", EventDraft{
		Summary: "renamed", Start: start, End: start.Add(time.Hour),
		Recur: &RecurSpec{Freq: FreqWeekly},
	}, start, berlin)
	if err != nil {
		t.Fatal(err)
	}
	if got := propOf(t, edited, ical.PropDateTimeStart).Params.Get(ical.ParamTimezoneID); got != "Europe/Berlin" {
		t.Errorf("DTSTART TZID = %q, want the preserved Europe/Berlin", got)
	}
}

// A Windows/Outlook TZID must RESOLVE, not fall through to the UTC form.
//
// Outlook writes DTSTART;TZID=Eastern Standard Time:20260825T200000 with BYDAY=TU.
// time.LoadLocation cannot read that name, so before the windowsToIANA mapping the
// anchor was flattened to 20260826T000000Z — a Wednesday in UTC — while BYDAY=TU
// stayed put, silently walking every Outlook-authored series from Tuesday to
// Monday on a summary-only edit. This is the exact property
// TestEditWithoutRuleChangeKeepsAnchorForm guards, for the Windows spelling.
func TestWindowsTZIDIsResolvedNotFlattenedToUTC(t *testing.T) {
	ny := mustZone(t, "America/New_York")
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Microsoft Corporation//Outlook//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:outlook-1\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART;TZID=Eastern Standard Time:20260825T200000\r\n" +
		"DTEND;TZID=Eastern Standard Time:20260825T210000\r\n" +
		"RRULE:FREQ=WEEKLY;BYDAY=TU\r\nSUMMARY:old\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	obj, err := Decode([]byte(ics), ny)
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 8, 20, 0, 0, 0, 0, ny)
	to := time.Date(2026, 11, 20, 0, 0, 0, 0, ny)
	before := occurrenceStarts(t, obj.Events[0], ny, from, to)
	if len(before) == 0 {
		t.Fatal("fixture produced no occurrences")
	}

	// A summary-only edit, in the shape production builds it (readEventDraft
	// resolves the form's fields in a.loc).
	edited, err := EditEvent(obj, "outlook-1", EventDraft{
		Summary: "renamed",
		Start:   obj.Events[0].Start.In(ny),
		End:     obj.Events[0].End.In(ny),
	}, time.Date(2026, 8, 1, 0, 0, 0, 0, ny), ny)
	if err != nil {
		t.Fatal(err)
	}

	dtstart := propOf(t, edited, ical.PropDateTimeStart)
	if strings.HasSuffix(dtstart.Value, "Z") {
		t.Errorf("Windows TZID was flattened to the UTC form (%q); the rule's BYDAY now names a different day",
			dtstart.Value)
	}
	// The IANA spelling is the point: it is the same zone in the form every other
	// client — and ensureVTimezone — can resolve.
	if got := dtstart.Params.Get(ical.ParamTimezoneID); got != "America/New_York" {
		t.Errorf("DTSTART TZID = %q, want the resolved America/New_York", got)
	}
	if got := dtstart.Value; got != "20260825T200000" {
		t.Errorf("DTSTART = %q, want the unchanged wall clock 20260825T200000", got)
	}

	after := occurrenceStarts(t, edited.Events[0], ny, from, to)
	if len(before) != len(after) {
		t.Fatalf("occurrence count changed: %d → %d\nbefore %v\nafter  %v", len(before), len(after), before, after)
	}
	for i := range before {
		if before[i] != after[i] {
			t.Errorf("occurrence %d moved: %s → %s", i, before[i], after[i])
		}
	}

	// The resolved zone must now be defined in the object.
	var found bool
	for _, c := range edited.Calendar.Children {
		if c.Name == ical.CompTimezone && text(c.Props, ical.PropTimezoneID) == "America/New_York" {
			found = true
		}
	}
	if !found {
		t.Error("no VTIMEZONE for the resolved America/New_York")
	}
}

// An EXDATE written in TZID form must not reference a zone the object does not
// define — a foreign object can carry DTSTART;TZID=X with no VTIMEZONE.
func TestAddExceptionDefinesReferencedTimezone(t *testing.T) {
	ny := mustZone(t, "America/New_York")
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//foreign//EN\r\nBEGIN:VEVENT\r\n" +
		"UID:no-vtz-1\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART;TZID=America/New_York:20260825T200000\r\n" +
		"DTEND;TZID=America/New_York:20260825T210000\r\n" +
		"RRULE:FREQ=WEEKLY;BYDAY=TU\r\nSUMMARY:s\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	obj, err := Decode([]byte(ics), ny)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range obj.Calendar.Children {
		if c.Name == ical.CompTimezone {
			t.Fatal("fixture already defines a VTIMEZONE; the test would be vacuous")
		}
	}
	victim := time.Date(2026, 9, 1, 20, 0, 0, 0, ny)

	out, err := AddException(obj, "no-vtz-1", victim, false, victim, ny)
	if err != nil {
		t.Fatal(err)
	}
	assertDefinesZoneFor(t, out, ical.PropExceptionDates)
}

// The same for a RECURRENCE-ID, which inherits the master's TZID.
func TestAddOccurrenceOverrideDefinesReferencedTimezone(t *testing.T) {
	ny := mustZone(t, "America/New_York")
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//foreign//EN\r\nBEGIN:VEVENT\r\n" +
		"UID:no-vtz-2\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART;TZID=America/New_York:20260825T200000\r\n" +
		"DTEND;TZID=America/New_York:20260825T210000\r\n" +
		"RRULE:FREQ=WEEKLY;BYDAY=TU\r\nSUMMARY:s\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	obj, err := Decode([]byte(ics), ny)
	if err != nil {
		t.Fatal(err)
	}
	target := time.Date(2026, 9, 1, 20, 0, 0, 0, ny)

	out, err := AddOccurrenceOverride(obj, "no-vtz-2", target, false, func(c *ical.Component) {
		c.Props.SetText(ical.PropSummary, "moved")
	}, target, ny)
	if err != nil {
		t.Fatal(err)
	}
	assertDefinesZoneFor(t, out, ical.PropRecurrenceID)
}

// assertDefinesZoneFor checks that the TZID carried by prop `name` somewhere in
// obj is backed by a VTIMEZONE in the same object.
func assertDefinesZoneFor(t *testing.T, obj *Parsed, name string) {
	t.Helper()
	var tzid string
	defined := map[string]bool{}
	for _, c := range obj.Calendar.Children {
		if c.Name == ical.CompTimezone {
			defined[text(c.Props, ical.PropTimezoneID)] = true
			continue
		}
		for _, p := range c.Props.Values(name) {
			if v := p.Params.Get(ical.ParamTimezoneID); v != "" {
				tzid = v
			}
		}
	}
	if tzid == "" {
		t.Fatalf("%s carries no TZID; the test would be vacuous", name)
	}
	if !defined[tzid] {
		t.Errorf("%s references TZID %q but the object defines no VTIMEZONE for it", name, tzid)
	}
}
