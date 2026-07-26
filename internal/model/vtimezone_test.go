package model

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

func TestIsNamedZone(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	for _, tc := range []struct {
		loc  *time.Location
		want bool
	}{
		{ny, true},
		{time.UTC, false},                       // UTC needs no TZID; Z is the correct form
		{time.FixedZone("EDT", -4*3600), false}, // an offset is not a zone identity
		{nil, false},
	} {
		if got := IsNamedZone(tc.loc); got != tc.want {
			t.Errorf("IsNamedZone(%v) = %v, want %v", tc.loc, got, tc.want)
		}
	}
}

// A DST zone gets both observances, each carrying the props timezoneUsable (and
// therefore go-ical's encoder) requires.
func TestBuildVTimezoneDSTZone(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	comp := BuildVTimezone(ny, time.Date(2026, 8, 25, 0, 0, 0, 0, ny))
	if comp == nil {
		t.Fatal("BuildVTimezone returned nil for a named DST zone")
	}
	if got := comp.Props.Get("TZID").Value; got != "America/New_York" {
		t.Errorf("TZID = %q", got)
	}
	var std, day *ical.Component
	for _, sub := range comp.Children {
		switch sub.Name {
		case ical.CompTimezoneStandard:
			std = sub
		case ical.CompTimezoneDaylight:
			day = sub
		}
	}
	if std == nil || day == nil {
		t.Fatalf("want both STANDARD and DAYLIGHT, got %d children", len(comp.Children))
	}
	for name, sub := range map[string]*ical.Component{"STANDARD": std, "DAYLIGHT": day} {
		for _, p := range []string{"DTSTART", "TZOFFSETFROM", "TZOFFSETTO"} {
			if sub.Props.Get(p) == nil {
				t.Errorf("%s missing %s", name, p)
			}
		}
	}
	if got := day.Props.Get("TZOFFSETTO").Value; got != "-0400" {
		t.Errorf("DAYLIGHT TZOFFSETTO = %q, want -0400", got)
	}
	if got := std.Props.Get("TZOFFSETTO").Value; got != "-0500" {
		t.Errorf("STANDARD TZOFFSETTO = %q, want -0500", got)
	}
	if r := day.Props.Get("RRULE"); r == nil || !strings.Contains(r.Value, "BYMONTH=3") {
		t.Errorf("DAYLIGHT RRULE = %v, want a March yearly rule", r)
	}
}

// A zone with no DST gets a single STANDARD observance and no RRULE.
func TestBuildVTimezoneFixedZone(t *testing.T) {
	kol, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Skip(err)
	}
	comp := BuildVTimezone(kol, time.Date(2026, 8, 25, 0, 0, 0, 0, kol))
	if comp == nil {
		t.Fatal("nil for Asia/Kolkata")
	}
	if len(comp.Children) != 1 || comp.Children[0].Name != ical.CompTimezoneStandard {
		t.Fatalf("want one STANDARD, got %v", comp.Children)
	}
	if got := comp.Children[0].Props.Get("TZOFFSETTO").Value; got != "+0530" {
		t.Errorf("TZOFFSETTO = %q, want +0530", got)
	}
}

// Southern hemisphere: DST starts in the second half of the year.
func TestBuildVTimezoneSouthernHemisphere(t *testing.T) {
	syd, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		t.Skip(err)
	}
	comp := BuildVTimezone(syd, time.Date(2026, 8, 25, 0, 0, 0, 0, syd))
	if comp == nil {
		t.Fatal("nil for Australia/Sydney")
	}
	if len(comp.Children) != 2 {
		t.Fatalf("want two observances, got %d", len(comp.Children))
	}
}

// The generated component must survive our own ingest heal, which drops any
// VTIMEZONE go-ical's encoder would reject.
func TestBuildVTimezoneIsUsable(t *testing.T) {
	for _, name := range []string{"America/New_York", "Europe/Berlin", "Asia/Kolkata", "Australia/Sydney"} {
		loc, err := time.LoadLocation(name)
		if err != nil {
			t.Skip(err)
		}
		comp := BuildVTimezone(loc, time.Date(2026, 8, 25, 0, 0, 0, 0, loc))
		if comp == nil || !timezoneUsable(comp) {
			t.Errorf("%s: generated VTIMEZONE is not usable", name)
		}
	}
}

// The typed values must not be written through Props.SetText: that stamps
// VALUE=TEXT and backslash-escapes the ';' separators, which would emit
// "RRULE;VALUE=TEXT:FREQ=YEARLY\;BYMONTH=3" — a line no other client can read.
// Encoding is the only check that sees the bytes actually put on the wire.
func TestBuildVTimezoneEncodesConformantLines(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	comp := BuildVTimezone(ny, time.Date(2026, 8, 25, 0, 0, 0, 0, ny))
	if comp == nil {
		t.Fatal("BuildVTimezone returned nil for a named DST zone")
	}

	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropProductID, "-//LazyPlanner//test//EN")
	cal.Props.SetText(ical.PropVersion, "2.0")
	cal.Children = append(cal.Children, comp)

	var buf strings.Builder
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		t.Fatalf("encode: %v", err)
	}
	out := strings.ReplaceAll(buf.String(), "\r\n", "\n")

	for _, want := range []string{
		"TZID:America/New_York",
		"DTSTART:20280312T020000",
		"TZOFFSETFROM:-0500",
		"TZOFFSETTO:-0400",
		"RRULE:FREQ=YEARLY;BYMONTH=3;BYDAY=2SU",
		"RRULE:FREQ=YEARLY;BYMONTH=11;BYDAY=1SU",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("encoded VTIMEZONE missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "VALUE=TEXT") {
		t.Errorf("encoded VTIMEZONE stamps VALUE=TEXT on a typed property:\n%s", out)
	}
	if strings.Contains(out, `\;`) {
		t.Errorf("encoded VTIMEZONE escapes a RECUR separator:\n%s", out)
	}
}

// The observance is anchored on the transition itself, rendered in the wall clock
// of the offset being switched FROM (RFC 5545 §3.6.5) — 02:00 for both US
// transitions, which is the shape other clients emit and are tested against.
func TestBuildVTimezoneObservanceAnchors(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	comp := BuildVTimezone(ny, time.Date(2026, 8, 25, 0, 0, 0, 0, ny))
	if comp == nil {
		t.Fatal("BuildVTimezone returned nil for a named DST zone")
	}
	want := map[string]struct{ dtstart, from, to, rrule string }{
		// The probe window ends two years after the anchor, so the most recent
		// transition of each kind is the 2028 spring-forward and 2027 fall-back.
		ical.CompTimezoneDaylight: {"20280312T020000", "-0500", "-0400", "FREQ=YEARLY;BYMONTH=3;BYDAY=2SU"},
		ical.CompTimezoneStandard: {"20271107T020000", "-0400", "-0500", "FREQ=YEARLY;BYMONTH=11;BYDAY=1SU"},
	}
	for _, sub := range comp.Children {
		w, ok := want[sub.Name]
		if !ok {
			t.Fatalf("unexpected observance %q", sub.Name)
		}
		for _, tc := range []struct{ prop, want string }{
			{ical.PropDateTimeStart, w.dtstart},
			{ical.PropTimezoneOffsetFrom, w.from},
			{ical.PropTimezoneOffsetTo, w.to},
			{ical.PropRecurrenceRule, w.rrule},
		} {
			p := sub.Props.Get(tc.prop)
			if p == nil {
				t.Errorf("%s missing %s", sub.Name, tc.prop)
				continue
			}
			if p.Value != tc.want {
				t.Errorf("%s %s = %q, want %q", sub.Name, tc.prop, p.Value, tc.want)
			}
		}
	}
}

// The real invariant behind the DTSTART/TZOFFSETFROM pairing: reading DTSTART at
// TZOFFSETFROM, the way a conforming client does, must land on the exact instant the
// zone's offset flips from TZOFFSETFROM to TZOFFSETTO. Rendering DTSTART in the
// wrong offset passes every presence check and still misplaces the onset by an hour.
func TestBuildVTimezoneObservanceOnsetIsTheRealTransition(t *testing.T) {
	for _, name := range []string{"America/New_York", "Europe/Berlin", "Australia/Sydney"} {
		loc, err := time.LoadLocation(name)
		if err != nil {
			t.Skip(err)
		}
		comp := BuildVTimezone(loc, time.Date(2026, 8, 25, 0, 0, 0, 0, loc))
		if comp == nil {
			t.Fatalf("nil for %s", name)
		}
		for _, sub := range comp.Children {
			offFrom := parseICalUTCOffset(t, sub.Props.Get(ical.PropTimezoneOffsetFrom).Value)
			offTo := parseICalUTCOffset(t, sub.Props.Get(ical.PropTimezoneOffsetTo).Value)
			onset, err := time.ParseInLocation(vtimezoneDateTimeLayout,
				sub.Props.Get(ical.PropDateTimeStart).Value, time.FixedZone("", offFrom))
			if err != nil {
				t.Fatalf("%s %s: parsing DTSTART: %v", name, sub.Name, err)
			}
			if _, got := onset.Add(-time.Second).In(loc).Zone(); got != offFrom {
				t.Errorf("%s %s: offset one second before onset = %d, want TZOFFSETFROM %d",
					name, sub.Name, got, offFrom)
			}
			if _, got := onset.In(loc).Zone(); got != offTo {
				t.Errorf("%s %s: offset at onset = %d, want TZOFFSETTO %d",
					name, sub.Name, got, offTo)
			}
		}
	}
}

func parseICalUTCOffset(t *testing.T, s string) int {
	t.Helper()
	var sign, hh, mm int
	if _, err := fmt.Sscanf(s[1:], "%2d%2d", &hh, &mm); err != nil {
		t.Fatalf("parsing UTC offset %q: %v", s, err)
	}
	switch s[0] {
	case '+':
		sign = 1
	case '-':
		sign = -1
	default:
		t.Fatalf("UTC offset %q has no sign", s)
	}
	return sign * (hh*3600 + mm*60)
}

// A "last Sunday" zone rule must render as BYDAY=-1SU, not as a fixed ordinal
// that drifts a week whenever the month has five of that weekday.
func TestBuildVTimezoneLastWeekdayRule(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip(err)
	}
	comp := BuildVTimezone(berlin, time.Date(2026, 8, 25, 0, 0, 0, 0, berlin))
	if comp == nil {
		t.Fatal("nil for Europe/Berlin")
	}
	for _, sub := range comp.Children {
		r := sub.Props.Get(ical.PropRecurrenceRule)
		if r == nil {
			t.Fatalf("%s missing RRULE", sub.Name)
		}
		if !strings.HasSuffix(r.Value, "BYDAY=-1SU") {
			t.Errorf("%s RRULE = %q, want a last-Sunday rule", sub.Name, r.Value)
		}
	}
}
