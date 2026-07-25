package model_test

import (
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// TestNestedComponentBricksResource is the Pass-21 HIGH regression guard (THIRD
// reopening of "the heal set must mirror go-ical's FULL validateComponent" — now
// at recursion depth). go-ical's encoder validates every component at every
// depth, but the required-prop/mutual-exclusion heals ran only on top-level
// components and allowedChildren had no entry for VALARM/STANDARD/DAYLIGHT — so a
// foreign object with a phantom component illegally nested under one of those
// decoded and surfaced its real top-level item, then bricked on re-encode the
// moment the user edited that item. Every case must Decode AND round-trip Encode.
func TestNestedComponentBricksResource(t *testing.T) {
	const head = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Foreign//EN\r\n"
	const tail = "END:VCALENDAR\r\n"

	cases := []struct {
		name string
		ics  string
	}{
		{
			// The confirmed repro: a VEVENT nested inside a VALARM with no DTSTAMP.
			name: "phantom VEVENT under VALARM missing DTSTAMP",
			ics: head +
				"BEGIN:VEVENT\r\nUID:real-event\r\nDTSTAMP:20260101T000000Z\r\n" +
				"DTSTART:20260101T090000Z\r\nSUMMARY:Real event\r\n" +
				"BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT15M\r\n" +
				"BEGIN:VEVENT\r\nUID:phantom\r\nDTSTART:20260101T080000Z\r\nSUMMARY:Phantom\r\n" +
				"END:VEVENT\r\nEND:VALARM\r\nEND:VEVENT\r\n" + tail,
		},
		{
			// Same class via a phantom VTODO nested under a STANDARD in a VTIMEZONE.
			name: "phantom VTODO under STANDARD missing DTSTAMP",
			ics: head +
				"BEGIN:VTIMEZONE\r\nTZID:Custom/Zone\r\n" +
				"BEGIN:STANDARD\r\nDTSTART:19701101T020000\r\nTZOFFSETFROM:-0400\r\nTZOFFSETTO:-0500\r\n" +
				"BEGIN:VTODO\r\nUID:phantom-todo\r\nSUMMARY:Phantom todo\r\n" +
				"END:VTODO\r\nEND:STANDARD\r\nEND:VTIMEZONE\r\n" +
				"BEGIN:VEVENT\r\nUID:real-event\r\nDTSTAMP:20260101T000000Z\r\n" +
				"DTSTART:20260101T090000Z\r\nSUMMARY:Real event\r\nEND:VEVENT\r\n" + tail,
		},
		{
			// A nested component that decodes fine but violates a mutual-exclusion
			// rule (DTEND+DURATION) — another recursive checkComponent failure.
			name: "phantom VEVENT under VALARM with DTEND+DURATION",
			ics: head +
				"BEGIN:VEVENT\r\nUID:real-event\r\nDTSTAMP:20260101T000000Z\r\n" +
				"DTSTART:20260101T090000Z\r\nSUMMARY:Real event\r\n" +
				"BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT15M\r\n" +
				"BEGIN:VEVENT\r\nUID:phantom\r\nDTSTAMP:20260101T000000Z\r\nDTSTART:20260101T080000Z\r\n" +
				"DTEND:20260101T083000Z\r\nDURATION:PT30M\r\nSUMMARY:Phantom\r\n" +
				"END:VEVENT\r\nEND:VALARM\r\nEND:VEVENT\r\n" + tail,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := model.Decode([]byte(tc.ics), time.UTC)
			if err != nil {
				t.Fatalf("Decode should succeed and surface the real item, got: %v", err)
			}
			if _, err := p.Encode(); err != nil {
				t.Fatalf("re-encode bricked the whole resource (real item now unwritable): %v", err)
			}
		})
	}
}

// TestLegitAlarmPropsSurviveNestedStrip pins that healing the nesting gap only
// strips illegally-nested *components*, never the VALARM's own properties: a real
// reminder (ACTION/TRIGGER) must round-trip intact.
func TestLegitAlarmPropsSurviveNestedStrip(t *testing.T) {
	const ics = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Foreign//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:real-event\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART:20260101T090000Z\r\nSUMMARY:Real event\r\n" +
		"BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT15M\r\nDESCRIPTION:Reminder\r\n" +
		"END:VALARM\r\nEND:VEVENT\r\n" +
		"END:VCALENDAR\r\n"

	p, err := model.Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	out, err := p.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	for _, want := range []string{"BEGIN:VALARM", "ACTION:DISPLAY", "TRIGGER:-PT15M", "DESCRIPTION:Reminder"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("legit VALARM property dropped: encoded output missing %q\n%s", want, out)
		}
	}
}
