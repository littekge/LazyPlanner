package ui

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
)

// testCalID returns the calendar id to seed test fixtures into. The fixture's
// "personal" calendar (selected by default in both overview lists) accepts
// both VEVENT and VTODO, so one id works for putTodo and putEvent alike.
func testCalID(a *app) string { return a.selectedCalendarID() }

// key feeds one key event through the app's global input capture, mimicking the
// event loop's dispatch (capture first, then the focused primitive).
func key(a *app, k tcell.Key, r rune) {
	ev := tcell.NewEventKey(k, r, tcell.ModNone)
	if out := a.globalKeys(ev); out != nil {
		if p := a.tv.GetFocus(); p != nil {
			if h := p.InputHandler(); h != nil {
				h(out, func(x tview.Primitive) { a.setFocus(x) })
			}
		}
	}
}

// TestSelectEnterPerContext covers V's entry gate: tree and calendar (drilled
// and un-drilled) accept with the right anchor; the agenda pane refuses.
func TestSelectEnterPerContext(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)

	// Tree context: anchor = current task UID.
	a.setMode(modeTasks)
	uid := putTodo(t, a, testCalID(a), "", "task A", now, true)
	a.refresh(uid)
	a.setFocus(a.tree)
	a.enterSelect()
	if !a.selecting || a.selAnchorUID != uid {
		t.Fatalf("tree enterSelect: selecting=%v anchor=%q, want true/%q", a.selecting, a.selAnchorUID, uid)
	}
	a.exitSelect()
	if a.selecting || a.selAnchorUID != "" || !a.selAnchorDay.IsZero() {
		t.Fatal("exitSelect must clear all anchor state")
	}

	// Un-drilled calendar: anchor = the selected day.
	a.setMode(modeCalendar)
	a.enterSelect()
	if !a.selecting || a.selAnchorDay.IsZero() {
		t.Fatalf("calendar enterSelect: selecting=%v dayAnchor=%v", a.selecting, a.selAnchorDay)
	}
	if got := a.selContext(); got != selDays {
		t.Fatalf("selContext = %d, want selDays", got)
	}
	a.exitSelect()

	// Agenda: refused.
	a.setMode(modeAgenda)
	a.enterSelect()
	if a.selecting {
		t.Fatal("agenda must refuse SELECT")
	}
}

// TestSelectBadge: SELECT surfaces in interactionMode, and an inner GRAB wins.
func TestSelectBadge(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	a.setMode(modeCalendar)
	a.enterSelect()
	if m := a.interactionMode(); m != modeSelect {
		t.Fatalf("selecting badge = %q, want SELECT", m)
	}
	a.grabbing = true
	if m := a.interactionMode(); m != modeGrab {
		t.Fatalf("nested grab badge = %q, want GRAB", m)
	}
	a.grabbing = false
	a.exitSelect()
	if m := a.interactionMode(); m != modeNormal {
		t.Fatalf("after exit badge = %q, want NORMAL", m)
	}
}

// TestSelectKeyLayer: motion passes through (the cursor move IS the range
// extension), mutating/context keys are inert, Esc and V exit exactly one level.
func TestSelectKeyLayer(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	u1 := putTodo(t, a, testCalID(a), "", "task A", now, true)
	putTodo(t, a, testCalID(a), "", "task B", now.AddDate(0, 0, 1), true)
	a.refresh(u1)
	a.setFocus(a.tree)
	a.enterSelect()

	// Motion passes through: j moves the tree cursor.
	before := a.currentTreeUID()
	key(a, tcell.KeyRune, 'j')
	if a.currentTreeUID() == before {
		t.Fatal("j must still move the cursor while selecting")
	}

	// Context/data keys are inert: t (pane), . (show-completed), e (edit form).
	key(a, tcell.KeyRune, 't')
	if a.mode != modeTasks {
		t.Fatal("pane switch must be inert while selecting")
	}
	sc := a.showCompleted
	key(a, tcell.KeyRune, '.')
	if a.showCompleted != sc {
		t.Fatal(". must be inert while selecting")
	}
	key(a, tcell.KeyRune, 'e')
	if a.modalOpen() {
		t.Fatal("e must not open the edit form while selecting")
	}

	// Esc exits SELECT only (still in Tasks, cursor kept).
	key(a, tcell.KeyEscape, 0)
	if a.selecting {
		t.Fatal("Esc must exit SELECT")
	}

	// V toggles: enter then V exits.
	a.enterSelect()
	key(a, tcell.KeyRune, 'V')
	if a.selecting {
		t.Fatal("V while selecting must exit SELECT")
	}
}

// TestSelectSwallowsModifiedArrows: a modified arrow (Ctrl-Left/Right resizes
// the left column outside SELECT) is not motion — it must be swallowed like
// every other non-motion key, not leaked through to the pre-existing Ctrl-gated
// resize handlers in globalKeys.
func TestSelectSwallowsModifiedArrows(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	a.setFocus(a.tree)
	a.enterSelect()

	before := a.leftWidth
	a.globalKeys(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModCtrl))
	if a.leftWidth != before {
		t.Fatalf("Ctrl-Left while selecting must not resize: leftWidth %d -> %d", before, a.leftWidth)
	}
	a.globalKeys(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModCtrl))
	if a.leftWidth != before {
		t.Fatalf("Ctrl-Right while selecting must not resize: leftWidth %d -> %d", before, a.leftWidth)
	}
	if !a.selecting {
		t.Fatal("SELECT must still be active after the swallowed modified arrows")
	}
}

// TestSelectPrefixGate: while selecting, gg still jumps (pure motion) but gt/gd
// (context jumps / modals) are blocked — gotoToday switches to Calendar mode,
// which would yank the context out from under the range.
func TestSelectPrefixGate(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	u1 := putTodo(t, a, testCalID(a), "", "task A", now, true)
	putTodo(t, a, testCalID(a), "", "task B", now.AddDate(0, 0, 1), true)
	a.refresh(u1)
	a.setFocus(a.tree)
	key(a, tcell.KeyRune, 'j') // move off the top
	a.enterSelect()

	key(a, tcell.KeyRune, 'g')
	key(a, tcell.KeyRune, 't') // gt = gotoToday: must be blocked
	if a.mode != modeTasks || !a.selecting {
		t.Fatal("gt must be blocked while selecting")
	}

	key(a, tcell.KeyRune, 'g')
	key(a, tcell.KeyRune, 'g') // gg = top: pure motion, allowed
	nodes := visibleTreeNodes(a.tree.GetRoot())
	if len(nodes) == 0 || a.tree.GetCurrentNode() != nodes[0] {
		t.Fatal("gg must still jump to the top while selecting")
	}
}

// TestSelectEscKeepsDrill: SELECT entered from a drilled day unwinds to DRILL,
// not to day navigation — Esc backs out exactly one mode level.
func TestSelectEscKeepsDrill(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	putEvent(t, a, testCalID(a), "meeting", now, false)
	a.setMode(modeCalendar)
	a.refresh("")
	a.month.reDrill(model.DayStart(now), 0)
	if !a.gridDrilled() {
		t.Fatal("precondition: day must be drilled")
	}
	a.enterSelect()
	if a.selContext() != selDrill || a.selAnchorUID == "" {
		t.Fatalf("drilled enterSelect: ctx=%d anchor=%q", a.selContext(), a.selAnchorUID)
	}
	key(a, tcell.KeyEscape, 0)
	if a.selecting {
		t.Fatal("Esc must exit SELECT")
	}
	if !a.gridDrilled() {
		t.Fatal("Esc from SELECT must land back in DRILL, not day navigation")
	}
}

// TestTreeRange: anchor→cursor over visible rows, inclusive, either direction;
// nil when the anchor vanished (e.g. a background sync deleted it).
func TestTreeRange(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	var uids []string
	for _, s := range []string{"A", "B", "C", "D", "E"} {
		uids = append(uids, putTodo(t, a, testCalID(a), "", "task "+s, now, true))
	}
	a.refresh(uids[0])
	a.setFocus(a.tree)

	// Visible order is the smart sort (same due date → title order A..E).
	a.selectTreeByUID(uids[1]) // anchor at B
	a.enterSelect()
	a.selectTreeByUID(uids[3]) // cursor at D
	got := a.selRange()
	if len(got) != 3 || got[0].uid != uids[1] || got[2].uid != uids[3] {
		t.Fatalf("forward range = %+v, want B..D", got)
	}

	// Reversed: cursor above the anchor selects the same rows.
	a.selectTreeByUID(uids[0])
	got = a.selRange()
	if len(got) != 2 || got[0].uid != uids[0] || got[1].uid != uids[1] {
		t.Fatalf("reversed range = %+v, want A..B", got)
	}

	// Single-item range: cursor on the anchor.
	a.selectTreeByUID(uids[1])
	if got = a.selRange(); len(got) != 1 || got[0].uid != uids[1] {
		t.Fatalf("single range = %+v, want just B", got)
	}
	a.exitSelect()
}

// TestTreeRangeAnchorVanished: after the anchor's task is gone, selRange is nil
// and syncSelectionVisuals (run by refresh) exits SELECT with a flash.
func TestTreeRangeAnchorVanished(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	u1 := putTodo(t, a, testCalID(a), "", "task A", now, true)
	u2 := putTodo(t, a, testCalID(a), "", "task B", now, true)
	a.refresh(u1)
	a.setFocus(a.tree)
	a.selectTreeByUID(u1)
	a.enterSelect()

	// Simulate a remote deletion landing via sync: delete + refresh.
	loc, _ := a.store.Locate(u1)
	if err := a.store.Delete(context.Background(), loc.CalID, loc.Name); err != nil {
		t.Fatal(err)
	}
	a.refresh(u2)
	if a.selecting {
		t.Fatal("SELECT must exit when the anchor vanishes")
	}
}

// TestDaysRange: the date interval materializes every visible item once — a
// multi-day event spanning several selected days is deduped to one target.
func TestDaysRange(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC) // Monday
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	putEvent(t, a, testCalID(a), "day1", now, false)
	putEvent(t, a, testCalID(a), "day3", now.AddDate(0, 0, 2), false)
	// A two-day timed event covering day1→day2 (spans midnight).
	putSpanningEvent(t, a, testCalID(a), "span", now, now.AddDate(0, 0, 1).Add(2*time.Hour))
	a.refresh("")

	a.month.selected = model.DayStart(now)
	a.enterSelect()
	a.month.selected = model.DayStart(now.AddDate(0, 0, 2)) // extend to day3
	got := a.selRange()
	count := map[string]int{}
	for _, tg := range got {
		count[tg.uid]++
	}
	if len(got) != 3 {
		t.Fatalf("range = %d targets, want 3 (day1, span, day3)", len(got))
	}
	for uid, n := range count {
		if n != 1 {
			t.Fatalf("uid %s appears %d times, want 1 (multi-day dedupe)", uid, n)
		}
	}
	a.exitSelect()
}

// TestDaysRangeReversed: cursor day before the anchor day selects the same
// interval as forward (mirrors TestTreeRange's reversed case).
func TestDaysRangeReversed(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC) // Monday
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	putEvent(t, a, testCalID(a), "day1", now, false)
	putEvent(t, a, testCalID(a), "day3", now.AddDate(0, 0, 2), false)
	// A two-day timed event covering day1→day2 (spans midnight).
	putSpanningEvent(t, a, testCalID(a), "span", now, now.AddDate(0, 0, 1).Add(2*time.Hour))
	a.refresh("")

	a.month.selected = model.DayStart(now.AddDate(0, 0, 2)) // anchor on day3
	a.enterSelect()
	a.month.selected = model.DayStart(now) // cursor back to day1
	got := a.selRange()
	if len(got) != 3 {
		t.Fatalf("reversed days range = %d targets, want 3 (day1, span, day3)", len(got))
	}
	a.exitSelect()
}

// TestDaysRangeSingleDay: cursor on the anchor day selects just that day's items.
func TestDaysRangeSingleDay(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC) // Monday
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	putEvent(t, a, testCalID(a), "day1", now, false)
	putEvent(t, a, testCalID(a), "day2", now.AddDate(0, 0, 1), false)
	a.refresh("")

	a.month.selected = model.DayStart(now)
	a.enterSelect()
	// Cursor stays on the anchor day: selRange must not reach into day2.
	got := a.selRange()
	if len(got) != 1 {
		t.Fatalf("single-day range = %d targets, want 1 (just day1)", len(got))
	}
	a.exitSelect()
}

// TestDaysRangeCapsAtMaxSelectDays: a cursor day far beyond maxSelectDays must
// not hang derivation or materialize past the cap — an item beyond it is
// excluded even though it's within the anchor→cursor interval as requested.
func TestDaysRangeCapsAtMaxSelectDays(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC) // Monday
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	putSpanningEvent(t, a, testCalID(a), "early", now.Add(2*time.Hour), now.Add(3*time.Hour))
	putSpanningEvent(t, a, testCalID(a), "toolate", now.AddDate(0, 0, 380).Add(2*time.Hour), now.AddDate(0, 0, 380).Add(3*time.Hour))
	a.refresh("")

	anchorDay := model.DayStart(now)
	a.month.selected = anchorDay
	a.enterSelect()
	a.month.selected = model.DayStart(now.AddDate(0, 0, 400)) // far beyond the cap
	got := a.selRange()

	cutoff := anchorDay.AddDate(0, 0, maxSelectDays+1) // exclusive: one day past the capped interval
	foundEarly := false
	for _, tg := range got {
		if tg.uid == "toolate@ev" {
			t.Fatalf("range included %q, want it excluded by the %d-day cap", tg.uid, maxSelectDays)
		}
		if tg.uid == "early@ev" {
			foundEarly = true
		}
		if !tg.occStart.Before(cutoff) {
			t.Fatalf("target %+v occurs at/after the cap cutoff %v", tg, cutoff)
		}
	}
	if !foundEarly {
		t.Fatal("range missing the in-range 'early' event")
	}
	a.exitSelect()
}

// TestDaysRangeEmptyDayStaysSelected: a date anchor is always resolvable
// (unlike a tree UID or drilled item, it can't "vanish"), so entering SELECT on
// a day with no items must stay selected — an empty materialized range is a
// valid empty selection, not a lost anchor. The range should be extendable
// toward days that do have items.
func TestDaysRangeEmptyDayStaysSelected(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC) // Monday, no events seeded
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	a.refresh("")

	a.month.selected = model.DayStart(now)
	a.enterSelect()
	if !a.selecting {
		t.Fatal("SELECT must stay active when the selected day has no items")
	}
	got := a.selRange()
	if got == nil {
		t.Fatal("selRange on an empty-but-anchored day must be a non-nil empty slice, not nil (nil means lost anchor)")
	}
	if len(got) != 0 {
		t.Fatalf("range = %+v, want empty", got)
	}
	a.exitSelect()
}

// TestSelectRangeSyncRace: derive the range continuously while a background
// goroutine mutates the store (the sync scenario) — run under -race. The store
// is internally locked; this asserts derivation never panics or returns
// targets for items that are mid-deletion, in the TestConcurrentSyncAndEditsRace
// mold (internal/sync/sync_test.go:372).
func TestSelectRangeSyncRace(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	anchor := putTodo(t, a, testCalID(a), "", "anchor", now, true)
	for i := 0; i < 20; i++ {
		putTodo(t, a, testCalID(a), "", fmt.Sprintf("task %02d", i), now, true)
	}
	a.refresh(anchor)
	a.setFocus(a.tree)
	a.selectTreeByUID(anchor)
	a.enterSelect()

	// Snapshot one resource to churn: the goroutine may not touch t (t.Fatal is
	// test-goroutine-only), so it uses raw store calls exclusively.
	churn := putTodo(t, a, testCalID(a), "", "churn", now, true)
	churnLoc, ok := a.store.Locate(churn)
	if !ok {
		t.Fatal("churn task missing")
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { // the "sync" goroutine: delete/recreate a resource repeatedly
		defer wg.Done()
		ctx := context.Background()
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = a.store.Delete(ctx, churnLoc.CalID, churnLoc.Name)
			_, _ = a.store.Put(ctx, churnLoc.CalID, churnLoc.Name, churnLoc.Object)
		}
	}()
	for i := 0; i < 500; i++ {
		_ = a.selRange() // must never panic; content raced deliberately
	}
	close(stop)
	wg.Wait()
	a.exitSelect()
}

// TestDrillRange: within a drilled day, the range covers the item list between
// anchor and cursor in drill-cycling order.
func TestDrillRange(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	putEvent(t, a, testCalID(a), "e1", now, false)
	putEvent(t, a, testCalID(a), "e2", now.Add(time.Hour), false)
	putEvent(t, a, testCalID(a), "e3", now.Add(2*time.Hour), false)
	a.refresh("")
	a.month.reDrill(model.DayStart(now), 0)
	a.enterSelect()
	a.month.eventIndex = 2 // cursor on the third item
	got := a.selRange()
	if len(got) != 3 {
		t.Fatalf("drill range = %d targets, want 3", len(got))
	}
}

// TestDrillRangeReversed: cursor index before the anchor index selects the
// same items as forward (mirrors TestTreeRange's reversed case).
func TestDrillRangeReversed(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	putEvent(t, a, testCalID(a), "e1", now, false)
	putEvent(t, a, testCalID(a), "e2", now.Add(time.Hour), false)
	putEvent(t, a, testCalID(a), "e3", now.Add(2*time.Hour), false)
	a.refresh("")
	a.month.reDrill(model.DayStart(now), 2) // anchor at the third item
	a.enterSelect()
	a.month.eventIndex = 0 // cursor back at the first item
	got := a.selRange()
	if len(got) != 3 {
		t.Fatalf("reversed drill range = %d targets, want 3", len(got))
	}
	a.exitSelect()
}

// TestDrillRangeSingle: cursor on the anchor index selects just that one item.
func TestDrillRangeSingle(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	putEvent(t, a, testCalID(a), "e1", now, false)
	putEvent(t, a, testCalID(a), "e2", now.Add(time.Hour), false)
	putEvent(t, a, testCalID(a), "e3", now.Add(2*time.Hour), false)
	a.refresh("")
	a.month.reDrill(model.DayStart(now), 1)
	a.enterSelect()
	// Cursor stays on the anchor index.
	got := a.selRange()
	if len(got) != 1 {
		t.Fatalf("single drill range = %d targets, want 1", len(got))
	}
	a.exitSelect()
}
