package model_test

import (
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// TestUnknownContainerBricksResource guards the deny-by-default rule in
// stripForbiddenChildren — the fix for the FOURTH reopening of the heal-set
// class (pass 23). go-ical's encodeComponent recurses into EVERY child regardless
// of name and runs checkComponent on it, while allowedChildren can only ever list
// the containers we know about. When the map was consulted allow-by-default, a
// phantom VEVENT/VTODO wrapped in an unknown container (X-CUSTOM-CONTAINER,
// VAVAILABILITY, a nested VCALENDAR) survived the heal, was never surfaced by
// Parse (which walks only the calendar's direct children), and made every later
// Encode fail — bricking the valid sibling item for edit, complete, grab and push.
func TestUnknownContainerBricksResource(t *testing.T) {
	const head = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Foreign//EN\r\n"
	const tail = "END:VCALENDAR\r\n"
	const realEvent = "BEGIN:VEVENT\r\nUID:real-event\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART:20260101T090000Z\r\nSUMMARY:Real event\r\nEND:VEVENT\r\n"

	cases := []struct {
		name string
		ics  string
	}{
		{
			name: "phantom VEVENT under X-CUSTOM-CONTAINER missing DTSTAMP",
			ics: head + realEvent +
				"BEGIN:X-CUSTOM-CONTAINER\r\nX-FOO:bar\r\n" +
				"BEGIN:VEVENT\r\nUID:phantom\r\nDTSTART:20260101T080000Z\r\nSUMMARY:Phantom\r\n" +
				"END:VEVENT\r\nEND:X-CUSTOM-CONTAINER\r\n" + tail,
		},
		{
			name: "phantom VTODO under VAVAILABILITY missing DTSTAMP",
			ics: head + realEvent +
				"BEGIN:VAVAILABILITY\r\nUID:avail-1\r\n" +
				"BEGIN:VTODO\r\nUID:phantom-todo\r\nSUMMARY:Phantom todo\r\n" +
				"END:VTODO\r\nEND:VAVAILABILITY\r\n" + tail,
		},
		{
			name: "empty nested VCALENDAR",
			ics: head + realEvent +
				"BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Nested//EN\r\n" +
				"END:VCALENDAR\r\n" + tail,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := model.Decode([]byte(tc.ics), time.UTC)
			if err != nil {
				t.Fatalf("Decode should succeed and surface the real item, got: %v", err)
			}
			if len(p.Events) == 0 {
				t.Fatalf("expected the valid sibling VEVENT to be surfaced")
			}
			if _, err := p.Encode(); err != nil {
				t.Fatalf("re-encode bricked the whole resource (real item now unwritable): %v", err)
			}
		})
	}
}

// TestUnknownContainerKeepsUnvalidatedChildren is the other half of the
// deny-by-default rule: it must strip only what can actually brick the resource.
// go-ical's checkComponent has no default case, so a component type it has no
// switch case for can never fail an encode — stripping it would destroy user data
// for nothing. A real RFC 7953 VAVAILABILITY and its AVAILABLE sub-components are
// exactly that shape, and must survive ingest untouched (the iron rule).
func TestUnknownContainerKeepsUnvalidatedChildren(t *testing.T) {
	const ics = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Foreign//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:real-event\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART:20260101T090000Z\r\nSUMMARY:Real event\r\nEND:VEVENT\r\n" +
		"BEGIN:VAVAILABILITY\r\nUID:avail-1\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART:20260101T000000Z\r\nBUSYTYPE:BUSY-UNAVAILABLE\r\n" +
		"BEGIN:AVAILABLE\r\nUID:avail-1-mon\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART:20260105T090000Z\r\nDTEND:20260105T170000Z\r\n" +
		"RRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR\r\nSUMMARY:Office hours\r\n" +
		"END:AVAILABLE\r\nEND:VAVAILABILITY\r\n" +
		"BEGIN:X-VENDOR-BLOB\r\nX-PAYLOAD:opaque\r\n" +
		"BEGIN:X-VENDOR-PART\r\nX-BIT:1\r\nEND:X-VENDOR-PART\r\nEND:X-VENDOR-BLOB\r\n" +
		"END:VCALENDAR\r\n"

	p, err := model.Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	out, err := p.Encode()
	if err != nil {
		t.Fatalf("re-encode failed: %v", err)
	}

	// The AVAILABLE sub-component and the nested vendor part are the load-bearing
	// assertions: both are children of a container with no allowedChildren entry.
	for _, want := range []string{
		"BEGIN:VAVAILABILITY", "BUSYTYPE:BUSY-UNAVAILABLE",
		"BEGIN:AVAILABLE", "UID:avail-1-mon", "RRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR",
		"BEGIN:X-VENDOR-BLOB", "X-PAYLOAD:opaque",
		"BEGIN:X-VENDOR-PART", "X-BIT:1",
		"UID:real-event",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("ingest dropped %q from an unknown container go-ical never validates:\n%s", want, out)
		}
	}
}
