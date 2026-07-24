package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// selectTreeRangeAll is a test helper: enter SELECT anchored on the first tree
// row and extend the cursor to the last.
func selectTreeRangeAll(t *testing.T, a *app) {
	t.Helper()
	nodes := visibleTreeNodes(a.tree.GetRoot())
	if len(nodes) == 0 {
		t.Fatal("empty tree")
	}
	a.tree.SetCurrentNode(nodes[0])
	a.enterSelect()
	a.tree.SetCurrentNode(nodes[len(nodes)-1])
}

// confirmYes drives an open a.confirm/confirmOK modal's affirmative button.
// tview.Modal.Focus delegates onward to its internal form, so
// a.tv.SetFocus(modal) leaves the application's actual focus on the first
// *tview.Button (the affirmative one — AddButtons put it first), not the
// Modal itself. Pressing Enter there activates it, which tview.Modal's
// SetDoneFunc turns into the onYes callback.
func confirmYes(t *testing.T, a *app) {
	t.Helper()
	if !a.root.HasPage(pageConfirm) {
		t.Fatal("expected a confirm modal open")
	}
	btn, ok := a.tv.GetFocus().(*tview.Button)
	if !ok {
		t.Fatalf("focus is %T, want the confirm modal's affirmative *tview.Button", a.tv.GetFocus())
	}
	btn.InputHandler()(keyEv(tcell.KeyEnter), func(tview.Primitive) {})
}

// TestBulkCompleteRange: every incomplete task in the range completes; the op
// is one undo step; SELECT exits.
func TestBulkCompleteRange(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	var uids []string
	for _, s := range []string{"A", "B", "C"} {
		uids = append(uids, putTodo(t, a, testCalID(a), "", "task "+s, now, true))
	}
	a.refresh(uids[0])
	a.setFocus(a.tree)
	undoBefore := len(a.undo)
	selectTreeRangeAll(t, a)
	a.bulkComplete()

	if a.selecting {
		t.Fatal("bulk complete must exit SELECT")
	}
	for _, u := range uids {
		loc, _ := a.store.Locate(u)
		if td := findTodo(loc.Object, u); td == nil || !td.Completed() {
			t.Fatalf("task %s not completed", u)
		}
	}
	if len(a.undo) != undoBefore+1 {
		t.Fatalf("bulk complete must push exactly one undo step, got %d", len(a.undo)-undoBefore)
	}
	a.undoLast()
	for _, u := range uids {
		loc, _ := a.store.Locate(u)
		if td := findTodo(loc.Object, u); td == nil || td.Completed() {
			t.Fatalf("undo must reopen task %s", u)
		}
	}
}

// TestBulkCompleteFolderChildFirst: processing runs children-first (reverse
// visible order), so selecting a folder together with its only incomplete
// child completes both — the child's completion un-folders the parent.
func TestBulkCompleteFolderChildFirst(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	parent := putTodo(t, a, testCalID(a), "", "folder", now, true)
	child := putTodo(t, a, testCalID(a), parent, "leaf", now, true)
	a.refresh(parent)
	a.setFocus(a.tree)
	selectTreeRangeAll(t, a)
	a.bulkComplete()

	for _, u := range []string{parent, child} {
		loc, _ := a.store.Locate(u)
		if td := findTodo(loc.Object, u); td == nil || !td.Completed() {
			t.Fatalf("%s not completed (children-first ordering broken)", u)
		}
	}
}

// TestBulkCompleteSkips: events and already-done tasks are skipped and counted;
// a recurring todo advances instead of completing (single-item semantics).
func TestBulkCompleteSkips(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	putEvent(t, a, testCalID(a), "meeting", now, false)
	due := putTodo(t, a, testCalID(a), "", "due today", now, true)
	recurring := putRecurringTodo(t, a, testCalID(a), "standup", now, "FREQ=DAILY")
	a.refresh("")
	a.month.reDrill(model.DayStart(now), 0)
	a.enterSelect()
	items := a.dayItems(model.DayStart(now))
	a.month.eventIndex = len(items) - 1 // extend over the whole day
	a.bulkComplete()

	loc, _ := a.store.Locate(due)
	if td := findTodo(loc.Object, due); td == nil || !td.Completed() {
		t.Fatal("plain task must complete")
	}
	loc, _ = a.store.Locate(recurring)
	if td := findTodo(loc.Object, recurring); td == nil || td.Completed() {
		t.Fatal("recurring todo must advance, not complete")
	} else if !td.Due.After(now) {
		t.Fatalf("recurring todo due must advance past %v, got %v", now, td.Due)
	}
	if s := a.statusLeft.GetText(true); !strings.Contains(s, "skipped") {
		t.Fatalf("summary must report the skipped event, got %q", s)
	}
}

// TestBulkCompleteStaleRollsBack: a write that fails mid-batch rolls back the
// items already written — all-or-nothing, no partial completion and no undo
// step.
//
// Note on the mechanism: bulkComplete re-Locates each item immediately before
// writing it (the safe pattern — a write always checks against the *current*
// store state right before it happens). That means a mutation made entirely
// *before* bulkComplete is called can never be observed as "stale" by it: by
// the time bulkComplete reads the item, that mutation already IS the current
// state, so there's no version mismatch to detect — a synchronous, single-
// threaded test cannot manufacture a CAS mismatch against a function that only
// ever compares to its own fresh read. Genuine mid-batch staleness would
// require a real concurrent writer racing the loop's Locate/PutIfUnchanged
// pair, which is inherently non-deterministic and unsuitable for a gate test.
//
// So this test exercises the identical rollback code path (PutIfUnchanged/the
// write returning a non-nil error, not the CAS itself) via the codebase's
// established deterministic-write-failure idiom: plant a directory at the
// exact path the store is about to rename a file onto, so the write fails for
// real (see TestCommitSplitRollsBackMasterOnFutureWriteFailure). Reverse
// visible order writes task B first, then fails on task A — proving the
// already-written B is rolled back and no undo step is pushed.
func TestBulkCompleteStaleRollsBack(t *testing.T) {
	dataDir := t.TempDir()
	s, err := store.Open(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newApp(s, "test", now)
	a.build()
	if err := s.CreateCalendarLocal(context.Background(), "tasks", store.CalendarMeta{DisplayName: "Tasks"}, []string{"VTODO"}); err != nil {
		t.Fatal(err)
	}
	a.reload()
	a.setMode(modeTasks)

	u1 := putTodo(t, a, "tasks", "", "task A", now, true)
	u2 := putTodo(t, a, "tasks", "", "task B", now, true)
	a.refresh(u1)
	a.setFocus(a.tree)
	undoBefore := len(a.undo)
	selectTreeRangeAll(t, a)

	// Reverse visible order processes B (u2) first, then A (u1) — plant the
	// blocker under A's resource path so its write is the one that fails.
	loc, ok := a.store.Locate(u1)
	if !ok {
		t.Fatal("locate u1")
	}
	path := filepath.Join(dataDir, "calendars", "tasks", loc.Name)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}

	a.bulkComplete()

	loc2, _ := a.store.Locate(u2)
	if td := findTodo(loc2.Object, u2); td == nil || td.Completed() {
		t.Fatal("write failure abort must roll back the already-completed sibling")
	}
	if len(a.undo) != undoBefore {
		t.Fatal("a rolled-back bulk op must not push an undo step")
	}
}

// TestBulkDeleteDedupeAndUndo: parent+child both selected → the child is
// absorbed into the parent's subtree (deleted once); the confirm names the
// full resource count; one undo step restores everything.
func TestBulkDeleteDedupeAndUndo(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	parent := putTodo(t, a, testCalID(a), "", "folder", now, true)
	child := putTodo(t, a, testCalID(a), parent, "leaf", now, true)
	solo := putTodo(t, a, testCalID(a), "", "solo", now, true)
	a.refresh(parent)
	a.setFocus(a.tree)
	undoBefore := len(a.undo)
	selectTreeRangeAll(t, a) // parent, leaf, solo all in range
	a.bulkDelete()
	confirmYes(t, a) // accept the confirm dialog

	for _, u := range []string{parent, child, solo} {
		if _, ok := a.store.Locate(u); ok {
			t.Fatalf("%s must be deleted", u)
		}
	}
	if len(a.undo) != undoBefore+1 {
		t.Fatalf("bulk delete must be one undo step, got %d", len(a.undo)-undoBefore)
	}
	a.undoLast()
	for _, u := range []string{parent, child, solo} {
		if _, ok := a.store.Locate(u); !ok {
			t.Fatalf("undo must restore %s", u)
		}
	}
}

// TestBulkDeleteSkipsRecurringEvent: a recurring event in a drilled-day range
// is skipped with a count; the non-recurring siblings delete.
func TestBulkDeleteSkipsRecurringEvent(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	putEvent(t, a, testCalID(a), "oneoff", now, false)
	rec := putRecurringEvent(t, a, testCalID(a), "weekly", now, "FREQ=WEEKLY")
	a.refresh("")
	a.month.reDrill(model.DayStart(now), 0)
	a.enterSelect()
	a.month.eventIndex = len(a.dayItems(model.DayStart(now))) - 1
	a.bulkDelete()
	confirmYes(t, a)

	if _, ok := a.store.Locate(rec); !ok {
		t.Fatal("recurring event must be skipped, not deleted")
	}
	if s := a.statusLeft.GetText(true); !strings.Contains(s, "recurring") {
		t.Fatalf("summary must count the recurring skip, got %q", s)
	}
}

// TestBulkDeleteReadOnlySkipped: an item on a read-only calendar is filtered
// out before the confirm and counted in the summary — mirrors the read-only
// guard in bulkComplete/deleteSelected (guardWrite), applied without aborting
// the whole batch. Uses calendar mode (not tasks) because the task tree shows
// one task list at a time, so a single tree range can never mix a read-only
// and a writable calendar's items the way a drilled day's event list can.
func TestBulkDeleteReadOnlySkipped(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	roCal := "ro-events"
	if err := a.store.CreateCalendarLocal(context.Background(), roCal, store.CalendarMeta{DisplayName: "RO"}, []string{"VEVENT"}); err != nil {
		t.Fatal(err)
	}
	a.reload()
	putEvent(t, a, roCal, "locked", now, false)
	putEvent(t, a, testCalID(a), "open", now, false)
	if err := a.store.SetCalendarReadOnly(context.Background(), roCal, true); err != nil {
		t.Fatal(err)
	}
	a.refresh("")
	a.month.reDrill(model.DayStart(now), 0)
	a.enterSelect()
	items := a.dayItems(model.DayStart(now))
	if len(items) < 2 {
		t.Fatalf("need 2 events on the day, got %d", len(items))
	}
	a.month.eventIndex = len(items) - 1 // extend over the whole day
	a.bulkDelete()
	confirmYes(t, a)

	remaining := a.dayItems(model.DayStart(now))
	if len(remaining) != 1 || remaining[0].Title != "locked" {
		t.Fatalf("read-only event must survive and the writable one must be deleted, got %v", remaining)
	}
	if s := a.statusLeft.GetText(true); !strings.Contains(s, "read-only") {
		t.Fatalf("summary must count the read-only skip, got %q", s)
	}
}
