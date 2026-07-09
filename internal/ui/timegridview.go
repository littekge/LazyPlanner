package ui

import (
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
)

const (
	gutterWidth = 6 // width of the hour-label column ("12pm ")
	blockColor  = tcell.ColorDarkSlateGray
	hoursPerDay = 24
)

// timeGridView is the week/day view: a 24-hour axis with events drawn as blocks
// sized by duration, all-day items in a band at the top, and overlapping events
// placed side by side (via model.LayoutDay). Day view is one column, week seven.
// The whole day is scaled to fill the pane height (no vertical scrolling).
type timeGridView struct {
	*tview.Box

	days     []time.Time // one (day view) or seven (week view), left to right
	timed    map[string][]model.Occurrence
	allDay   map[string][]model.Occurrence
	now      time.Time
	selected time.Time

	eventMode  bool // cycling the selected day's items (all-day, then timed events + tasks)
	eventIndex int

	// items is the per-day drill list — all-day items, then timed events and due
	// tasks by time (model.DayAgenda order). The drill cycles these so a task is
	// selectable (and thus completable/editable) like an event; it's separate from
	// the timed/allDay/dueTasks draw data. Set by the app alongside setData.
	items map[string][]model.AgendaItem

	onSelectDay   func(day time.Time)
	onSelectEvent func(model.AgendaItem)
	onExit        func() // Esc in day mode: hand focus back to the overview

	// occColor resolves an occurrence to its calendar's color; ok is false when
	// the calendar has none, so the default block/event color is used.
	occColor func(model.Occurrence) (calColor, bool)

	// dueTasks holds tasks due on each day (keyed like timed/allDay). Timed-due
	// tasks draw a marker at their due time; all-day-due tasks sit in the top band.
	dueTasks map[string][]*model.Todo
	// taskColor resolves a task to its list's color; ok is false when the list has
	// none, so the default task color (aqua) is used.
	taskColor func(*model.Todo) (calColor, bool)
	// isFolder reports whether a task UID is a folder (▸ marker instead of a box).
	isFolder func(uid string) bool
}

// folderTask reports whether a task is a folder (has incomplete children).
func (tg *timeGridView) folderTask(t *model.Todo) bool {
	return tg.isFolder != nil && tg.isFolder(t.UID)
}

// dueParts splits a day's due tasks into all-day (top band) and timed (a marker
// at the due time in the grid body).
func (tg *timeGridView) dueParts(day time.Time) (allDay, timed []*model.Todo) {
	for _, t := range tg.dueTasks[dayKey(day)] {
		if t.DueAllDay {
			allDay = append(allDay, t)
		} else {
			timed = append(timed, t)
		}
	}
	return allDay, timed
}

// taskFg is a task's list color, or aqua when its list has none.
func (tg *timeGridView) taskFg(t *model.Todo) tcell.Color {
	if tg.taskColor != nil {
		if cc, ok := tg.taskColor(t); ok {
			return cc.fg
		}
	}
	return tcell.ColorAqua
}

// taskMarkerLabel formats a due task's one-line label in the time-grid with the
// same marker convention as the month grid and task tree: ▸ folder, [ ]
// uncompleted, [■] completed. The foreground-only text (over the grid, not a
// filled block) already distinguishes a due task from an event.
func taskMarkerLabel(t *model.Todo, folder bool) string {
	return todoMark(t, folder) + nonEmpty(t.Summary, "(untitled)")
}

func newTimeGridView() *timeGridView {
	return &timeGridView{Box: tview.NewBox()}
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

// daySelectables is the selected day's drill list: all-day items first, then
// timed events and due tasks by time (model.DayAgenda order).
func (tg *timeGridView) daySelectables() []model.AgendaItem {
	return tg.items[dayKey(tg.selected)]
}

// enterEventMode starts cycling the selected day's items (events and due tasks),
// selecting the first. A no-op when the day has none. Vertical motion in day mode
// enters here so the day navigates like a list; a repeated motion (a count, or
// held j) then advances via handleEventMode.
func (tg *timeGridView) enterEventMode() {
	if !tg.eventMode && len(tg.daySelectables()) > 0 {
		tg.eventMode = true
		tg.eventIndex = 0
		tg.emitEvent()
	}
}

func (tg *timeGridView) emitEvent() {
	items := tg.daySelectables()
	if tg.eventIndex >= 0 && tg.eventIndex < len(items) && tg.onSelectEvent != nil {
		tg.onSelectEvent(items[tg.eventIndex])
	}
}

// selectedItem is the item currently highlighted in event mode, or nil.
func (tg *timeGridView) selectedItem() *model.AgendaItem {
	if !tg.eventMode {
		return nil
	}
	items := tg.daySelectables()
	if tg.eventIndex >= 0 && tg.eventIndex < len(items) {
		return &items[tg.eventIndex]
	}
	return nil
}

// drillState / reDrill implement calGrid so focus can be restored into the same
// day after a modal (see app.restoreFocus).
func (tg *timeGridView) drillState() (time.Time, bool, int) {
	return tg.selected, tg.eventMode, tg.eventIndex
}

func (tg *timeGridView) reDrill(day time.Time, index int) {
	tg.selected = day
	if items := tg.daySelectables(); len(items) > 0 {
		tg.eventMode = true
		tg.eventIndex = clampIndex(index, len(items))
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

// --- spatial navigation within a drilled day (week/day view) ---
//
// The drill navigates the day's on-screen layout: j/k move vertically by time,
// h/l move between concurrent events (the overlap lanes model.LayoutDay already
// computes). The all-day band is the top row (h/l between its items; j enters the
// timed grid, k from the top timed row returns to it); timed due-task markers are
// single-lane rows in the vertical flow. Movement stops at the day's edges — f/b
// changes the period, Esc returns to day navigation.
const (
	navUp = iota
	navDown
	navLeft
	navRight
)

const (
	cellBand  = iota // all-day band (top row)
	cellEvent        // timed event (positioned by an overlap lane)
	cellTask         // timed due-task marker (single lane, full width)
)

// navCell is a drilled item's position for spatial navigation.
type navCell struct {
	kind  int
	start time.Time
	end   time.Time
	lane  int
}

func (c navCell) timed() bool { return c.kind != cellBand }
func (c navCell) rank() int {
	if c.kind == cellTask {
		return 1
	}
	return 0
} // task sorts below an event at the same time
func (a navCell) overlaps(b navCell) bool {
	return a.start.Before(b.end) && b.start.Before(a.end)
}

// sameLevel: two timed cells at the same vertical position (a horizontal row).
func sameLevel(a, b navCell) bool {
	return a.start.Equal(b.start) && a.rank() == b.rank()
}

// levelLess: a is vertically above b (earlier time, or same time with an event
// above a task).
func levelLess(a, b navCell) bool {
	if !a.start.Equal(b.start) {
		return a.start.Before(b.start)
	}
	return a.rank() < b.rank()
}

// navCells maps each item of the drilled day (daySelectables order) to its
// on-screen position.
func (tg *timeGridView) navCells() []navCell {
	items := tg.daySelectables()
	cells := make([]navCell, len(items))
	placements := model.LayoutDay(tg.timed[dayKey(tg.selected)])
	band := 0
	for i, it := range items {
		switch {
		case isAllDayItem(it):
			cells[i] = navCell{kind: cellBand, lane: band}
			band++
		case it.Todo != nil: // timed due task
			cells[i] = navCell{kind: cellTask, start: it.Start, end: it.Start, lane: 0}
		default: // timed event — lane + end from the overlap layout
			lane, end := 0, it.Start
			for _, p := range placements {
				if p.Occ.Event == it.Event && p.Occ.Start.Equal(it.Start) {
					lane, end = p.Lane, p.Occ.End
					break
				}
			}
			cells[i] = navCell{kind: cellEvent, start: it.Start, end: end, lane: lane}
		}
	}
	return cells
}

func (tg *timeGridView) spatialMove(dir int) {
	if t := tg.spatialTarget(dir); t >= 0 {
		tg.eventIndex = t
		tg.emitEvent()
	}
}

// spatialTarget returns the index (in daySelectables) to move to for dir, or -1
// at an edge.
func (tg *timeGridView) spatialTarget(dir int) int {
	cells := tg.navCells()
	if tg.eventIndex < 0 || tg.eventIndex >= len(cells) {
		return -1
	}
	cur := cells[tg.eventIndex]
	switch dir {
	case navLeft, navRight:
		step := 1
		if dir == navLeft {
			step = -1
		}
		for i, c := range cells {
			switch cur.kind {
			case cellBand:
				if c.kind == cellBand && c.lane == cur.lane+step {
					return i
				}
			case cellEvent:
				if c.kind == cellEvent && c.lane == cur.lane+step && cur.overlaps(c) {
					return i
				}
			}
		}
		return -1 // tasks are single-lane; band/event lane edges stop
	case navDown:
		if cur.kind == cellBand {
			return tg.edgeTimed(cells, cur.lane, true)
		}
		return tg.nearestLevel(cells, cur, true)
	case navUp:
		if cur.kind == cellBand {
			return -1
		}
		if t := tg.nearestLevel(cells, cur, false); t >= 0 {
			return t
		}
		return tg.bandNearest(cells, cur.lane) // top timed row → all-day band
	}
	return -1
}

// nearestLevel finds the timed cell one vertical level below (down) or above cur,
// landing on the lane nearest cur's.
func (tg *timeGridView) nearestLevel(cells []navCell, cur navCell, down bool) int {
	var target navCell
	found := false
	for _, c := range cells {
		if !c.timed() || sameLevel(c, cur) {
			continue
		}
		inDir := levelLess(cur, c)
		if !down {
			inDir = levelLess(c, cur)
		}
		if !inDir {
			continue
		}
		if !found || (down && levelLess(c, target)) || (!down && levelLess(target, c)) {
			target, found = c, true
		}
	}
	if !found {
		return -1
	}
	return laneNearest(cells, func(c navCell) bool { return c.timed() && sameLevel(c, target) }, cur.lane)
}

// edgeTimed returns the earliest timed cell (from the band going down), nearest lane.
func (tg *timeGridView) edgeTimed(cells []navCell, prefer int, _ bool) int {
	var top navCell
	found := false
	for _, c := range cells {
		if c.timed() && (!found || levelLess(c, top)) {
			top, found = c, true
		}
	}
	if !found {
		return -1
	}
	return laneNearest(cells, func(c navCell) bool { return c.timed() && sameLevel(c, top) }, prefer)
}

func (tg *timeGridView) bandNearest(cells []navCell, prefer int) int {
	return laneNearest(cells, func(c navCell) bool { return c.kind == cellBand }, prefer)
}

// laneNearest returns the index of the matching cell whose lane is closest to
// prefer (ties break to the smaller lane).
func laneNearest(cells []navCell, match func(navCell) bool, prefer int) int {
	best := -1
	for i, c := range cells {
		if !match(c) {
			continue
		}
		if best < 0 || laneCloser(c.lane, cells[best].lane, prefer) {
			best = i
		}
	}
	return best
}

func laneCloser(a, b, target int) bool {
	da, db := absInt(a-target), absInt(b-target)
	if da != db {
		return da < db
	}
	return a < b
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func (tg *timeGridView) handleDayMode(ev *tcell.EventKey) {
	move := func(days int) {
		if tg.onSelectDay != nil {
			tg.onSelectDay(tg.selected.AddDate(0, 0, days))
		}
	}
	switch ev.Key() {
	case tcell.KeyLeft:
		move(-1)
	case tcell.KeyRight:
		move(1)
	// Up/Down do nothing un-drilled: days are navigated horizontally (h/l), and
	// you drill in with Enter.
	case tcell.KeyHome: // gg: first day of the view
		if tg.onSelectDay != nil && len(tg.days) > 0 {
			tg.onSelectDay(tg.days[0])
		}
	case tcell.KeyEnd: // G: last day of the view
		if tg.onSelectDay != nil && len(tg.days) > 0 {
			tg.onSelectDay(tg.days[len(tg.days)-1])
		}
	case tcell.KeyEnter:
		tg.enterEventMode()
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
		}
	}
}

func (tg *timeGridView) handleEventMode(ev *tcell.EventKey) {
	items := tg.daySelectables()
	switch ev.Key() {
	case tcell.KeyEscape:
		tg.eventMode = false
	case tcell.KeyUp:
		tg.spatialMove(navUp)
	case tcell.KeyDown:
		tg.spatialMove(navDown)
	case tcell.KeyLeft:
		tg.spatialMove(navLeft)
	case tcell.KeyRight:
		tg.spatialMove(navRight)
	case tcell.KeyHome: // gg: first item of the day
		if len(items) > 0 {
			tg.eventIndex = 0
			tg.emitEvent()
		}
	case tcell.KeyEnd: // G: last item of the day
		if len(items) > 0 {
			tg.eventIndex = len(items) - 1
			tg.emitEvent()
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
	sel := tg.selectedItem()

	// Header: one date per column (selected day reversed).
	for di, day := range tg.days {
		style := tcell.StyleDefault.Foreground(accentColor).Bold(true)
		if model.SameDay(day, tg.selected) {
			style = style.Reverse(true)
		}
		printStyled(screen, colStart+di*colW+1, y, colW-1, day.Format("Mon 2"), style)
	}

	// All-day band. On the selected day, while cycling an all-day event, show that
	// event highlighted so it can be picked like a timed one.
	printStyled(screen, x, y+1, gutterWidth, "all", tcell.StyleDefault.Foreground(adjacentColor))
	for di, day := range tg.days {
		ad := tg.allDay[dayKey(day)]
		adTasks, _ := tg.dueParts(day)
		total := len(ad) + len(adTasks)
		if total == 0 {
			continue
		}
		// Lead with the first all-day event, else the first all-day-due task; the
		// rest collapse into a "+N" count.
		var label string
		var style tcell.Style
		if len(ad) > 0 {
			label = nonEmpty(ad[0].Event.Summary, "(untitled)")
			style = tcell.StyleDefault.Foreground(eventColor)
			if tg.occColor != nil {
				if cc, ok := tg.occColor(ad[0]); ok {
					style = tcell.StyleDefault.Foreground(cc.fg)
				}
			}
		} else {
			label = taskMarkerLabel(adTasks[0], tg.folderTask(adTasks[0]))
			style = tcell.StyleDefault.Foreground(tg.taskFg(adTasks[0]))
		}
		if total > 1 {
			label = fmt.Sprintf("%s +%d", label, total-1)
		}
		// While cycling, if the selected item is an all-day item on this day, show
		// it highlighted so it can be picked like a timed one.
		if sel != nil && model.SameDay(day, tg.selected) && isAllDayItem(*sel) {
			label = nonEmpty(sel.Title, "(untitled)")
			style = selectionStyle
		}
		printStyled(screen, colStart+di*colW+1, y+1, colW-1, label, style)
	}
	for xx := x; xx < x+w; xx++ {
		screen.SetContent(xx, y+2, tcell.RuneHLine, nil, sepStyle)
	}

	bodyY := y + 3
	bodyH := h - 3
	if bodyH < 1 {
		return
	}

	// Hour labels: all 24 hours mapped across the body height. Skip a label when
	// the day is compressed enough that it would land on an already-labelled row.
	lastRow := -1
	for hour := 0; hour < hoursPerDay; hour++ {
		row := bodyY + hour*bodyH/hoursPerDay
		if row == lastRow || row >= bodyY+bodyH {
			continue
		}
		printStyled(screen, x, row, gutterWidth-1, hourLabel(hour), tcell.StyleDefault.Foreground(adjacentColor))
		lastRow = row
	}
	// Column separators.
	for di := 0; di <= n; di++ {
		sx := colStart + di*colW - 1
		if di == 0 {
			sx = colStart - 1
		}
		for yy := bodyY; yy < bodyY+bodyH && yy < y+h; yy++ {
			screen.SetContent(sx, yy, tcell.RuneVLine, nil, sepStyle)
		}
	}

	// Event blocks. In event mode the cycled timed event on the selected day is
	// highlighted.
	for di, day := range tg.days {
		places := model.LayoutDay(tg.timed[dayKey(day)])
		for _, p := range places {
			selected := sel != nil && sel.Event != nil && !sel.Event.AllDay && model.SameDay(day, tg.selected) &&
				p.Occ.Event == sel.Event && p.Occ.Start.Equal(sel.Start)
			tg.drawBlock(screen, p, colStart+di*colW, colW, bodyY, bodyH, selected)
		}
		// Timed due-task markers, drawn on top at their due time; the cycled task on
		// the selected day is highlighted.
		_, timedTasks := tg.dueParts(day)
		for _, t := range timedTasks {
			selected := sel != nil && sel.Todo != nil && sel.Todo.UID == t.UID && model.SameDay(day, tg.selected)
			tg.drawTaskMarker(screen, t, colStart+di*colW, colW, bodyY, bodyH, selected)
		}
	}
}

// isAllDayItem reports whether it sits in the top all-day band (an all-day event
// or an all-day-due task) rather than at a time in the grid body.
func isAllDayItem(it model.AgendaItem) bool {
	return (it.Event != nil && it.Event.AllDay) || (it.Todo != nil && it.Todo.DueAllDay)
}

// drawTaskMarker draws a one-row colored marker for a timed due task at its due
// time in the grid body. It's a foreground marker (no fill), distinguishing a due
// task from the filled event blocks; it may sit over an event block at the same
// time.
func (tg *timeGridView) drawTaskMarker(screen tcell.Screen, t *model.Todo, colX, colW, bodyY, bodyH int, selected bool) {
	due := t.Due.In(time.Local)
	row := bodyY + int(hourFloat(due)*float64(bodyH)/hoursPerDay)
	if row < bodyY {
		row = bodyY
	}
	if row >= bodyY+bodyH {
		row = bodyY + bodyH - 1
	}
	style := tcell.StyleDefault.Foreground(tg.taskFg(t))
	if selected {
		style = selectionStyle
	}
	printStyled(screen, colX, row, colW-1, taskMarkerLabel(t, tg.folderTask(t)), style)
}

func (tg *timeGridView) drawBlock(screen tcell.Screen, p model.Placement, colX, colW, bodyY, bodyH int, selected bool) {
	startT := p.Occ.Start.In(time.Local)
	endT := p.Occ.End.In(time.Local)
	startHF := hourFloat(startT)
	endHF := hourFloat(endT)
	if endHF <= startHF {
		endHF = hoursPerDay // ends at/after midnight: extend to the bottom of the day
	}

	top := bodyY + int(startHF*float64(bodyH)/hoursPerDay)
	bottom := bodyY + int(endHF*float64(bodyH)/hoursPerDay)
	if top < bodyY {
		top = bodyY
	}
	height := bottom - top
	if height < 1 {
		height = 1
	}
	if top+height > bodyY+bodyH {
		height = bodyY + bodyH - top
	}
	if height < 1 {
		return
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
	spanStyle := style.Foreground(tcell.ColorSilver) // dimmed time line on the default block
	if tg.occColor != nil {
		if cc, ok := tg.occColor(p.Occ); ok {
			// The calendar color fills the block; pick a contrasting text color and
			// keep the time line the same (silver is unreadable on light fills).
			text := tcell.ColorBlack
			if cc.dark {
				text = tcell.ColorWhite
			}
			style = tcell.StyleDefault.Background(cc.fg).Foreground(text)
			spanStyle = style
		}
	}
	if selected {
		style = tcell.StyleDefault.Background(accentColor).Foreground(tcell.ColorBlack).Bold(true)
		spanStyle = style
	}
	for yy := top; yy < top+height; yy++ {
		for xx := bx; xx < bx+bw; xx++ {
			screen.SetContent(xx, yy, ' ', nil, style)
		}
	}
	printStyled(screen, bx, top, bw, nonEmpty(p.Occ.Event.Summary, "(untitled)"), style)
	if height >= 2 {
		span := startT.Format("3:04") + "-" + endT.Format("3:04pm")
		printStyled(screen, bx, top+1, bw, span, spanStyle)
	}
}

func hourFloat(t time.Time) float64 {
	return float64(t.Hour()) + float64(t.Minute())/60
}

func hourLabel(hour int) string {
	return time.Date(2000, 1, 1, hour, 0, 0, 0, time.UTC).Format("3pm")
}
