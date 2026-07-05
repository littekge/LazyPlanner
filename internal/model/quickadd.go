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
	}

	if wd, ok := weekday(tok); ok {
		return nextWeekday(today, wd), 1, true
	}

	// "jul 20" / "july 20": a month name followed by a day number.
	if mon, ok := monthName(tok); ok && i+1 < len(tokens) {
		if day, err := strconv.Atoi(tokens[i+1]); err == nil && day >= 1 && day <= 31 {
			return rollForwardMonthDay(today, mon, day, loc), 2, true
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
// date has already passed.
func rollForwardMonthDay(today time.Time, mon time.Month, day int, loc *time.Location) time.Time {
	d := time.Date(today.Year(), mon, day, 0, 0, 0, 0, loc)
	if d.Before(today) {
		d = time.Date(today.Year()+1, mon, day, 0, 0, 0, 0, loc)
	}
	return d
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
		return time.Date(year, time.Month(mon), day, 0, 0, 0, 0, loc), true
	}
	return rollForwardMonthDay(today, time.Month(mon), day, loc), true
}
