package model

import (
	"strings"
	"testing"
	"time"
)

// TestHealDTEndAndDuration guards pass-10 HIGH #1: a VEVENT with both DTEND and
// DURATION decodes but go-ical refuses to re-encode it ("only one of DTEND and
// DURATION"). The ingest heal drops the redundant DURATION, keeping DTEND.
func TestHealDTEndAndDuration(t *testing.T) {
	data := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:e\r\nDTSTAMP:20260101T090000Z\r\n" +
		"DTSTART:20260101T090000Z\r\nDTEND:20260101T100000Z\r\nDURATION:PT2H\r\n" +
		"SUMMARY:x\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	p, err := Decode([]byte(data), time.UTC)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, err := p.Encode(); err != nil {
		t.Fatalf("encode after heal failed: %v", err)
	}
	// DTEND (the parser's End) is kept; DURATION is the value dropped.
	if p.Events[0].End.IsZero() {
		t.Error("heal dropped DTEND instead of the redundant DURATION")
	}
}

// TestHealVJournalNesting guards pass-10 MED #7: a VJOURNAL/VFREEBUSY carrying a
// nested component decodes but cannot re-encode ("nested components are
// forbidden"), poisoning a shared resource. The heal strips the nesting so a
// sibling VEVENT still re-encodes.
func TestHealVJournalNesting(t *testing.T) {
	data := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\n" +
		"BEGIN:VJOURNAL\r\nUID:j\r\nDTSTAMP:20260101T090000Z\r\n" +
		"BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT5M\r\nEND:VALARM\r\n" +
		"END:VJOURNAL\r\n" +
		"BEGIN:VEVENT\r\nUID:e\r\nDTSTAMP:20260101T090000Z\r\nDTSTART:20260101T090000Z\r\n" +
		"SUMMARY:keep\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	p, err := Decode([]byte(data), time.UTC)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	out, err := p.Encode()
	if err != nil {
		t.Fatalf("encode after heal failed: %v", err)
	}
	if !strings.Contains(string(out), "SUMMARY:keep") {
		t.Errorf("sibling VEVENT lost after heal:\n%s", out)
	}
}
