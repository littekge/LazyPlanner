package model

import (
	"strings"
	"testing"
	"time"
)

// TestMalformedVTimezoneDroppedKeepsResourceWritable guards the fix for the HIGH:
// a VTIMEZONE whose STANDARD/DAYLIGHT block is missing a required property
// (TZOFFSETFROM/TZOFFSETTO/DTSTART), or a VTIMEZONE lacking TZID, decodes cleanly
// but go-ical refuses to encode it — which used to make the whole resource
// (including a valid sibling VEVENT) unwritable on the first edit.
// dropUnusableTimezones now strips such an unencodable VTIMEZONE on ingest (it
// carries no usable offset data; a referenced TZID degrades to floating time), so
// the resource re-encodes and the VEVENT survives.
func TestMalformedVTimezoneDroppedKeepsResourceWritable(t *testing.T) {
	cases := map[string]string{
		"standard_missing_tzoffsetfrom": "BEGIN:VCALENDAR\r\n" +
			"VERSION:2.0\r\n" +
			"PRODID:-//Test//Test//EN\r\n" +
			"BEGIN:VTIMEZONE\r\n" +
			"TZID:Custom/Zone\r\n" +
			"BEGIN:STANDARD\r\n" +
			"DTSTART:19701025T030000\r\n" +
			"TZOFFSETTO:+0100\r\n" +
			"END:STANDARD\r\n" +
			"END:VTIMEZONE\r\n" +
			"BEGIN:VEVENT\r\n" +
			"UID:evt-1\r\n" +
			"DTSTAMP:20260101T000000Z\r\n" +
			"DTSTART:20260101T120000Z\r\n" +
			"SUMMARY:Hello\r\n" +
			"END:VEVENT\r\n" +
			"END:VCALENDAR\r\n",
		"standard_missing_dtstart": "BEGIN:VCALENDAR\r\n" +
			"VERSION:2.0\r\n" +
			"PRODID:-//Test//Test//EN\r\n" +
			"BEGIN:VTIMEZONE\r\n" +
			"TZID:Custom/Zone\r\n" +
			"BEGIN:STANDARD\r\n" +
			"TZOFFSETFROM:+0200\r\n" +
			"TZOFFSETTO:+0100\r\n" +
			"END:STANDARD\r\n" +
			"END:VTIMEZONE\r\n" +
			"BEGIN:VEVENT\r\n" +
			"UID:evt-2\r\n" +
			"DTSTAMP:20260101T000000Z\r\n" +
			"DTSTART:20260101T120000Z\r\n" +
			"SUMMARY:Hello\r\n" +
			"END:VEVENT\r\n" +
			"END:VCALENDAR\r\n",
		"vtimezone_missing_tzid": "BEGIN:VCALENDAR\r\n" +
			"VERSION:2.0\r\n" +
			"PRODID:-//Test//Test//EN\r\n" +
			"BEGIN:VTIMEZONE\r\n" +
			"BEGIN:STANDARD\r\n" +
			"DTSTART:19701025T030000\r\n" +
			"TZOFFSETFROM:+0200\r\n" +
			"TZOFFSETTO:+0100\r\n" +
			"END:STANDARD\r\n" +
			"END:VTIMEZONE\r\n" +
			"BEGIN:VEVENT\r\n" +
			"UID:evt-3\r\n" +
			"DTSTAMP:20260101T000000Z\r\n" +
			"DTSTART:20260101T120000Z\r\n" +
			"SUMMARY:Hello\r\n" +
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
