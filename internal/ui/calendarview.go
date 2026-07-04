package ui

import (
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
)

// calendarView is a custom-drawn month/week grid. Each day is a cell that fills
// the available width and height and lists the day's events/tasks, unlike a
// tview.Table (content-width, single-line cells). Arrow/hjkl keys move the
// selected day and invoke onSelect; the app decides whether to re-anchor.
type calendarView struct {
	*tview.Box

	weeks       [][]time.Time                 // rows of 7 days to draw
	items       map[string][]model.AgendaItem // dayKey -> that day's agenda
	selected    time.Time
	now         time.Time
	month       time.Month // focused month for dimming adjacent days; 0 = don't dim (week view)
	mondayFirst bool

	onSelect func(day time.Time)
}

func newCalendarView() *calendarView {
	return &calendarView{Box: tview.NewBox(), items: map[string][]model.AgendaItem{}}
}

// setData replaces what the grid draws.
func (cv *calendarView) setData(weeks [][]time.Time, items map[string][]model.AgendaItem, month time.Month, selected, now time.Time, mondayFirst bool) {
	cv.weeks = weeks
	cv.items = items
	cv.month = month
	cv.selected = selected
	cv.now = now
	cv.mondayFirst = mondayFirst
}

func (cv *calendarView) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return cv.WrapInputHandler(func(ev *tcell.EventKey, _ func(tview.Primitive)) {
		delta := 0
		switch ev.Key() {
		case tcell.KeyLeft:
			delta = -1
		case tcell.KeyRight:
			delta = 1
		case tcell.KeyUp:
			delta = -7
		case tcell.KeyDown:
			delta = 7
		case tcell.KeyRune:
			switch ev.Rune() {
			case 'h':
				delta = -1
			case 'l':
				delta = 1
			case 'k':
				delta = -7
			case 'j':
				delta = 7
			}
		}
		if delta != 0 && cv.onSelect != nil {
			cv.onSelect(cv.selected.AddDate(0, 0, delta))
		}
	})
}

func (cv *calendarView) Draw(screen tcell.Screen) {
	cv.Box.DrawForSubclass(screen, cv)
	x, y, w, h := cv.GetInnerRect()
	rows := len(cv.weeks)
	if w < 7 || h < 3 || rows == 0 {
		return
	}

	const cols = 7
	colW := w / cols
	names := weekdayHeaderNames(cv.mondayFirst)
	sepStyle := tcell.StyleDefault.Foreground(borderIdle)

	// Header row of weekday names.
	for c := 0; c < cols; c++ {
		printStyled(screen, x+c*colW+1, y, colW-1, names[c],
			tcell.StyleDefault.Foreground(accentColor).Bold(true))
	}
	// Rule under the header.
	for xx := x; xx < x+w; xx++ {
		screen.SetContent(xx, y+1, tcell.RuneHLine, nil, sepStyle)
	}

	bodyY := y + 2
	cellH := (h - 2) / rows
	if cellH < 1 {
		cellH = 1
	}
	bottom := bodyY + rows*cellH

	// Day cells.
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			cv.drawCell(screen, cv.weeks[r][c], x+c*colW+1, bodyY+r*cellH, colW-1, cellH)
		}
	}
	// Vertical separators between columns.
	for c := 1; c < cols; c++ {
		sx := x + c*colW
		for yy := y; yy < bottom && yy < y+h; yy++ {
			screen.SetContent(sx, yy, tcell.RuneVLine, nil, sepStyle)
		}
	}
}

func (cv *calendarView) drawCell(screen tcell.Screen, day time.Time, cx, cy, cw, ch int) {
	if cw < 1 {
		return
	}
	selected := model.SameDay(day, cv.selected)
	today := model.SameDay(day, cv.now)
	adjacent := cv.month != 0 && day.Month() != cv.month

	bg := tcell.ColorDefault
	if selected {
		if cv.HasFocus() {
			bg = tcell.ColorTeal
		} else {
			bg = tcell.ColorDarkSlateGray
		}
	}
	base := tcell.StyleDefault.Background(bg)

	// Fill the cell so the selection background is solid.
	for yy := cy; yy < cy+ch; yy++ {
		for xx := cx; xx < cx+cw; xx++ {
			screen.SetContent(xx, yy, ' ', nil, base)
		}
	}

	// Day number (with an event count when the cell is only one line tall).
	items := cv.items[dayKey(day)]
	numFg := tcell.ColorWhite
	switch {
	case selected:
		numFg = tcell.ColorWhite
	case today:
		numFg = todayColor
	case adjacent:
		numFg = adjacentColor
	}
	numStyle := base.Foreground(numFg)
	if today {
		numStyle = numStyle.Bold(true)
	}
	num := fmt.Sprintf("%d", day.Day())
	if ch <= 1 && len(items) > 0 {
		num = fmt.Sprintf("%d (%d)", day.Day(), len(items))
	}
	printStyled(screen, cx, cy, cw, num, numStyle)

	// Event/task lines.
	avail := ch - 1
	if avail <= 0 || len(items) == 0 {
		return
	}
	shown := len(items)
	if shown > avail {
		shown = avail - 1 // reserve the last line for the overflow note
		if shown < 0 {
			shown = 0
		}
	}
	for i := 0; i < shown; i++ {
		it := items[i]
		fg := eventColor
		var label string
		switch {
		case it.IsTodo():
			fg = tcell.ColorAqua
			label = "[] " + nonEmpty(it.Title, "(untitled)")
		case it.AllDay:
			label = nonEmpty(it.Title, "(untitled)")
		default:
			label = it.Start.Format("3pm") + " " + nonEmpty(it.Title, "(untitled)")
		}
		if selected {
			fg = tcell.ColorWhite
		}
		printStyled(screen, cx, cy+1+i, cw, label, base.Foreground(fg))
	}
	if len(items) > avail {
		printStyled(screen, cx, cy+1+shown, cw, fmt.Sprintf("+%d more", len(items)-shown),
			base.Foreground(adjacentColor))
	}
}

// weekdayHeaderNames returns the weekday abbreviations in display order.
func weekdayHeaderNames(mondayFirst bool) []string {
	if mondayFirst {
		return []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	}
	return []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
}

// printStyled draws text at (x,y) clipped to maxWidth using style, honoring
// rune display widths. tview.Print only sets a foreground color, so this exists
// for styled (background-aware) drawing.
func printStyled(screen tcell.Screen, x, y, maxWidth int, text string, style tcell.Style) {
	if maxWidth <= 0 {
		return
	}
	text = runewidth.Truncate(text, maxWidth, "")
	col := x
	for _, r := range text {
		rw := runewidth.RuneWidth(r)
		if rw == 0 {
			rw = 1
		}
		if col+rw > x+maxWidth {
			break
		}
		screen.SetContent(col, y, r, nil, style)
		col += rw
	}
}
