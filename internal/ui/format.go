package ui

import "time"

// Time/date formatting honors the [appearance] config (time_format, date_format).
// These helpers take the resolved preference as a bool so both app methods and
// the custom widgets (which hold their own clock24 flag) can share them.

// clockStr formats a time of day: "3:04pm" (12h) or "15:04" (24h).
func clockStr(t time.Time, use24 bool) string {
	if use24 {
		return t.Format("15:04")
	}
	return t.Format("3:04pm")
}

// hourAxisLabel formats a whole-hour axis/cell label: "3pm" (12h) or "15" (24h).
func hourAxisLabel(hour int, use24 bool) string {
	tt := time.Date(2000, 1, 1, hour, 0, 0, 0, time.UTC)
	if use24 {
		return tt.Format("15")
	}
	return tt.Format("3pm")
}

// dateStr formats a full numeric date: "01/02/2006" (US) or "2006-01-02" (ISO).
func dateStr(t time.Time, iso bool) string {
	if iso {
		return t.Format("2006-01-02")
	}
	return t.Format("01/02/2006")
}

// dateShortStr formats a numeric date without the year: "01/02" (US) or "01-02"
// (ISO).
func dateShortStr(t time.Time, iso bool) string {
	if iso {
		return t.Format("01-02")
	}
	return t.Format("01/02")
}

// parseWeekStartMonday resolves the first_day_of_week config to a bool (default
// Monday when unset/unknown).
func parseWeekStartMonday(s string) bool { return s != "sunday" }

// parseDefaultView resolves the default_view config to a viewMode (default month).
func parseDefaultView(s string) int {
	switch s {
	case "week":
		return viewWeek
	case "day":
		return viewDay
	default:
		return viewMonth
	}
}
