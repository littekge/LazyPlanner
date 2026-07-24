package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
