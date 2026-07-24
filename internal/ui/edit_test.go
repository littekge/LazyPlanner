package ui

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// newWritableTestApp copies the store fixture into a temp dir and opens the app
// over it, so editing tests can mutate the cache without touching the fixture.
func newWritableTestApp(t *testing.T, now time.Time) *app {
	t.Helper()
	dir := t.TempDir()
	copyTree(t, "../store/testdata/vdir", dir)
	s, err := store.Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	a := newApp(s, "test", now)
	a.build()
	a.reload()
	return a
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
}

func TestCreateTaskAndUndo(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)

	calID := a.selectedTasklistID()
	if calID == "" {
		t.Fatal("no task list selected")
	}
	a.createTask(calID, "", "Write docs tomorrow !2 #work")

	td := todoBySummary(a.store, "Write docs")
	if td == nil {
		t.Fatal("created task not found in store")
	}
	if td.Priority != 2 {
		t.Errorf("priority = %d, want 2", td.Priority)
	}
	if !td.HasDue || !td.DueAllDay {
		t.Errorf("due = %+v, want an all-day due", td)
	}
	if len(td.Categories) != 1 || td.Categories[0] != "work" {
		t.Errorf("categories = %v, want [work]", td.Categories)
	}

	a.undoLast()
	if todoBySummary(a.store, "Write docs") != nil {
		t.Error("undo did not remove the created task")
	}
}

func TestToggleCompleteAndUndo(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)
	a.selectTreeByUID("grocery@lazyplanner.test")

	a.toggleComplete()
	if loc, ok := a.store.Locate("grocery@lazyplanner.test"); !ok || !findTodo(loc.Object, "grocery@lazyplanner.test").Completed() {
		t.Fatal("task not completed after toggle")
	}

	a.undoLast()
	loc, ok := a.store.Locate("grocery@lazyplanner.test")
	if !ok || findTodo(loc.Object, "grocery@lazyplanner.test").Completed() {
		t.Error("undo did not restore the incomplete state")
	}
}

// TestUndoWhileDrilledKeepsDrill reproduces the v1.5.0 phase-2 bug where undoing
// a mutation while drilled into a calendar day (event-cycling) silently kicked
// the user back out to day navigation. Every sibling mutation path (Space-complete
// via refreshKeepingDrill, delete via refresh(""), form-save via the modal
// captureFocus/restoreFocus stack) preserves the grid's drill across its refresh;
// undoLast passed the mutation's non-empty selUID straight to plain refresh,
// which only preserves drill when selUID == "" — dropping it here.
func TestUndoWhileDrilledKeepsDrill(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeCalendar)
	a.viewMode = viewDay
	a.anchor = model.DayStart(now)

	calID := a.selectedTasklistID()
	if calID == "" {
		t.Fatal("no task list selected")
	}
	a.createTask(calID, "", "Errand today")
	errand := todoBySummary(a.store, "Errand")
	if errand == nil {
		t.Fatal("seed task missing")
	}

	a.buildCenterCalendar()
	g, ok := a.calendarPrimitive().(calGrid)
	if !ok {
		t.Fatal("calendar primitive is not a calGrid")
	}

	// Drill onto the task's row (its position among the anchor day's items).
	items := a.dayItems(a.anchor)
	idx := -1
	for i, it := range items {
		if it.Todo != nil && it.Todo.UID == errand.UID {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatal("task not present among the drilled day's items")
	}
	g.reDrill(a.anchor, idx)
	if _, drilled, _ := g.drillState(); !drilled {
		t.Fatal("precondition: not drilled")
	}

	a.toggleComplete()
	if !todoBySummary(a.store, "Errand").Completed() {
		t.Fatal("task did not complete")
	}
	g2, ok := a.calendarPrimitive().(calGrid)
	if !ok {
		t.Fatal("calendar primitive missing after toggle")
	}
	if _, drilled, _ := g2.drillState(); !drilled {
		t.Fatal("toggle-complete should have kept the drill (sibling path) — test setup is broken")
	}

	a.undoLast()
	if todoBySummary(a.store, "Errand").Completed() {
		t.Error("undo did not restore the incomplete state")
	}
	g3, ok := a.calendarPrimitive().(calGrid)
	if !ok {
		t.Fatal("calendar primitive missing after undo")
	}
	if _, drilled, _ := g3.drillState(); !drilled {
		t.Error("undoLast kicked the user out of the drilled calendar day (drill state dropped)")
	}
}

func TestReparentIndentAndUndo(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)
	calID := a.selectedTasklistID()

	// Two root siblings; titles sort so "Alpha" precedes "Beta".
	a.createTask(calID, "", "Alpha")
	a.createTask(calID, "", "Beta")

	alpha := todoBySummary(a.store, "Alpha")
	beta := todoBySummary(a.store, "Beta")
	if alpha == nil || beta == nil {
		t.Fatal("seed tasks missing")
	}

	a.selectTreeByUID(beta.UID)
	a.reparentSelected(indent)

	moved := todoBySummary(a.store, "Beta")
	if moved.ParentUID != alpha.UID {
		t.Errorf("Beta.ParentUID = %q, want %q (Alpha)", moved.ParentUID, alpha.UID)
	}

	a.undoLast()
	if got := todoBySummary(a.store, "Beta").ParentUID; got != "" {
		t.Errorf("undo did not clear the parent: %q", got)
	}
}

// TestReparentUsesOnScreenSibling locks the H/L fix: indent must nest under the
// task shown directly above — even a just-completed (sticky) one — rather than a
// separately-rebuilt forest that omits it.
func TestReparentUsesOnScreenSibling(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)
	a.showCompleted = false
	calID := a.selectedTasklistID()

	// Names sort after the fixture's "Buy groceries" so ordering is A, B, C.
	a.createTask(calID, "", "Task A")
	a.createTask(calID, "", "Task B")
	a.createTask(calID, "", "Task C")

	// Complete Task B while hidden -> it stays visible (sticky) directly above C.
	a.selectTreeByUID(todoBySummary(a.store, "Task B").UID)
	a.toggleComplete()

	c := todoBySummary(a.store, "Task C")
	a.selectTreeByUID(c.UID)
	a.reparentSelected(indent)

	got := todoBySummary(a.store, "Task C").ParentUID
	want := todoBySummary(a.store, "Task B").UID // the row shown above C
	if got != want {
		t.Errorf("indent nested under %q, want the on-screen row above (Task B = %q)", got, want)
	}
}

func TestFolderBlocksCompletionUntilChildrenDone(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)
	calID := a.selectedTasklistID()

	a.createTask(calID, "", "Parent")
	parent := todoBySummary(a.store, "Parent")
	a.createTask(calID, parent.UID, "Child")
	child := todoBySummary(a.store, "Child")

	// Parent is now a folder — completing it is blocked.
	a.selectTreeByUID(parent.UID)
	a.toggleComplete()
	if todoBySummary(a.store, "Parent").Completed() {
		t.Fatal("a folder with an incomplete child should not complete")
	}

	// Complete the child; the parent is no longer a folder and can complete.
	a.selectTreeByUID(child.UID)
	a.toggleComplete()
	if !todoBySummary(a.store, "Child").Completed() {
		t.Fatal("child did not complete")
	}
	a.selectTreeByUID(parent.UID)
	a.toggleComplete()
	if !todoBySummary(a.store, "Parent").Completed() {
		t.Error("parent should complete once its children are done")
	}
}

func TestStickyKeepsCompletedVisibleUntilLeavingList(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)
	calID := a.selectedTasklistID()
	a.showCompleted = false

	a.createTask(calID, "", "Solo")
	solo := todoBySummary(a.store, "Solo")
	a.selectTreeByUID(solo.UID)
	a.toggleComplete() // completed while hidden → should stay visible

	if findTreeNode(a.tree.GetRoot(), solo.UID) == nil {
		t.Fatal("just-completed task should remain visible in the list")
	}

	// Leaving the list (switching panes) drops the pin, hiding it.
	a.setMode(modeCalendar)
	a.setMode(modeTasks)
	if findTreeNode(a.tree.GetRoot(), solo.UID) != nil {
		t.Error("completed task should be hidden after leaving the list")
	}
}

// TestCycleCalendar guards the [ / ] calendar cycling (the keys the misleading
// "[/]" hint was about — it is not the / key).
func TestCycleCalendar(t *testing.T) {
	a := newWritableTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	a.setMode(modeCalendar)
	if a.calendars.GetItemCount() < 2 {
		t.Skip("need at least two calendars to cycle")
	}
	before := a.calendars.GetCurrentItem()
	a.cycleCalendar(1)
	if a.calendars.GetCurrentItem() == before {
		t.Error("cycleCalendar did not move the calendar highlight")
	}
}

// TestStickyWorksOnNonFirstList reproduces the bug where sticky-complete only
// worked for the first task list: completing a task in a later list must keep it
// visible too, despite the panel rebuild parking selection at index 0.
func TestStickyWorksOnNonFirstList(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)
	a.showCompleted = false

	// Fixture "work" holds only events; add a todo so it becomes a second list.
	// Calendars sort by id: "personal" (0) then "work" (1).
	a.createTask("work", "", "WorkTask")
	if len(a.tasklistIDs) < 2 {
		t.Fatalf("expected two task lists, got %v", a.tasklistIDs)
	}
	workIdx := -1
	for i, id := range a.tasklistIDs {
		if id == "work" {
			workIdx = i
		}
	}
	if workIdx <= 0 {
		t.Fatalf("work should be a non-first list, got index %d", workIdx)
	}

	// Select the work list and complete its task.
	a.tasklists.SetCurrentItem(workIdx)
	a.buildTree()
	work := todoBySummary(a.store, "WorkTask")
	a.selectTreeByUID(work.UID)
	a.toggleComplete()

	if findTreeNode(a.tree.GetRoot(), work.UID) == nil {
		t.Error("completed task on a non-first list should stay visible (sticky)")
	}
}

func TestDescendants(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)
	calID := a.selectedTasklistID()

	a.createTask(calID, "", "Root")
	root := todoBySummary(a.store, "Root")
	a.createTask(calID, root.UID, "Mid")
	mid := todoBySummary(a.store, "Mid")
	a.createTask(calID, mid.UID, "Leaf")

	if got := len(a.descendants(root.UID)); got != 2 {
		t.Errorf("descendants(Root) = %d, want 2 (Mid, Leaf)", got)
	}
}
