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
	a.setFocus(a.calendarPrimitive())
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
	a.setFocus(a.calendarPrimitive())
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

// putTodoWithParent writes a VTODO with an explicit UID and RELATED-TO parent
// (default RELTYPE=PARENT per RFC 5545). Needed over putTodo for fixtures that
// require a specific pre-known UID — e.g. a reciprocal parent cycle, where both
// ends must reference a UID that doesn't exist yet when the other is created.
func putTodoWithParent(t *testing.T, a *app, calID, uid, summary, parentUID string, due time.Time) string {
	t.Helper()
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\nBEGIN:VTODO\r\nUID:" + uid +
		"\r\nSUMMARY:" + summary + "\r\nDTSTAMP:20260701T000000Z\r\nDUE:" + due.UTC().Format("20060102T150405Z")
	if parentUID != "" {
		ics += "\r\nRELATED-TO:" + parentUID
	}
	ics += "\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"
	parsed, err := model.Decode([]byte(ics), time.Local)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.Put(context.Background(), calID, store.ResourceName(uid), parsed); err != nil {
		t.Fatal(err)
	}
	return uid
}

// TestBulkDeleteRootsSurvivesParentCycle: a reciprocal RELATED-TO cycle
// (malformed/foreign .ics data — untrusted, never assume acyclic) must not
// hang the ancestor walk. cycle-c's parent chain (cycle-c → cycle-a → cycle-b
// → cycle-a → ...) enters the cycle without ever reaching a selected ancestor,
// so cycle-c must survive as its own root — reached inside a timeout because an
// unguarded walk here freezes the single-threaded UI event loop forever, not
// just this goroutine.
func TestBulkDeleteRootsSurvivesParentCycle(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	calID := testCalID(a)
	putTodoWithParent(t, a, calID, "cycle-a", "A", "cycle-b", now)
	putTodoWithParent(t, a, calID, "cycle-b", "B", "cycle-a", now)
	putTodoWithParent(t, a, calID, "cycle-c", "C", "cycle-a", now)

	targets := []editTarget{{isTodo: true, uid: "cycle-c"}}

	type result struct {
		roots []editTarget
		skips bulkSkip
	}
	done := make(chan result, 1)
	go func() {
		roots, skips := a.bulkDeleteRoots(targets)
		done <- result{roots, skips}
	}()
	select {
	case r := <-done:
		if len(r.roots) != 1 || r.roots[0].uid != "cycle-c" {
			t.Fatalf("expected cycle-c to survive as its own root, got %v (skips=%v)", r.roots, r.skips)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("bulkDeleteRoots hung walking a RELATED-TO parent cycle")
	}
}

// TestBulkDeleteRootsAbsorbsOnlyIntoSurvivingAncestor: a child whose only
// selected ancestor is itself filtered out (here: read-only) must not be
// silently absorbed — it has to survive as its own root, since the ancestor it
// would have traveled with is never actually going to be deleted.
func TestBulkDeleteRootsAbsorbsOnlyIntoSurvivingAncestor(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	roCal := "ro-tasks"
	if err := a.store.CreateCalendarLocal(context.Background(), roCal, store.CalendarMeta{DisplayName: "RO"}, []string{"VTODO"}); err != nil {
		t.Fatal(err)
	}
	a.reload()
	putTodoWithParent(t, a, roCal, "ro-parent", "folder", "", now)
	putTodoWithParent(t, a, testCalID(a), "child-of-ro", "leaf", "ro-parent", now)
	if err := a.store.SetCalendarReadOnly(context.Background(), roCal, true); err != nil {
		t.Fatal(err)
	}

	targets := []editTarget{
		{isTodo: true, uid: "ro-parent"},
		{isTodo: true, uid: "child-of-ro"},
	}
	roots, skips := a.bulkDeleteRoots(targets)

	foundChild := false
	for _, r := range roots {
		if r.uid == "ro-parent" {
			t.Fatal("read-only parent must not survive into roots")
		}
		if r.uid == "child-of-ro" {
			foundChild = true
		}
	}
	if !foundChild {
		t.Fatalf("child whose only selected ancestor was filtered out must survive as its own root, got %v", roots)
	}
	if skips["read-only"] != 1 {
		t.Fatalf("expected exactly 1 read-only skip, got %v", skips)
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
	a.setFocus(a.calendarPrimitive())
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

// TestBulkYankPasteUnder: cut a range of siblings, paste under another task —
// all roots move (order preserved), one undo step, clipboard persists.
func TestBulkYankPasteUnder(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	target := putTodo(t, a, testCalID(a), "", "a target", now, true)
	u1 := putTodo(t, a, testCalID(a), "", "b move1", now, true)
	u2 := putTodo(t, a, testCalID(a), "", "c move2", now, true)
	a.refresh(target)
	a.setFocus(a.tree)

	a.selectTreeByUID(u1)
	a.enterSelect()
	a.selectTreeByUID(u2)
	a.bulkYank(true) // cut
	if a.selecting {
		t.Fatal("yank must exit SELECT")
	}
	if len(a.yankUIDs) != 2 {
		t.Fatalf("clipboard = %v, want two roots", a.yankUIDs)
	}

	a.selectTreeByUID(target)
	undoBefore := len(a.undo)
	a.pasteUnderSelection()
	for _, u := range []string{u1, u2} {
		loc, _ := a.store.Locate(u)
		if td := findTodo(loc.Object, u); td == nil || td.ParentUID != target {
			t.Fatalf("%s must be re-parented under the target", u)
		}
	}
	if len(a.undo) != undoBefore+1 {
		t.Fatal("multi-paste must be one undo step")
	}
	if len(a.yankUIDs) != 2 {
		t.Fatal("clipboard must persist after paste")
	}
}

// TestBulkYankCopyPasteMultiRoot: Y two roots (one carrying a child), paste
// into the same list — both roots copy with fresh UIDs, the child's parent
// remaps to its own copied root (not the original), the originals are
// untouched, and the whole paste is one undo step.
func TestBulkYankCopyPasteMultiRoot(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	root1 := putTodo(t, a, testCalID(a), "", "a root1", now, true)
	child := putTodo(t, a, testCalID(a), root1, "a root1 child", now, true)
	root2 := putTodo(t, a, testCalID(a), "", "b root2", now, true)
	target := putTodo(t, a, testCalID(a), "", "c target", now, true)
	a.refresh(root1)
	a.setFocus(a.tree)

	// The visible range root1→root2 includes child implicitly (it's nested
	// under root1) — bulkYank's dedupe folds it into root1's subtree rather
	// than yanking it as a third root.
	a.selectTreeByUID(root1)
	a.enterSelect()
	a.selectTreeByUID(root2)
	a.bulkYank(false) // copy
	if len(a.yankUIDs) != 2 {
		t.Fatalf("clipboard = %v, want two roots (root1, root2)", a.yankUIDs)
	}

	a.selectTreeByUID(target)
	undoBefore := len(a.undo)
	a.pasteUnderSelection()

	// Originals untouched: same UID, same parent, still present.
	for u, wantParent := range map[string]string{root1: "", child: root1, root2: ""} {
		loc, ok := a.store.Locate(u)
		if !ok {
			t.Fatalf("original %s must survive a copy", u)
		}
		if td := findTodo(loc.Object, u); td == nil || td.ParentUID != wantParent {
			t.Fatalf("original %s must keep parent %q", u, wantParent)
		}
	}

	// Two fresh copies land under target: root1's copy and root2's copy.
	var root1CopyUID string
	for _, td := range todosBySummary(a, "a root1") {
		if td.UID == root1 {
			continue
		}
		if _, ok := a.store.Locate(td.UID); !ok || td.ParentUID != target {
			t.Fatalf("root1 copy %s must be parented under target, got parent %q", td.UID, td.ParentUID)
		}
		root1CopyUID = td.UID
	}
	if root1CopyUID == "" {
		t.Fatal("no fresh copy of root1 found")
	}
	root2CopyFound := false
	for _, td := range todosBySummary(a, "b root2") {
		if td.UID == root2 {
			continue
		}
		if td.ParentUID != target {
			t.Fatalf("root2 copy %s must be parented under target, got parent %q", td.UID, td.ParentUID)
		}
		root2CopyFound = true
	}
	if !root2CopyFound {
		t.Fatal("no fresh copy of root2 found")
	}

	// The child's copy must be re-parented onto root1's copy, not the original.
	childCopyFound := false
	for _, td := range todosBySummary(a, "a root1 child") {
		if td.UID == child {
			continue
		}
		if td.ParentUID != root1CopyUID {
			t.Fatalf("child copy %s parent = %q, want the copied root1 %q", td.UID, td.ParentUID, root1CopyUID)
		}
		childCopyFound = true
	}
	if !childCopyFound {
		t.Fatal("no fresh copy of the child found")
	}

	if len(a.undo) != undoBefore+1 {
		t.Fatalf("multi-root copy-paste must be one undo step, got %d", len(a.undo)-undoBefore)
	}
	a.undoLast()
	for _, sum := range []string{"a root1", "a root1 child", "b root2"} {
		if got := len(todosBySummary(a, sum)); got != 1 {
			t.Fatalf("after undo, %q must appear exactly once (the original), got %d", sum, got)
		}
	}
}

// TestBulkYankCrossListMoveMultiRoot: y two roots (one carrying a child),
// paste into a different writable list — both subtrees recreate in the
// target and delete from the source, as one undo step that restores
// everything in the source and removes the target copies.
func TestBulkYankCrossListMoveMultiRoot(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	srcCal := testCalID(a)
	root1 := putTodo(t, a, srcCal, "", "a root1", now, true)
	child := putTodo(t, a, srcCal, root1, "a root1 child", now, true)
	root2 := putTodo(t, a, srcCal, "", "b root2", now, true)
	a.refresh(root1)
	a.setFocus(a.tree)

	a.selectTreeByUID(root1)
	a.enterSelect()
	a.selectTreeByUID(root2)
	a.bulkYank(true) // cut
	if len(a.yankUIDs) != 2 {
		t.Fatalf("clipboard = %v, want two roots (root1, root2)", a.yankUIDs)
	}

	// "work" becomes a second writable task list once it holds a VTODO (the
	// TestStickyWorksOnNonFirstList idiom) — switch to it via SetCurrentItem,
	// whose changed-callback rebuilds the tree for the new list.
	a.createTask("work", "", "seed")
	workIdx := -1
	for i, id := range a.tasklistIDs {
		if id == "work" {
			workIdx = i
		}
	}
	if workIdx < 0 {
		t.Fatalf("work list not registered as a task list: %v", a.tasklistIDs)
	}
	undoBefore := len(a.undo) // after the seed task's own undo entry
	a.tasklists.SetCurrentItem(workIdx)

	a.pasteAtTop()

	// A move (unlike a copy) keeps each item's UID — moveSubtreeOps recreates
	// the same UID at the destination and removes it from the source, so
	// "recreated in the target" means Locate(uid) now resolves to "work", and
	// "deleted from the source" means it no longer resolves to srcCal.
	for u, wantParent := range map[string]string{root1: "", child: root1, root2: ""} {
		loc, ok := a.store.Locate(u)
		if !ok {
			t.Fatalf("%s must still exist (relocated, not deleted outright)", u)
		}
		if loc.CalID != "work" {
			t.Fatalf("%s calID = %q, want %q (moved to the target list)", u, loc.CalID, "work")
		}
		if td := findTodo(loc.Object, u); td == nil || td.ParentUID != wantParent {
			t.Fatalf("%s parent = %v, want %q", u, td, wantParent)
		}
	}
	if len(a.undo) != undoBefore+1 {
		t.Fatalf("cross-list multi-root move must be one undo step, got %d", len(a.undo)-undoBefore)
	}

	a.undoLast()
	for u, wantParent := range map[string]string{root1: "", child: root1, root2: ""} {
		loc, ok := a.store.Locate(u)
		if !ok {
			t.Fatalf("undo must restore %s", u)
		}
		if loc.CalID != srcCal {
			t.Fatalf("undo: %s calID = %q, want back in the source %q", u, loc.CalID, srcCal)
		}
		if td := findTodo(loc.Object, u); td == nil || td.ParentUID != wantParent {
			t.Fatalf("undo: %s parent = %v, want %q", u, td, wantParent)
		}
	}
}

// TestBulkYankDedupesSubtree: yanking a parent and its child cuts one root —
// the child travels inside the parent's subtree, not as a second root.
func TestBulkYankDedupesSubtree(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	parent := putTodo(t, a, testCalID(a), "", "folder", now, true)
	leaf := putTodo(t, a, testCalID(a), parent, "leaf", now, true)
	a.refresh(parent)
	a.setFocus(a.tree)
	// Range parent→leaf only (not selectTreeRangeAll): the shared fixture seeds
	// its own top-level "grocery" task, and a whole-tree range would pick that
	// up as an unrelated third row — this test is about subtree dedup, not the
	// whole tree, so the range is bounded to exactly the folder's own subtree.
	a.selectTreeByUID(parent)
	a.enterSelect()
	a.selectTreeByUID(leaf)
	a.bulkYank(false) // copy
	if len(a.yankUIDs) != 1 || a.yankUIDs[0] != parent {
		t.Fatalf("clipboard roots = %v, want just the parent", a.yankUIDs)
	}
}

// TestBulkYankTreeOnly: y in a calendar-days range flashes and keeps SELECT.
func TestBulkYankTreeOnly(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	a.setFocus(a.calendarPrimitive())
	a.refresh("")
	a.enterSelect()
	a.bulkYank(true)
	if !a.selecting || len(a.yankUIDs) != 0 {
		t.Fatal("yank outside the tree must flash and keep SELECT with an empty clipboard")
	}
}

// TestBulkPasteCycleGuard: pasting a cut range onto one of its own roots (or
// into a root's subtree) is refused before any write.
func TestBulkPasteCycleGuard(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	u1 := putTodo(t, a, testCalID(a), "", "a root1", now, true)
	u2 := putTodo(t, a, testCalID(a), "", "b root2", now, true)
	a.refresh(u1)
	a.setFocus(a.tree)
	a.selectTreeByUID(u1)
	a.enterSelect()
	a.selectTreeByUID(u2)
	a.bulkYank(true)

	a.selectTreeByUID(u2) // paste target = one of the cut roots
	a.pasteUnderSelection()
	loc, _ := a.store.Locate(u1)
	if td := findTodo(loc.Object, u1); td == nil || td.ParentUID != "" {
		t.Fatal("cycle-guarded paste must not move anything")
	}
}
