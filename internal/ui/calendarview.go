package ui

import (
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
)

// calendarView is the custom-drawn month grid. Each day is a cell that fills the
// available space and lists the day's events/tasks. The selected day is marked
// with an outline box (never a solid fill) so event text stays readable. Arrow
// keys move the selection; Enter drops into "event mode" to cycle that day's
// items, reporting the highlighted one so the Detail pane can show it.
type calendarView struct {
	*tview.Box

	weeks       [][]time.Time
	items       map[string][]model.AgendaItem
	selected    time.Time
	now         time.Time
	month       time.Month // for dimming adjacent-month days; 0 = don't dim (week)
	mondayFirst bool

	eventMode  bool // cycling events within the selected day
	eventIndex int

	onSelectDay   func(day time.Time)
	onSelectEvent func(item model.AgendaItem)
	onExit        func() // Esc in day mode: hand focus back to the overview

	// itemColor resolves an item to its calendar's color; ok is false when the
	// calendar has none, so the default event/task color is used.
	itemColor func(model.AgendaItem) (calColor, bool)
	// isFolder reports whether a task UID is a folder (▸ marker instead of a box).
	isFolder func(uid string) bool
}

func newCalendarView() *calendarView {
	return &calendarView{Box: tview.NewBox(), items: map[string][]model.AgendaItem{}}
}

func (cv *calendarView) setData(weeks [][]time.Time, items map[string][]model.AgendaItem, month time.Month, selected, now time.Time, mondayFirst bool) {
	cv.weeks = weeks
	cv.items = items
	cv.month = month
	cv.selected = selected
	cv.now = now
	cv.mondayFirst = mondayFirst
	cv.eventMode = false
	cv.eventIndex = 0
}

func (cv *calendarView) selectedItems() []model.AgendaItem {
	return cv.items[dayKey(cv.selected)]
}

func (cv *calendarView) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return cv.WrapInputHandler(func(ev *tcell.EventKey, _ func(tview.Primitive)) {
		if cv.eventMode {
			cv.handleEventMode(ev)
			return
		}
		cv.handleDayMode(ev)
	})
}

func (cv *calendarView) handleDayMode(ev *tcell.EventKey) {
	move := func(days int) {
		if cv.onSelectDay != nil {
			cv.onSelectDay(cv.selected.AddDate(0, 0, days))
		}
	}
	switch ev.Key() {
	case tcell.KeyLeft:
		move(-1)
	case tcell.KeyRight:
		move(1)
	case tcell.KeyUp:
		move(-7)
	case tcell.KeyDown:
		move(7)
	case tcell.KeyHome: // gg: jump to the first day cell
		if cv.onSelectDay != nil && len(cv.weeks) > 0 {
			cv.onSelectDay(cv.weeks[0][0])
		}
	case tcell.KeyEnd: // G: jump to the last day cell
		if cv.onSelectDay != nil && len(cv.weeks) > 0 {
			last := cv.weeks[len(cv.weeks)-1]
			cv.onSelectDay(last[len(last)-1])
		}
	case tcell.KeyEnter:
		if len(cv.selectedItems()) > 0 {
			cv.eventMode = true
			cv.eventIndex = 0
			cv.emitEvent()
		}
	case tcell.KeyEscape:
		if cv.onExit != nil {
			cv.onExit()
		}
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'h':
			move(-1)
		case 'l':
			move(1)
		case 'k':
			move(-7)
		case 'j':
			move(7)
		}
	}
}

func (cv *calendarView) handleEventMode(ev *tcell.EventKey) {
	items := cv.selectedItems()
	switch ev.Key() {
	case tcell.KeyEscape:
		cv.eventMode = false
		if cv.onSelectDay != nil {
			cv.onSelectDay(cv.selected)
		}
	case tcell.KeyUp:
		if cv.eventIndex > 0 {
			cv.eventIndex--
			cv.emitEvent()
		}
	case tcell.KeyDown:
		if cv.eventIndex < len(items)-1 {
			cv.eventIndex++
			cv.emitEvent()
		}
	case tcell.KeyHome: // gg: first event of the day
		if len(items) > 0 {
			cv.eventIndex = 0
			cv.emitEvent()
		}
	case tcell.KeyEnd: // G: last event of the day
		if len(items) > 0 {
			cv.eventIndex = len(items) - 1
			cv.emitEvent()
		}
	case tcell.KeyLeft, tcell.KeyRight:
		cv.eventMode = false
		days := 1
		if ev.Key() == tcell.KeyLeft {
			days = -1
		}
		if cv.onSelectDay != nil {
			cv.onSelectDay(cv.selected.AddDate(0, 0, days))
		}
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'k':
			if cv.eventIndex > 0 {
				cv.eventIndex--
				cv.emitEvent()
			}
		case 'j':
			if cv.eventIndex < len(items)-1 {
				cv.eventIndex++
				cv.emitEvent()
			}
		}
	}
}

func (cv *calendarView) emitEvent() {
	items := cv.selectedItems()
	if cv.eventIndex >= 0 && cv.eventIndex < len(items) && cv.onSelectEvent != nil {
		cv.onSelectEvent(items[cv.eventIndex])
	}
}

// drillState / reDrill implement calGrid so focus can be restored into the same
// day after a modal closes (see app.restoreFocus).
func (cv *calendarView) drillState() (time.Time, bool, int) {
	return cv.selected, cv.eventMode, cv.eventIndex
}

func (cv *calendarView) reDrill(day time.Time, index int) {
	cv.selected = day
	if items := cv.selectedItems(); len(items) > 0 {
		cv.eventMode = true
		cv.eventIndex = clampIndex(index, len(items))
		cv.emitEvent()
	}
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
	sepStyle := tcell.StyleDefault.Foreground(borderIdle)

	for c, name := range weekdayHeaderNames(cv.mondayFirst) {
		printStyled(screen, x+c*colW+1, y, colW-1, name,
			tcell.StyleDefault.Foreground(accentColor).Bold(true))
	}
	for xx := x; xx < x+w; xx++ {
		screen.SetContent(xx, y+1, tcell.RuneHLine, nil, sepStyle)
	}

	bodyY := y + 2
	cellH := (h - 2) / rows
	if cellH < 1 {
		cellH = 1
	}
	bottom := bodyY + rows*cellH

	// Column separators.
	for c := 1; c < cols; c++ {
		sx := x + c*colW
		for yy := y; yy < bottom && yy < y+h; yy++ {
			screen.SetContent(sx, yy, tcell.RuneVLine, nil, sepStyle)
		}
	}
	// Day cells.
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			cv.drawCell(screen, cv.weeks[r][c], x+c*colW+1, bodyY+r*cellH, colW-1, cellH)
		}
	}
}

func (cv *calendarView) drawCell(screen tcell.Screen, day time.Time, cellX, cellY, cellW, cellH int) {
	if cellW < 1 {
		return
	}
	selected := model.SameDay(day, cv.selected)
	today := model.SameDay(day, cv.now)
	adjacent := cv.month != 0 && day.Month() != cv.month

	cx, cy, cw, ch := cellX, cellY, cellW, cellH
	if selected {
		boxColor := borderIdle
		if cv.HasFocus() {
			boxColor = borderFocused
		}
		drawBox(screen, cellX, cellY, cellW, cellH, tcell.StyleDefault.Foreground(boxColor))
		cx, cy, cw, ch = cellX+1, cellY+1, cellW-2, cellH-2
	}
	if cw < 1 || ch < 1 {
		return
	}

	items := cv.items[dayKey(day)]
	numFg := tcell.ColorWhite
	switch {
	case today:
		numFg = todayColor
	case adjacent:
		numFg = adjacentColor
	}
	numStyle := tcell.StyleDefault.Foreground(numFg)
	if today {
		numStyle = numStyle.Bold(true)
	}
	num := fmt.Sprintf("%d", day.Day())
	if ch <= 1 && len(items) > 0 {
		num = fmt.Sprintf("%d (%d)", day.Day(), len(items))
	}
	printStyled(screen, cx, cy, cw, num, numStyle)

	avail := ch - 1
	if avail <= 0 || len(items) == 0 {
		return
	}
	n := len(items)
	capItems := avail
	overflow := n > avail
	if overflow {
		capItems = avail - 1 // reserve one row for the "+N more" indicator
		if capItems < 0 {
			capItems = 0
		}
	}
	// When this day is drilled, scroll the visible window so the highlighted item
	// stays on screen instead of disappearing into the overflow.
	start := 0
	if overflow && selected && cv.eventMode && capItems > 0 {
		if cv.eventIndex >= capItems {
			start = cv.eventIndex - capItems + 1
		}
		if maxStart := n - capItems; start > maxStart {
			start = maxStart
		}
		if start < 0 {
			start = 0
		}
	}
	end := start + capItems
	if end > n {
		end = n
	}
	for i := start; i < end; i++ {
		style := itemStyle(items[i])
		if cv.itemColor != nil {
			if cc, ok := cv.itemColor(items[i]); ok {
				style = style.Foreground(cc.fg)
			}
		}
		if selected && cv.eventMode && i == cv.eventIndex {
			style = style.Reverse(true)
		}
		printStyled(screen, cx, cy+1+(i-start), cw, itemLabel(items[i], cv.folderItem(items[i])), style)
	}
	if overflow {
		// hidden counts every item outside the window (above when scrolled, below
		// otherwise) — the drilled item is always inside it, so never hidden.
		printStyled(screen, cx, cy+1+capItems, cw, fmt.Sprintf("+%d more", n-(end-start)),
			tcell.StyleDefault.Foreground(adjacentColor))
	}
}

// folderItem reports whether an agenda item is a task that's a folder.
func (cv *calendarView) folderItem(it model.AgendaItem) bool {
	return it.IsTodo() && cv.isFolder != nil && cv.isFolder(it.Todo.UID)
}

// itemLabel and itemStyle format a day-cell agenda line. folder marks a task with
// incomplete children (▸, matching the tree) instead of a checkbox.
func itemLabel(it model.AgendaItem, folder bool) string {
	switch {
	case it.IsTodo():
		return todoMark(it.Todo, folder) + nonEmpty(it.Title, "(untitled)")
	case it.AllDay:
		return nonEmpty(it.Title, "(untitled)")
	default:
		return it.Start.In(time.Local).Format("3pm") + " " + nonEmpty(it.Title, "(untitled)")
	}
}

func itemStyle(it model.AgendaItem) tcell.Style {
	if it.IsTodo() {
		return tcell.StyleDefault.Foreground(tcell.ColorAqua)
	}
	return tcell.StyleDefault.Foreground(eventColor)
}

// drawBox draws a rectangle border of the given style.
func drawBox(screen tcell.Screen, x, y, w, h int, style tcell.Style) {
	if w < 2 || h < 2 {
		return
	}
	for xx := x + 1; xx < x+w-1; xx++ {
		screen.SetContent(xx, y, tcell.RuneHLine, nil, style)
		screen.SetContent(xx, y+h-1, tcell.RuneHLine, nil, style)
	}
	for yy := y + 1; yy < y+h-1; yy++ {
		screen.SetContent(x, yy, tcell.RuneVLine, nil, style)
		screen.SetContent(x+w-1, yy, tcell.RuneVLine, nil, style)
	}
	// Rounded (soft) corners, matching the pane borders.
	screen.SetContent(x, y, '╭', nil, style)
	screen.SetContent(x+w-1, y, '╮', nil, style)
	screen.SetContent(x, y+h-1, '╰', nil, style)
	screen.SetContent(x+w-1, y+h-1, '╯', nil, style)
}

func weekdayHeaderNames(mondayFirst bool) []string {
	if mondayFirst {
		return []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	}
	return []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
}

// printStyled draws text clipped to maxWidth using style, honoring rune widths.
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
