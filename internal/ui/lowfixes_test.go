package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// putTimedEvent writes a one-hour VEVENT into calID and returns its UID.
func putTimedEvent(t *testing.T, a *app, calID, summary string, start time.Time) string {
	t.Helper()
	uid := summary + "@ev"
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\nBEGIN:VEVENT\r\nUID:" + uid +
		"\r\nSUMMARY:" + summary + "\r\nDTSTAMP:20260701T000000Z\r\nDTSTART:" + start.UTC().Format("20060102T150405Z") +
		"\r\nDTEND:" + start.Add(time.Hour).UTC().Format("20060102T150405Z") +
		"\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	parsed, err := model.Decode([]byte(ics), time.Local)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.Put(context.Background(), calID, store.ResourceName(uid), parsed); err != nil {
		t.Fatal(err)
	}
	return uid
}

// TestTimeGridDrilledAllDayTaskKeepsMarker locks L3: a selected (drilled) all-day
// due task in the top band must keep its [ ]/▸ marker — it was overwritten with a
// bare title while selected.
func TestTimeGridDrilledAllDayTaskKeepsMarker(t *testing.T) {
	tg := newTimeGridView()
	day := time.Date(2026, 7, 4, 0, 0, 0, 0, time.Local)
	allDayTask := &model.Todo{UID: "t2", Summary: "Renewpass", HasDue: true, DueAllDay: true, Due: day}
	tg.setData([]time.Time{day}, nil, nil, day, day)
	tg.dueTasks = map[string][]*model.Todo{dayKey(day): {allDayTask}}
	// items drives the drill selection (daySelectables); the all-day task is its
	// sole entry.
	tg.items = map[string][]model.AgendaItem{dayKey(day): {{Start: day, AllDay: true, Title: allDayTask.Summary, Todo: allDayTask}}}

	tg.reDrill(day, 0) // drill onto the all-day task (all-day items come first)
	sel := tg.selectedItem()
	if sel == nil || !sel.IsTodo() || sel.Todo.UID != "t2" {
		t.Fatalf("expected the all-day task selected, got %+v", sel)
	}

	out := renderPrimitive(t, tg, 100, 40)
	if !strings.Contains(out, "[ ] Renewpass") {
		t.Errorf("drilled all-day due task lost its checkbox marker:\n%s", out)
	}
}

// TestGrabTimeHintIsModeAware locks L4: the "how to change the time" grab hint
// names `v` only in calendar mode (where v cycles to week/day); in agenda mode v
// is a no-op, so the hint must not name it.
func TestGrabTimeHintIsModeAware(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))

	a.mode = modeCalendar
	if h := a.grabTimeHint("change the time"); !strings.Contains(h, "(v)") {
		t.Errorf("calendar-mode hint = %q, want it to name (v)", h)
	}
	a.mode = modeAgenda
	if h := a.grabTimeHint("change the time"); strings.Contains(h, "(v)") {
		t.Errorf("agenda-mode hint = %q, must not name the dead key v", h)
	}
}

// TestSpaceOnDrilledEventFlashes locks L5: Space on a drilled event flashes
// (events can't be completed) instead of silently flipping a calendar's visibility.
func TestSpaceOnDrilledEventFlashes(t *testing.T) {
	when := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, when)
	if err := a.store.CreateCalendarLocal(context.Background(), "ev", store.CalendarMeta{DisplayName: "EV"}, []string{"VEVENT"}); err != nil {
		t.Fatal(err)
	}
	putTimedEvent(t, a, "ev", "Standup", time.Date(2026, 7, 20, 9, 0, 0, 0, time.Local))
	a.reload()

	a.setMode(modeCalendar)
	a.viewMode = viewMonth
	a.anchor = model.DayStart(when)
	a.buildCenterCalendar()
	a.month.selected = a.anchor
	items := a.month.selectedItems()
	idx := -1
	for i, it := range items {
		if !it.IsTodo() {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("event not present in the day's items: %+v", items)
	}
	a.month.eventMode = true
	a.month.eventIndex = idx

	calID := a.selectedCalendarID()
	hiddenBefore := a.hidden[calID]
	a.globalKeys(runeKey(' '))

	if a.hidden[calID] != hiddenBefore {
		t.Error("Space on a drilled event toggled calendar visibility; it should not")
	}
	if got := a.statusLeft.GetText(true); !strings.Contains(got, "Can't complete an event") {
		t.Errorf("status = %q, want the \"Can't complete an event\" flash", got)
	}
}

// TestSpaceOnAgendaEventFlashes locks the key-contract rule: Space on an event
// in Agenda mode must flash "Can't complete an event", not silently no-op the
// way toggleComplete used to for a non-task target.
func TestSpaceOnAgendaEventFlashes(t *testing.T) {
	when := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, when)
	if err := a.store.CreateCalendarLocal(context.Background(), "ev", store.CalendarMeta{DisplayName: "EV"}, []string{"VEVENT"}); err != nil {
		t.Fatal(err)
	}
	putTimedEvent(t, a, "ev", "Standup", time.Date(2026, 7, 20, 9, 0, 0, 0, time.Local))
	a.reload()

	a.setMode(modeAgenda)
	a.buildAgendaLeft()
	items := a.dayItems(model.DayStart(when))
	idx := -1
	for i, it := range items {
		if !it.IsTodo() {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("event not present in today's agenda: %+v", items)
	}
	a.agendaList.SetCurrentItem(idx)

	a.globalKeys(runeKey(' '))
	if got := a.statusLeft.GetText(true); !strings.Contains(got, "Can't complete an event") {
		t.Errorf("status = %q, want the \"Can't complete an event\" flash", got)
	}
}

// TestAgendaSelectionBoxFollowsFocus locks L1: the agenda selection box tracks
// focus (via the active closure) like the calendar day box, rather than being
// hardwired to the focused color.
func TestAgendaSelectionBoxFollowsFocus(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	if a.agenda.active == nil {
		t.Fatal("agenda.active closure is not wired")
	}
	a.setMode(modeAgenda) // focuses the agenda left list
	if !a.agenda.active() {
		t.Error("agenda box should read active while the agenda list is focused")
	}
	a.setMode(modeTasks) // focus moves off the agenda list
	if a.agenda.active() {
		t.Error("agenda box should read inactive when the agenda list is not focused")
	}
}
