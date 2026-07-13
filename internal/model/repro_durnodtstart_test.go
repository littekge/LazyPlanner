package model

import (
	"testing"
	"time"
)

func TestReproVTodoDurationNoDTStart(t *testing.T) {
	data := "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//test//test//EN\r\n" +
		"BEGIN:VTODO\r\n" +
		"UID:t\r\n" +
		"DTSTAMP:20260101T090000Z\r\n" +
		"DURATION:PT1H\r\n" +
		"SUMMARY:x\r\n" +
		"END:VTODO\r\n" +
		"END:VCALENDAR\r\n"

	p, err := Decode([]byte(data), time.UTC)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if len(p.Todos) != 1 {
		t.Fatalf("want 1 todo, got %d", len(p.Todos))
	}
	if _, err := p.Encode(); err != nil {
		t.Fatalf("Encode failed after decode: %v", err)
	}
}
