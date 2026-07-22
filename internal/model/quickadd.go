package model

import (
	"strconv"
	"strings"
	"time"
)

// QuickAdd is the result of parsing a one-line quick-add entry. The parser is
// deliberately conservative and predictable: a token becomes a date, time,
// priority, or tag only when it clearly matches one of the documented forms;
// anything else stays in Title (see main.md "when in doubt, leave text in the
// title rather than guess").
//
//   - Dates:    today/tod, tomorrow/tom/tmr, weekday names (mon..sunday),
//     "jul 20" / "july 20", "7/20", "7/20/2026", "2026-07-20".
//   - Times:    require am/pm or a colon — "3pm", "3:30pm", "15:00". A bare
//     number is never a time (so "jul 20" keeps 20 as the day).
//   - Priority: "!1".."!9", or "!high"/"!med"/"!low" (aliases !h/!m/!l).
//   - Tags:     "#tag".
//
// Only the first date and first time win; later matches fall back to the title.
// Date and time are independent: HasTime without HasDate lets a caller apply the
// time to a context day (e.g. the selected calendar day) rather than assuming
// today.
type QuickAdd struct {
	Title    string
	HasDate  bool
	Date     time.Time // local midnight of the parsed date, valid when HasDate
	HasTime  bool
	Hour     int
	Minute   int
	Priority int // 0 = none
	Tags     []string
}

// ParseQuickAdd parses input relative to now (in loc), extracting date, time,
// priority, and tags and leaving the remainder as the title.
func ParseQuickAdd(input string, now time.Time, loc *time.Location) QuickAdd {
	if loc == nil {
		loc = time.Local
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	var qa QuickAdd
	var titleTokens []string

	tokens := strings.Fields(input)
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]

		switch tok[0] {
		case '#':
			if tag := strings.TrimSpace(tok[1:]); tag != "" {
				qa.Tags = append(qa.Tags, tag)
				continue
			}
		case '!':
			if p, ok := parsePriority(tok[1:]); ok && qa.Priority == 0 {
				qa.Priority = p
				continue
			}
		}

		if !qa.HasTime {
			if h, m, ok := parseClock(tok); ok {
				qa.HasTime = true
				qa.Hour, qa.Minute = h, m
				continue
			}
		}

		if !qa.HasDate {
			if d, consumed, ok := parseDate(tokens, i, today, loc); ok {
				qa.HasDate = true
				qa.Date = d
				i += consumed - 1
				continue
			}
		}

		titleTokens = append(titleTokens, tok)
	}

	qa.Title = strings.Join(titleTokens, " ")
	return qa
}

// At combines a QuickAdd's date and time onto base (used when no explicit date
// was given, e.g. the selected calendar day), returning the resulting time and
// whether it should be treated as all-day (a date with no time).
func (qa QuickAdd) At(base time.Time, loc *time.Location) (when time.Time, allDay bool) {
	y, mo, d := base.Year(), base.Month(), base.Day()
	if qa.HasDate {
		y, mo, d = qa.Date.Year(), qa.Date.Month(), qa.Date.Day()
	}
	if qa.HasTime {
		return time.Date(y, mo, d, qa.Hour, qa.Minute, 0, 0, loc), false
	}
	return time.Date(y, mo, d, 0, 0, 0, 0, loc), true
}

// parsePriority maps the text after "!" to a 1–9 priority, or reports no match.
func parsePriority(s string) (int, bool) {
	switch strings.ToLower(s) {
	case "high", "h":
		return 1, true
	case "med", "medium", "m":
		return 5, true
	case "low", "l":
		return 9, true
	}
	if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= 9 {
		return n, true
	}
	return 0, false
}

// parseClock parses a time-of-day token. To stay unambiguous it accepts only
// values carrying am/pm or a colon; a bare integer is rejected so day numbers
// are not mistaken for times.
func parseClock(tok string) (hour, minute int, ok bool) {
	s := strings.ToLower(tok)

	ampm := ""
	switch {
	case strings.HasSuffix(s, "am"):
		ampm, s = "am", strings.TrimSuffix(s, "am")
	case strings.HasSuffix(s, "pm"):
		ampm, s = "pm", strings.TrimSuffix(s, "pm")
	}

	if ampm == "" && !strings.Contains(s, ":") {
		return 0, 0, false // bare number: not a time
	}

	h, m := 0, 0
	if idx := strings.IndexByte(s, ':'); idx >= 0 {
		var err error
		if h, err = strconv.Atoi(s[:idx]); err != nil {
			return 0, 0, false
		}
		if m, err = strconv.Atoi(s[idx+1:]); err != nil {
			return 0, 0, false
		}
	} else {
		var err error
		if h, err = strconv.Atoi(s); err != nil {
			return 0, 0, false
		}
	}
	if m < 0 || m > 59 {
		return 0, 0, false
	}

	switch ampm {
	case "am":
		if h < 1 || h > 12 {
			return 0, 0, false
		}
		if h == 12 {
			h = 0
		}
	case "pm":
		if h < 1 || h > 12 {
			return 0, 0, false
		}
		if h != 12 {
			h += 12
		}
	default:
		if h < 0 || h > 23 {
			return 0, 0, false
		}
	}
	return h, m, true
}

// parseDate tries to read a date starting at tokens[i], returning the date, how
// many tokens it consumed (1 or 2), and whether it matched. Past month-name and
// numeric dates roll forward to the next occurrence, which suits due dates.
func parseDate(tokens []string, i int, today time.Time, loc *time.Location) (time.Time, int, bool) {
	tok := strings.ToLower(tokens[i])

	switch tok {
	case "today", "tod":
		return today, 1, true
	case "tomorrow", "tom", "tmr":
		return today.AddDate(0, 0, 1), 1, true
	case "next":
		if d, ok := parseNextDate(tokens, i, today); ok {
			return d, 2, true
		}
	case "in":
		if d, ok := parseInDate(tokens, i, today); ok {
			return d, 3, true
		}
	}

	if wd, ok := weekday(tok); ok {
		return nextWeekday(today, wd), 1, true
	}

	// "jul 20" / "july 20": a month name followed by a day number.
	if mon, ok := monthName(tok); ok && i+1 < len(tokens) {
		if day, err := strconv.Atoi(tokens[i+1]); err == nil && day >= 1 && day <= 31 {
			if d, ok := rollForwardMonthDay(today, mon, day, loc); ok {
				return d, 2, true
			}
		}
	}

	// Numeric forms: 2026-07-20, 7/20, 7/20/2026.
	if d, ok := parseNumericDate(tok, today, loc); ok {
		return d, 1, true
	}

	return time.Time{}, 0, false
}

func weekday(s string) (time.Weekday, bool) {
	days := map[string]time.Weekday{
		"sun": time.Sunday, "sunday": time.Sunday,
		"mon": time.Monday, "monday": time.Monday,
		"tue": time.Tuesday, "tues": time.Tuesday, "tuesday": time.Tuesday,
		"wed": time.Wednesday, "weds": time.Wednesday, "wednesday": time.Wednesday,
		"thu": time.Thursday, "thur": time.Thursday, "thurs": time.Thursday, "thursday": time.Thursday,
		"fri": time.Friday, "friday": time.Friday,
		"sat": time.Saturday, "saturday": time.Saturday,
	}
	wd, ok := days[s]
	return wd, ok
}

// nextWeekday returns the soonest day on or after today that falls on wd.
func nextWeekday(today time.Time, wd time.Weekday) time.Time {
	delta := (int(wd) - int(today.Weekday()) + 7) % 7
	return today.AddDate(0, 0, delta)
}

// parseNextDate handles the two-token "next …" relative forms starting at
// tokens[i] (which is "next"): "next <weekday>" is the bare-weekday result plus
// seven days (a single rule with no week-start dependence, so "next fri" typed
// on a Friday is a full week out), "next week" is today+7, and "next month" is
// the same day-of-month next month (clamped to that month's last day). Anything
// else — "next steps", "next year" — is not a date, leaving "next" in the title.
func parseNextDate(tokens []string, i int, today time.Time) (time.Time, bool) {
	if i+1 >= len(tokens) {
		return time.Time{}, false
	}
	next := strings.ToLower(tokens[i+1])
	if wd, ok := weekday(next); ok {
		return nextWeekday(today, wd).AddDate(0, 0, 7), true
	}
	switch next {
	case "week":
		return today.AddDate(0, 0, 7), true
	case "month":
		return addMonthsClamped(today, 1), true
	}
	return time.Time{}, false
}

// parseInDate handles the three-token "in N days/weeks/months" relative form
// starting at tokens[i] (which is "in"). N is 1–3 digits; units accept both the
// plural and singular spellings; months clamp the day-of-month like "next
// month". A follower that isn't a bounded count + known unit is not a date, so
// "in room 5" / "in 2026 days" / "in 5 minutes" leave "in" in the title.
func parseInDate(tokens []string, i int, today time.Time) (time.Time, bool) {
	if i+2 >= len(tokens) {
		return time.Time{}, false
	}
	countTok := tokens[i+1]
	if len(countTok) < 1 || len(countTok) > 3 || !isAllDigits(countTok) {
		return time.Time{}, false
	}
	n, err := strconv.Atoi(countTok)
	if err != nil {
		return time.Time{}, false
	}
	switch strings.ToLower(tokens[i+2]) {
	case "day", "days":
		return today.AddDate(0, 0, n), true
	case "week", "weeks":
		return today.AddDate(0, 0, 7*n), true
	case "month", "months":
		return addMonthsClamped(today, n), true
	}
	return time.Time{}, false
}

// isAllDigits reports whether s is one or more ASCII digits and nothing else.
func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// addMonthsClamped adds n months to t keeping the day-of-month, clamped down to
// the target month's last day when the original day doesn't exist there (Jan 31
// + 1 month -> Feb 28/29). time.Date's own normalization would instead spill the
// overflow into the following month, which is not the intended relative-date
// meaning.
func addMonthsClamped(t time.Time, n int) time.Time {
	y, m, d := t.Year(), int(t.Month())-1+n, t.Day()
	y += m / 12
	m = m%12 + 1
	if last := daysInMonth(y, time.Month(m)); d > last {
		d = last
	}
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, t.Location())
}

// daysInMonth returns the number of days in the given month (day 0 of the next
// month is the last day of this one).
func daysInMonth(year int, mon time.Month) int {
	return time.Date(year, mon+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func monthName(s string) (time.Month, bool) {
	months := map[string]time.Month{
		"jan": time.January, "january": time.January,
		"feb": time.February, "february": time.February,
		"mar": time.March, "march": time.March,
		"apr": time.April, "april": time.April,
		"may": time.May,
		"jun": time.June, "june": time.June,
		"jul": time.July, "july": time.July,
		"aug": time.August, "august": time.August,
		"sep": time.September, "sept": time.September, "september": time.September,
		"oct": time.October, "october": time.October,
		"nov": time.November, "november": time.November,
		"dec": time.December, "december": time.December,
	}
	m, ok := months[s]
	return m, ok
}

// rollForwardMonthDay returns mon/day in the current year, or next year if that
// date has already passed. It reports ok=false for an impossible day-of-month
// (e.g. Feb 30, Apr 31) rather than letting time.Date normalize it into another
// month — matching the ISO parser, which rejects such a date outright, so the
// same logical input can't parse one way slashed and another way as ISO. A day
// that is real only in a leap year (Feb 29) is honored in whichever of the two
// candidate years actually has it.
func rollForwardMonthDay(today time.Time, mon time.Month, day int, loc *time.Location) (time.Time, bool) {
	for _, year := range []int{today.Year(), today.Year() + 1} {
		if !validYMD(year, mon, day) {
			continue
		}
		d := time.Date(year, mon, day, 0, 0, 0, 0, loc)
		if !d.Before(today) {
			return d, true
		}
	}
	return time.Time{}, false
}

// validYMD reports whether day is a real calendar day of mon in year. time.Date
// silently normalizes an out-of-range day into the following month (Feb 30 →
// Mar 2), so a round-trip through it is the check: if the normalized date's
// fields differ from the inputs, the day was impossible.
func validYMD(year int, mon time.Month, day int) bool {
	d := time.Date(year, mon, day, 0, 0, 0, 0, time.UTC)
	return d.Year() == year && d.Month() == mon && d.Day() == day
}

// parseNumericDate handles ISO (2006-01-02) and slashed US (m/d, m/d/y) dates.
func parseNumericDate(tok string, today time.Time, loc *time.Location) (time.Time, bool) {
	if strings.Count(tok, "-") == 2 {
		if t, err := time.ParseInLocation("2006-1-2", tok, loc); err == nil {
			return t, true
		}
		return time.Time{}, false
	}

	parts := strings.Split(tok, "/")
	if len(parts) != 2 && len(parts) != 3 {
		return time.Time{}, false
	}
	mon, err := strconv.Atoi(parts[0])
	if err != nil || mon < 1 || mon > 12 {
		return time.Time{}, false
	}
	day, err := strconv.Atoi(parts[1])
	if err != nil || day < 1 || day > 31 {
		return time.Time{}, false
	}
	if len(parts) == 3 {
		year, err := strconv.Atoi(parts[2])
		if err != nil {
			return time.Time{}, false
		}
		if year < 100 {
			year += 2000
		}
		if !validYMD(year, time.Month(mon), day) {
			return time.Time{}, false
		}
		return time.Date(year, time.Month(mon), day, 0, 0, 0, 0, loc), true
	}
	return rollForwardMonthDay(today, time.Month(mon), day, loc)
}
