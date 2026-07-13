package model

import (
	"strings"
	"testing"
	"time"
)

// Repro for HIGH finding: an empty VTIMEZONE (natural, or left empty after
// stripForbiddenNesting drops its only child) cannot be re-encoded, making the
// whole resource — including a sibling VEVENT — unwritable.
func TestReproEmptyVTimezoneBlocksEncode(t *testing.T) {
	cases := map[string]string{
		"caseA_natural_empty_vtimezone": "BEGIN:VCALENDAR\r\n" +
			"VERSION:2.0\r\n" +
			"PRODID:-//Test//Test//EN\r\n" +
			"BEGIN:VTIMEZONE\r\n" +
			"TZID:X\r\n" +
			"END:VTIMEZONE\r\n" +
			"BEGIN:VEVENT\r\n" +
			"UID:evt-1\r\n" +
			"DTSTAMP:20260101T000000Z\r\n" +
			"DTSTART:20260101T120000Z\r\n" +
			"SUMMARY:Hello\r\n" +
			"END:VEVENT\r\n" +
			"END:VCALENDAR\r\n",
		"caseB_heal_emptied_vtimezone": "BEGIN:VCALENDAR\r\n" +
			"VERSION:2.0\r\n" +
			"PRODID:-//Test//Test//EN\r\n" +
			"BEGIN:VTIMEZONE\r\n" +
			"TZID:Y\r\n" +
			"BEGIN:VEVENT\r\n" +
			"UID:nested-forbidden\r\n" +
			"DTSTAMP:20260101T000000Z\r\n" +
			"DTSTART:20260101T120000Z\r\n" +
			"END:VEVENT\r\n" +
			"END:VTIMEZONE\r\n" +
			"BEGIN:VEVENT\r\n" +
			"UID:evt-2\r\n" +
			"DTSTAMP:20260101T000000Z\r\n" +
			"DTSTART:20260101T120000Z\r\n" +
			"SUMMARY:Sibling\r\n" +
			"END:VEVENT\r\n" +
			"END:VCALENDAR\r\n",
	}

	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			p, err := Decode([]byte(raw), time.UTC)
			if err != nil {
				t.Fatalf("Decode failed (expected heal, not decode error): %v", err)
			}
			out, err := p.Encode()
			if err != nil {
				t.Fatalf("Encode failed — resource unwritable: %v", err)
			}
			if !strings.Contains(string(out), "VEVENT") {
				t.Fatalf("re-encoded resource lost its VEVENT:\n%s", out)
			}
		})
	}
}
