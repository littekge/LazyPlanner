package ui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// nastyStrings are display-hostile summaries the custom-drawn widgets must render
// without panicking or hanging: over-long text, double-width CJK and emoji,
// zero-width combining marks, RTL scripts, tabs/control-ish runes, and a lone
// wide rune. Rune-width miscalculations in a hand-drawn grid can slice past a
// cell boundary (panic) or loop (hang), so these probe exactly that.
var nastyStrings = []string{
	strings.Repeat("A", 3000),
	strings.Repeat("会議", 500),
	strings.Repeat("🎉🎊✅📅", 300),
	"ạ́̈́combining" + strings.Repeat("́", 200),
	"مرحبا بالعالم שלום עולם RTL mixed",
	"tab\tvtab\vcr\rmix",
	"世界",
	strings.Repeat("🇺🇸", 100), // regional-indicator flag pairs
	"",
}

// drawGeom draws p at w×h on a fresh simulation screen, failing the test if the
// draw panics or does not finish within the watchdog. A panic in any Draw path
// would crash the live app, so containment here is the property under test.
func drawGeom(t *testing.T, label string, p tview.Primitive, w, h int) {
	t.Helper()
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("init screen: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(w, h)
	p.SetRect(0, 0, w, h)

	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		p.Draw(screen)
	}()
	select {
	case r := <-done:
		if r != nil {
			t.Fatalf("%s: Draw panicked at %dx%d: %v", label, w, h, r)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("%s: Draw hung at %dx%d", label, w, h)
	}
}

var stressGeoms = []struct{ w, h int }{
	{1, 1}, {2, 2}, {1, 24}, {24, 1}, {3, 3}, {5, 5},
	{10, 4}, {20, 8}, {40, 12}, {80, 24}, {200, 60}, {400, 150},
}

// putTodo/putEvent inject content with an exact summary (bypassing quick-add's
// tokenizer) so the hostile strings reach the renderer verbatim.
func putTodo(t *testing.T, a *app, calID, parentUID, summary string, due time.Time, hasDue bool) string {
	t.Helper()
	d := model.TodoDraft{Summary: summary, ParentUID: parentUID}
	if hasDue {
		d.HasDue, d.Due = true, due
	}
	obj := model.NewTodoObject(d, a.now)
	uid := obj.Todos[0].UID
	if _, err := a.store.Put(context.Background(), calID, store.ResourceName(uid), obj); err != nil {
		t.Fatal(err)
	}
	return uid
}

func putEvent(t *testing.T, a *app, calID, summary string, start time.Time, allDay bool) {
	t.Helper()
	end := start.Add(time.Hour)
	if allDay {
		end = start.AddDate(0, 0, 1)
	}
	obj, err := model.NewEventObject(model.EventDraft{Summary: summary, Start: start, End: end, AllDay: allDay}, a.now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.Put(context.Background(), calID, store.ResourceName(obj.Events[0].UID), obj); err != nil {
		t.Fatal(err)
	}
}

// TestDisplayStress drives every view into a hostile-content state and draws the
// full layout across a matrix of terminal geometries — from 1×1 up to 400×150 —
// asserting no custom Draw path panics or hangs.
func TestDisplayStress(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	cal := a.selectedCalendarID()
	list := a.selectedTasklistID()
	day := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)

	// Pile hostile content onto a single day and into the task tree.
	for i, s := range nastyStrings {
		putEvent(t, a, cal, s, day.Add(time.Duration(i)*time.Minute), false)
		putEvent(t, a, cal, "allday:"+s, day, true)
		putTodo(t, a, list, "", s, day.Add(time.Duration(i)*time.Hour), true)
	}
	// Many overlapping events on one day (month-cell overflow + time-grid lanes).
	for i := 0; i < 150; i++ {
		putEvent(t, a, cal, fmt.Sprintf("evt %d 会議🎉", i), day.Add(time.Duration(i%24)*time.Hour), false)
	}
	// A deep subtask chain and a wide sibling fan-out.
	parent := ""
	for i := 0; i < 30; i++ {
		parent = putTodo(t, a, list, parent, fmt.Sprintf("deep-%d-%s", i, nastyStrings[i%len(nastyStrings)]), time.Time{}, false)
	}
	for i := 0; i < 300; i++ {
		putTodo(t, a, list, "", fmt.Sprintf("flat-%d", i), time.Time{}, false)
	}
	a.reload()

	// Reach each interaction state via the real key paths, drawing the whole
	// layout at every geometry in each.
	states := []struct {
		name  string
		enter func()
	}{
		{"tasks", func() { a.globalKeys(runeKey('t')); a.buildTree(); expandAllNodes(a.tree.GetRoot()) }},
		{"calendar-month", func() { a.globalKeys(runeKey('c')) }},
		{"calendar-week", func() { a.globalKeys(runeKey('c')); a.globalKeys(runeKey('v')) }},
		{"calendar-day", func() { a.globalKeys(runeKey('c')); a.globalKeys(runeKey('v')); a.globalKeys(runeKey('v')) }},
		{"calendar-day-drilled", func() {
			a.globalKeys(runeKey('c'))
			a.globalKeys(runeKey('v'))
			a.globalKeys(runeKey('v'))
			a.globalKeys(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
		}},
		{"calendar-month-drilled", func() {
			a.globalKeys(runeKey('c'))
			a.globalKeys(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
		}},
		{"agenda", func() { a.globalKeys(runeKey('a')) }},
		{"accordion-collapsed", func() {
			a.globalKeys(runeKey('t'))
			a.buildTree()
			a.setAccordion(true)
		}},
	}

	for _, st := range states {
		st.enter()
		spray(a) // move the selection/drill to extremes so scroll/window math is exercised
		for _, g := range stressGeoms {
			drawGeom(t, st.name, a.root, g.w, g.h)
		}
	}

	// SELECT-mode range active: the range visuals (tree reverse-video, month/
	// time-grid day-range boxes, drilled-item highlighting) must survive the
	// same hostile content and geometries — new draw branches are a new
	// freeze/panic surface. 'V' starts the range where the state above left the
	// cursor, then spray extends it to the extremes before drawing.
	//
	// V only enters SELECT with the selection surface itself focused (Fix 1) —
	// c/t alone leave focus on the overview list, so each state explicitly
	// focuses the tree/grid first, matching the real workflow of Enter-ing into
	// it from the overview.
	selectStates := []struct {
		name  string
		enter func()
	}{
		{"select-tasks", func() {
			a.globalKeys(runeKey('t'))
			a.buildTree()
			expandAllNodes(a.tree.GetRoot())
			a.setFocus(a.tree)
			a.globalKeys(runeKey('V'))
		}},
		{"select-calendar-month", func() {
			a.globalKeys(runeKey('c'))
			a.setFocus(a.calendarPrimitive())
			a.globalKeys(runeKey('V'))
		}},
		{"select-calendar-week", func() {
			a.globalKeys(runeKey('c'))
			a.globalKeys(runeKey('v'))
			a.setFocus(a.calendarPrimitive())
			a.globalKeys(runeKey('V'))
		}},
		{"select-calendar-month-drilled", func() {
			a.globalKeys(runeKey('c'))
			a.globalKeys(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
			a.setFocus(a.calendarPrimitive())
			a.globalKeys(runeKey('V'))
		}},
		{"select-calendar-day-drilled", func() {
			a.globalKeys(runeKey('c'))
			a.globalKeys(runeKey('v'))
			a.globalKeys(runeKey('v'))
			a.globalKeys(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
			a.setFocus(a.calendarPrimitive())
			a.globalKeys(runeKey('V'))
		}},
	}
	for _, st := range selectStates {
		st.enter()
		spray(a)
		for _, g := range stressGeoms {
			drawGeom(t, st.name, a.root, g.w, g.h)
		}
		a.exitSelect()
	}
}

// spray presses a burst of navigation keys so the highlight/drill lands on
// hostile items and at the edges of the current view, exercising the scroll and
// window math (not just the initial position) before the draw.
func spray(a *app) {
	keys := []*tcell.EventKey{
		tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone),
		tcell.NewEventKey(tcell.KeyEnd, 0, tcell.ModNone),
		tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone),
		tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone),
		runeKey('G'),
	}
	for n := 0; n < 40; n++ {
		a.globalKeys(keys[n%len(keys)])
	}
}

// TestEditFormStress opens the edit form and quick-add line over a hostile item
// (a 3000-char / emoji summary prefilled into the fields) and draws the modal
// across the geometry matrix — the caretForm Draw path.
func TestEditFormStress(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	list := a.selectedTasklistID()
	putTodo(t, a, list, "", strings.Repeat("A", 3000)+" 会議🎉 שלום", time.Time{}, false)
	a.reload()
	a.globalKeys(runeKey('t'))
	a.buildTree()
	a.globalKeys(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)) // select the hostile task

	for _, open := range []struct {
		name string
		key  *tcell.EventKey
	}{
		{"edit-form", runeKey('e')},
		{"quick-add-task", runeKey('t')}, // via the i-prefix below
	} {
		if open.name == "quick-add-task" {
			a.globalKeys(runeKey('i'))
		}
		a.globalKeys(open.key)
		for _, g := range stressGeoms {
			drawGeom(t, open.name, a.root, g.w, g.h)
		}
		a.globalKeys(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	}
}

var shortGeoms = []struct{ w, h int }{
	{1, 1}, {3, 2}, {10, 3}, {20, 4}, {40, 6}, {80, 10}, {120, 40},
}

// TestMonthGridDrillScrollStress drills into a day holding 150 hostile items and
// cycles past the end, then draws at tiny heights — exercising the month cell's
// scroll window and "+N more" math at its extreme, with double-width content.
func TestMonthGridDrillScrollStress(t *testing.T) {
	cv := newCalendarView()
	day := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	start := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	var weeks [][]time.Time
	for w := 0; w < 6; w++ {
		var row []time.Time
		for d := 0; d < 7; d++ {
			row = append(row, start.AddDate(0, 0, w*7+d))
		}
		weeks = append(weeks, row)
	}
	items := map[string][]model.AgendaItem{dayKey(day): hostileAgendaItems(day, 150)}
	cv.setData(weeks, items, time.July, day, day, true)

	handle := cv.InputHandler()
	handle(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
	for i := 0; i < 200; i++ {
		handle(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), func(tview.Primitive) {})
	}
	for _, g := range shortGeoms {
		drawGeom(t, "month-drill", cv, g.w, g.h)
	}
}

// TestTimeGridDrillScrollStress drills into a day of 150 hostile events, walks to
// the far edge in both axes, and draws at 1–3-row heights — the time-grid's
// scroll-to-drilled-item window math at its extreme.
func TestTimeGridDrillScrollStress(t *testing.T) {
	tg := newTimeGridView()
	day := time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local)
	occs := make([]model.Occurrence, 150)
	items := make([]model.AgendaItem, 150)
	for i := 0; i < 150; i++ {
		ev := &model.Event{
			Summary: fmt.Sprintf("%d-%s", i, nastyStrings[i%len(nastyStrings)]),
			Start:   day.Add(time.Duration(i%24)*time.Hour + time.Duration(i)*time.Minute),
		}
		occs[i] = model.Occurrence{Start: ev.Start, End: ev.Start.Add(time.Hour), Event: ev}
		items[i] = model.AgendaItem{Start: ev.Start, Title: ev.Summary, Event: ev}
	}
	tg.setData([]time.Time{day}, map[string][]model.Occurrence{dayKey(day): occs}, nil, day, day)
	tg.items = map[string][]model.AgendaItem{dayKey(day): items}

	handle := tg.InputHandler()
	handle(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
	for i := 0; i < 300; i++ {
		handle(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), func(tview.Primitive) {})
		handle(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone), func(tview.Primitive) {})
	}
	for _, g := range shortGeoms {
		drawGeom(t, "timegrid-drill", tg, g.w, g.h)
	}
	// Also zoom the hour-row height to extremes while drilled, then redraw.
	for i := 0; i < 60; i++ {
		handle(runeKey('+'), func(tview.Primitive) {})
	}
	for _, g := range shortGeoms {
		drawGeom(t, "timegrid-drill-zoomed", tg, g.w, g.h)
	}
}

func hostileAgendaItems(day time.Time, n int) []model.AgendaItem {
	out := make([]model.AgendaItem, n)
	for i := 0; i < n; i++ {
		ev := &model.Event{Summary: fmt.Sprintf("%d-%s", i, nastyStrings[i%len(nastyStrings)]), Start: day.Add(time.Duration(i%24) * time.Hour)}
		out[i] = model.AgendaItem{Start: ev.Start, Title: ev.Summary, Event: ev}
	}
	return out
}

// TestColorPickerStress draws the swatch picker across the geometry matrix — it
// is a custom Draw path not reached by the main layout.
func TestColorPickerStress(t *testing.T) {
	picker := newColorPicker()
	picker.preselect("#3366cc")
	for _, g := range stressGeoms {
		drawGeom(t, "colorpicker", picker, g.w, g.h)
	}
}

// TestAccountPickerStress draws the :account picker across the geometry matrix
// with hostile account names, since it is a modal overlay not reached by the main
// layout's draw paths.
func TestAccountPickerStress(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC))
	a.accounts = []string{"personal", nastyStrings[0], "work", nastyStrings[len(nastyStrings)-1]}
	a.activeAccount = "work"
	list := a.accountPickerList()
	for _, g := range stressGeoms {
		drawGeom(t, "accountpicker", list, g.w, g.h)
	}
}
