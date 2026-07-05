package ui

import (
	"fmt"
	"math"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
)

const (
	gutterWidth = 6 // width of the hour-label column ("12pm ")
	blockColor  = tcell.ColorDarkSlateGray
)

// timeGridView is the week/day view: an hour axis with events drawn as blocks
// sized by duration, all-day items in a band at the top, and overlapping events
// placed side by side (via model.LayoutDay). Day view is one column, week seven.
type timeGridView struct {
	*tview.Box

	days       []time.Time // one (day view) or seven (week view), left to right
	timed      map[string][]model.Occurrence
	allDay     map[string][]model.Occurrence
	now        time.Time
	selected   time.Time
	scrollHour int // topmost visible hour

	eventMode  bool // cycling the selected day's timed events
	eventIndex int

	onSelectDay   func(day time.Time)
	onSelectEvent func(occ model.Occurrence)
	onExit        func() // Esc in day mode: hand focus back to the overview
}

func newTimeGridView() *timeGridView {
	return &timeGridView{Box: tview.NewBox(), scrollHour: 7}
}

func (tg *timeGridView) setData(days []time.Time, timed, allDay map[string][]model.Occurrence, selected, now time.Time) {
	tg.days = days
	tg.timed = timed
	tg.allDay = allDay
	tg.selected = selected
	tg.now = now
	tg.eventMode = false
	tg.eventIndex = 0
}

// dayOccs is the selected day's timed events, the set cycled in event mode.
// (All-day items live in the band and aren't part of the grid's cycle.)
func (tg *timeGridView) dayOccs() []model.Occurrence {
	return tg.timed[dayKey(tg.selected)]
}

func (tg *timeGridView) emitEvent() {
	occs := tg.dayOccs()
	if tg.eventIndex >= 0 && tg.eventIndex < len(occs) && tg.onSelectEvent != nil {
		tg.onSelectEvent(occs[tg.eventIndex])
	}
}

// drillState / reDrill implement calGrid so focus can be restored into the same
// day after a modal (see app.restoreFocus).
func (tg *timeGridView) drillState() (time.Time, bool, int) {
	return tg.selected, tg.eventMode, tg.eventIndex
}

func (tg *timeGridView) reDrill(day time.Time, index int) {
	tg.selected = day
	if occs := tg.dayOccs(); len(occs) > 0 {
		tg.eventMode = true
		tg.eventIndex = clampIndex(index, len(occs))
		tg.emitEvent()
	}
}

func (tg *timeGridView) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return tg.WrapInputHandler(func(ev *tcell.EventKey, _ func(tview.Primitive)) {
		if tg.eventMode {
			tg.handleEventMode(ev)
			return
		}
		tg.handleDayMode(ev)
	})
}

func (tg *timeGridView) handleDayMode(ev *tcell.EventKey) {
	move := func(days int) {
		if tg.onSelectDay != nil {
			tg.onSelectDay(tg.selected.AddDate(0, 0, days))
		}
	}
	scroll := func(by int) {
		tg.scrollHour += by
		if tg.scrollHour < 0 {
			tg.scrollHour = 0
		}
		if tg.scrollHour > 23 {
			tg.scrollHour = 23
		}
	}
	switch ev.Key() {
	case tcell.KeyLeft:
		move(-1)
	case tcell.KeyRight:
		move(1)
	case tcell.KeyUp:
		scroll(-1)
	case tcell.KeyDown:
		scroll(1)
	case tcell.KeyPgUp:
		scroll(-6)
	case tcell.KeyPgDn:
		scroll(6)
	case tcell.KeyEnter:
		if len(tg.dayOccs()) > 0 {
			tg.eventMode = true
			tg.eventIndex = 0
			tg.emitEvent()
		}
	case tcell.KeyEscape:
		if tg.onExit != nil {
			tg.onExit()
		}
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'h':
			move(-1)
		case 'l':
			move(1)
		case 'k':
			scroll(-1)
		case 'j':
			scroll(1)
		}
	}
}

func (tg *timeGridView) handleEventMode(ev *tcell.EventKey) {
	occs := tg.dayOccs()
	move := func(days int) {
		tg.eventMode = false
		if tg.onSelectDay != nil {
			tg.onSelectDay(tg.selected.AddDate(0, 0, days))
		}
	}
	switch ev.Key() {
	case tcell.KeyEscape:
		tg.eventMode = false
	case tcell.KeyUp:
		if tg.eventIndex > 0 {
			tg.eventIndex--
			tg.emitEvent()
		}
	case tcell.KeyDown:
		if tg.eventIndex < len(occs)-1 {
			tg.eventIndex++
			tg.emitEvent()
		}
	case tcell.KeyLeft:
		move(-1)
	case tcell.KeyRight:
		move(1)
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'k':
			if tg.eventIndex > 0 {
				tg.eventIndex--
				tg.emitEvent()
			}
		case 'j':
			if tg.eventIndex < len(occs)-1 {
				tg.eventIndex++
				tg.emitEvent()
			}
		}
	}
}

func (tg *timeGridView) Draw(screen tcell.Screen) {
	tg.Box.DrawForSubclass(screen, tg)
	x, y, w, h := tg.GetInnerRect()
	n := len(tg.days)
	if n == 0 || w < gutterWidth+n || h < 4 {
		return
	}

	colW := (w - gutterWidth) / n
	colStart := x + gutterWidth
	sepStyle := tcell.StyleDefault.Foreground(borderIdle)

	// Header: one date per column (selected day reversed).
	for di, day := range tg.days {
		style := tcell.StyleDefault.Foreground(accentColor).Bold(true)
		if model.SameDay(day, tg.selected) {
			style = style.Reverse(true)
		}
		printStyled(screen, colStart+di*colW+1, y, colW-1, day.Format("Mon 2"), style)
	}

	// All-day band.
	printStyled(screen, x, y+1, gutterWidth, "all", tcell.StyleDefault.Foreground(adjacentColor))
	for di, day := range tg.days {
		if ad := tg.allDay[dayKey(day)]; len(ad) > 0 {
			label := nonEmpty(ad[0].Event.Summary, "(untitled)")
			if len(ad) > 1 {
				label = fmt.Sprintf("%s +%d", label, len(ad)-1)
			}
			printStyled(screen, colStart+di*colW+1, y+1, colW-1, label, tcell.StyleDefault.Foreground(eventColor))
		}
	}
	for xx := x; xx < x+w; xx++ {
		screen.SetContent(xx, y+2, tcell.RuneHLine, nil, sepStyle)
	}

	bodyY := y + 3
	visible := h - 3
	if visible < 1 {
		return
	}

	// Hour labels + faint hour lines.
	for i := 0; i < visible; i++ {
		hour := tg.scrollHour + i
		if hour > 23 {
			break
		}
		printStyled(screen, x, bodyY+i, gutterWidth-1, hourLabel(hour), tcell.StyleDefault.Foreground(adjacentColor))
	}
	// Column separators.
	for di := 0; di <= n; di++ {
		sx := colStart + di*colW - 1
		if di == 0 {
			sx = colStart - 1
		}
		for yy := bodyY; yy < bodyY+visible && yy < y+h; yy++ {
			screen.SetContent(sx, yy, tcell.RuneVLine, nil, sepStyle)
		}
	}

	// Event blocks. In event mode the cycled event on the selected day is boxed.
	var sel *model.Occurrence
	if tg.eventMode {
		if occs := tg.dayOccs(); tg.eventIndex >= 0 && tg.eventIndex < len(occs) {
			sel = &occs[tg.eventIndex]
		}
	}
	for di, day := range tg.days {
		places := model.LayoutDay(tg.timed[dayKey(day)])
		for _, p := range places {
			selected := sel != nil && model.SameDay(day, tg.selected) &&
				p.Occ.Event == sel.Event && p.Occ.Start.Equal(sel.Start)
			tg.drawBlock(screen, p, colStart+di*colW, colW, bodyY, visible, selected)
		}
	}
}

func (tg *timeGridView) drawBlock(screen tcell.Screen, p model.Placement, colX, colW, bodyY, visible int, selected bool) {
	startT := p.Occ.Start.In(time.Local)
	endT := p.Occ.End.In(time.Local)
	startH := hourFloat(startT)
	endH := hourFloat(endT)

	top := bodyY + int(math.Floor(startH)) - tg.scrollHour
	end := bodyY + int(math.Ceil(endH)) - tg.scrollHour
	if end <= bodyY || top >= bodyY+visible {
		return // outside the visible hour window
	}
	if top < bodyY {
		top = bodyY
	}
	if end > bodyY+visible {
		end = bodyY + visible
	}
	height := end - top
	if height < 1 {
		height = 1
	}

	lanes := p.Lanes
	if lanes < 1 {
		lanes = 1
	}
	laneW := (colW - 1) / lanes
	if laneW < 1 {
		laneW = 1
	}
	bx := colX + p.Lane*laneW
	bw := laneW

	style := tcell.StyleDefault.Background(blockColor).Foreground(tcell.ColorWhite)
	if selected {
		style = tcell.StyleDefault.Background(accentColor).Foreground(tcell.ColorBlack).Bold(true)
	}
	for yy := top; yy < top+height; yy++ {
		for xx := bx; xx < bx+bw; xx++ {
			screen.SetContent(xx, yy, ' ', nil, style)
		}
	}
	printStyled(screen, bx, top, bw, nonEmpty(p.Occ.Event.Summary, "(untitled)"), style)
	if height >= 2 {
		span := startT.Format("3:04") + "-" + endT.Format("3:04pm")
		printStyled(screen, bx, top+1, bw, span, style.Foreground(tcell.ColorSilver))
	}
}

func hourFloat(t time.Time) float64 {
	return float64(t.Hour()) + float64(t.Minute())/60
}

func hourLabel(hour int) string {
	return time.Date(2000, 1, 1, hour, 0, 0, 0, time.UTC).Format("3pm")
}
