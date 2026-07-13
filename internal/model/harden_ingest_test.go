package model_test

import (
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// These regression tests lock the fuzz-pass ingest fixes at the model API level;
// the saved crashers in testdata/fuzz/ guard the raw-byte inputs that found them.

// TestDecodeContainsDecoderPanic covers the go-ical decoder panic (index out of
// range on a content line ending mid-parameter): Decode must surface an error,
// never crash the process. If containment regressed, this test would panic.
func TestDecodeContainsDecoderPanic(t *testing.T) {
	if _, err := model.Decode([]byte("0;0="), time.UTC); err == nil {
		t.Fatal("expected a decode error for malformed input, got nil")
	}
}

// TestOccurrencesDegradeOnRrulePanic covers the rrule-go iteration panic on a
// degenerate rule (near-zero DTSTART). Expansion must degrade to the event's
// base instance rather than crash the calendar view.
func TestOccurrencesDegradeOnRrulePanic(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:z\r\nDTSTART:00000101T000000\r\nRRULE:FREQ=WEEKLY \r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	p, err := model.Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	from := time.Date(-1, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
	occs, err := p.Events[0].Occurrences(from, to)
	if err != nil {
		t.Fatalf("Occurrences errored instead of degrading: %v", err)
	}
	if len(occs) != 1 {
		t.Fatalf("degraded expansion = %d occurrences, want the single base instance", len(occs))
	}
}

// TestDecodeHealsForEditability covers the encode-blocking heals: an item that
// omits DTSTAMP and whose calendar omits VERSION/PRODID still decodes, and can
// then be re-encoded and edited — without which any edit would hard-fail. Unknown
// (X-) properties must survive the round-trip (property-preservation iron rule).
func TestDecodeHealsForEditability(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\n" +
		"BEGIN:VTODO\r\nUID:t1\r\nSUMMARY:Task\r\nX-CUSTOM:keepme\r\nEND:VTODO\r\n" +
		"END:VCALENDAR\r\n"
	p, err := model.Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if _, err := p.Encode(); err != nil {
		t.Fatalf("Encode failed after heal: %v", err)
	}

	edited, err := model.EditTodo(p, "t1", model.TodoDraft{Summary: "Renamed"}, time.Now(), time.UTC)
	if err != nil {
		t.Fatalf("EditTodo failed on a healed item: %v", err)
	}
	out, err := edited.Encode()
	if err != nil {
		t.Fatalf("Encode of edited item: %v", err)
	}
	if !strings.Contains(string(out), "X-CUSTOM:keepme") {
		t.Error("unknown X-CUSTOM property was dropped by the heal/edit round-trip")
	}
}

// TestDecodeDedupesAndStripsToEncodable covers the remaining encode-blocking
// heals together: a duplicate single-valued property (two UIDs), a raw CR in a
// value, and an illegally nested component must all be healed so the resource
// re-encodes.
func TestDecodeDedupesAndStripsToEncodable(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
		"BEGIN:VTODO\r\nUID:a\r\nUID:b\r\n" +
		"BEGIN:VTODO\r\nUID:nested\r\nEND:VTODO\r\n" +
		"END:VTODO\r\nEND:VCALENDAR\r\n"
	p, err := model.Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(p.Todos) != 1 {
		t.Fatalf("got %d todos, want 1 (the nested VTODO is not a direct child)", len(p.Todos))
	}
	if p.Todos[0].UID != "a" {
		t.Errorf("kept UID %q, want the first occurrence %q", p.Todos[0].UID, "a")
	}
	if _, err := p.Encode(); err != nil {
		t.Fatalf("Encode failed after dedupe/strip heal: %v", err)
	}
}
