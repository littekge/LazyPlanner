package ui

import (
	"testing"
	"time"

	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
)

// Pass-24 MED, iron-rule violation ("editing a known field preserves everything
// else"): a task's LOCATION was destroyed by two ordinary actions.
//
// TodoDraft carries Location and applyTodo writes it, but neither UI path filled
// it in, and applyTodo's setTextOrDel DELETES a property whose draft field is
// empty. So draftFromTodo omitting it made every quick-set (`sp`, `sd`) erase the
// location, and the task form having no Location input at all made a no-op
// open-and-Save erase it too — while the event form has had the field all along.
//
// Both tests assert the location is set BEFORE acting, so they cannot pass
// vacuously on a task that never had one.
func TestTodoLocationSurvivesQuickSet(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.Local)
	a := newWritableTestApp(t, now)
	a.loc = time.Local
	a.root = tview.NewPages()
	a.setMode(modeTasks)
	a.createTask(a.selectedTasklistID(), "", "Pickup @depot tomorrow")
	td := todoBySummary(a.store, "Pickup")
	if td == nil || td.Location != "depot" {
		t.Fatalf("setup: %+v", td)
	}
	uid := td.UID

	a.applyTodoField(uid, "set priority", func(d *model.TodoDraft) { d.Priority = 3 })

	td2 := todoBySummary(a.store, "Pickup")
	if td2 == nil {
		t.Fatal("gone")
	}
	t.Logf("after quick set: Priority=%d Location=%q", td2.Priority, td2.Location)
	if td2.Location != "depot" {
		t.Errorf("LOCATION erased by quick set: got %q", td2.Location)
	}
}

func TestTodoLocationSurvivesFormRoundtrip(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.Local)
	a := newWritableTestApp(t, now)
	a.loc = time.Local
	a.root = tview.NewPages()
	a.setMode(modeTasks)
	a.createTask(a.selectedTasklistID(), "", "Pickup @depot tomorrow")
	td := todoBySummary(a.store, "Pickup")
	uid := td.UID
	loc, _ := a.store.Locate(uid)

	_, fields := a.newTodoForm(td, nil)
	d, err := a.readTodoDraft(fields)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := model.EditTodo(loc.Object, uid, d, a.now, a.loc)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("form roundtrip Location=%q", obj.Todos[0].Location)
	if obj.Todos[0].Location != "depot" {
		t.Errorf("LOCATION erased by form save: got %q", obj.Todos[0].Location)
	}
}
