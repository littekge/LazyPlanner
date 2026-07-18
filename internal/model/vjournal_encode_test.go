package model

import (
	"strings"
	"testing"
	"time"
)

// TestHealVJournalMissingDTSTAMP guards the fix for the decode-but-unencodable
// HIGH: a resource with a valid VEVENT plus a sibling VJOURNAL that omits DTSTAMP
// decodes fine but must still re-encode — otherwise the whole resource (incl. the
// valid VEVENT) is unwritable. healComponentConstraints now DTSTAMP-heals
// VJOURNAL/VFREEBUSY the same way Parse does VEVENT/VTODO.
func TestHealVJournalMissingDTSTAMP(t *testing.T) {
	const src = "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//Test//EN\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:event-1\r\n" +
		"DTSTAMP:20260101T000000Z\r\n" +
		"DTSTART:20260101T090000Z\r\n" +
		"SUMMARY:A real meeting\r\n" +
		"END:VEVENT\r\n" +
		"BEGIN:VJOURNAL\r\n" +
		"UID:journal-1\r\n" +
		"SUMMARY:Hand-written note with no DTSTAMP\r\n" +
		"END:VJOURNAL\r\n" +
		"END:VCALENDAR\r\n"

	p, err := Decode([]byte(src), time.UTC)
	if err != nil {
		t.Fatalf("Decode failed (should succeed): %v", err)
	}
	if len(p.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(p.Events))
	}

	// Editing the VEVENT re-encodes the whole resource.
	if _, err := p.Encode(); err != nil {
		t.Fatalf("Encode failed after decoding a resource with a DTSTAMP-less VJOURNAL: %v", err)
	}
}

// TestHealVJournalDuplicateSingleValued guards the duplicate-single-valued
// variant: a VJOURNAL (or VFREEBUSY) carrying a duplicate single-valued property
// go-ical's encoder rejects — now deduped because singleValuedProps has a table
// entry for those component types.
func TestHealVJournalDuplicateSingleValued(t *testing.T) {
	const src = "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//Test//EN\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:event-1\r\n" +
		"DTSTAMP:20260101T000000Z\r\n" +
		"DTSTART:20260101T090000Z\r\n" +
		"SUMMARY:A real meeting\r\n" +
		"END:VEVENT\r\n" +
		"BEGIN:VJOURNAL\r\n" +
		"UID:journal-1\r\n" +
		"DTSTAMP:20260101T000000Z\r\n" +
		"SUMMARY:first\r\n" +
		"SUMMARY:second\r\n" +
		"END:VJOURNAL\r\n" +
		"END:VCALENDAR\r\n"

	p, err := Decode([]byte(src), time.UTC)
	if err != nil {
		t.Fatalf("Decode failed (should succeed): %v", err)
	}
	if _, err := p.Encode(); err != nil {
		if strings.Contains(err.Error(), "SUMMARY") {
			t.Fatalf("Encode failed on duplicate VJOURNAL SUMMARY: %v", err)
		}
		t.Fatalf("Encode failed: %v", err)
	}
}

// TestHealVFreeBusyMissingDTSTAMP exercises the VFREEBUSY branch of the same heal:
// a DTSTAMP-less VFREEBUSY sibling must not brick the resource on re-encode.
func TestHealVFreeBusyMissingDTSTAMP(t *testing.T) {
	const src = "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//Test//EN\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:event-1\r\n" +
		"DTSTAMP:20260101T000000Z\r\n" +
		"DTSTART:20260101T090000Z\r\n" +
		"SUMMARY:A real meeting\r\n" +
		"END:VEVENT\r\n" +
		"BEGIN:VFREEBUSY\r\n" +
		"UID:fb-1\r\n" +
		"END:VFREEBUSY\r\n" +
		"END:VCALENDAR\r\n"

	p, err := Decode([]byte(src), time.UTC)
	if err != nil {
		t.Fatalf("Decode failed (should succeed): %v", err)
	}
	if _, err := p.Encode(); err != nil {
		t.Fatalf("Encode failed after decoding a resource with a DTSTAMP-less VFREEBUSY: %v", err)
	}
}
