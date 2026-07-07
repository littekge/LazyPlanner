package model_test

import (
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

func eventWithStart(tzidParam, value string) string {
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:tz@test\r\nDTSTAMP:20260701T120000Z\r\n" +
		"DTSTART" + tzidParam + ":" + value + "\r\n" +
		"SUMMARY:TZ test\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
}

func TestParseEventTimezones(t *testing.T) {
	tests := []struct {
		name        string
		tzidParam   string
		value       string
		wantUTCHr   int // expected hour after converting Start to UTC
		wantSurvive bool
	}{
		{
			// Windows/Outlook zone name — not IANA, must be mapped, not dropped.
			// 2026-07-04 13:00 America/New_York (EDT, UTC-4) = 17:00 UTC.
			name:        "windows zone name",
			tzidParam:   ";TZID=Eastern Standard Time",
			value:       "20260704T130000",
			wantUTCHr:   17,
			wantSurvive: true,
		},
		{
			// A real IANA TZID keeps working. Europe/Berlin summer (CEST, UTC+2):
			// 13:00 local = 11:00 UTC.
			name:        "iana zone name",
			tzidParam:   ";TZID=Europe/Berlin",
			value:       "20260704T130000",
			wantUTCHr:   11,
			wantSurvive: true,
		},
		{
			// Unknown/custom TZID: kept as floating (interpreted in loc=UTC here),
			// so 13:00 stays 13:00 UTC rather than the event vanishing.
			name:        "unresolvable zone falls back to floating",
			tzidParam:   ";TZID=Custom/Nonsense",
			value:       "20260704T130000",
			wantUTCHr:   13,
			wantSurvive: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := model.Decode([]byte(eventWithStart(tc.tzidParam, tc.value)), time.UTC)
			if err != nil {
				if tc.wantSurvive {
					t.Fatalf("event was dropped (Decode error): %v", err)
				}
				return
			}
			if len(parsed.Events) != 1 {
				t.Fatalf("got %d events, want 1", len(parsed.Events))
			}
			if got := parsed.Events[0].Start.UTC().Hour(); got != tc.wantUTCHr {
				t.Errorf("Start UTC hour = %d, want %d (Start=%s)", got, tc.wantUTCHr, parsed.Events[0].Start)
			}
		})
	}
}
