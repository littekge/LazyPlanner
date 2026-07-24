package ui

import (
	"context"
	"fmt"
	"sort"

	"github.com/littekge/LazyPlanner/internal/model"
)

// Bulk operations run over the SELECT range with one shared shape:
// materialize → filter (counting skips) → execute with rollback → summarize.
// Execution follows the moveSubtree template: every write is version-checked,
// any failure or stale result rolls back the writes already made (newest
// first), and full success pushes ONE compound undo step.

// bulkSkip counts filtered-out items per reason for the summary flash.
type bulkSkip map[string]int

func (s bulkSkip) add(reason string) { s[reason]++ }

// summary renders the counts deterministically (sorted by reason).
func (s bulkSkip) summary() string {
	if len(s) == 0 {
		return ""
	}
	reasons := make([]string, 0, len(s))
	for r := range s {
		reasons = append(reasons, r)
	}
	sort.Strings(reasons)
	out := ""
	for i, r := range reasons {
		if i > 0 {
			out += " · "
		}
		out += fmt.Sprintf("%d %s", s[r], r)
	}
	return out
}

func bulkSummary(verb string, n int, skips bulkSkip) string {
	s := fmt.Sprintf("%d %s", n, verb)
	if sk := skips.summary(); sk != "" {
		s += " · skipped: " + sk
	}
	return s
}

// calReadOnly is guardWrite's test without the flash, for bulk filters that
// count read-only items instead of aborting on the first one.
func (a *app) calReadOnly(calID string) bool {
	cal, ok := a.store.Calendar(calID)
	return ok && cal.ReadOnly
}

// bulkComplete (Space in SELECT) completes every incomplete task in the range:
// plain tasks are marked done, recurring todos advance one occurrence — the
// single-item Space semantics applied per item. Events, folders with
// incomplete children, already-done tasks, and read-only items are skipped and
// counted. Processing runs in REVERSE visible order (children before parents),
// so completing a folder together with its last incomplete child works in one
// pass: the child's write lands in the store before the folder's
// hasIncompleteChildren check runs, so the freshly-completed child no longer
// blocks it. All-or-nothing: a failed or stale write rolls back this op's
// writes; each item's Locate happens immediately before its write (never
// cached from materialization), so a write is always checked against the
// current store state right before it happens.
func (a *app) bulkComplete() {
	targets := a.selRange()
	if targets == nil {
		a.exitSelect()
		a.flash("Selection no longer valid")
		return
	}
	ctx := context.Background()
	skips := bulkSkip{}
	var ops []undoOp
	var rollback []func()
	fail := func(msg string) {
		for i := len(rollback) - 1; i >= 0; i-- {
			rollback[i]()
		}
		a.exitSelect()
		a.refreshKeepingDrill("")
		a.flash(msg)
	}
	done := 0
	var sticky []string
	for i := len(targets) - 1; i >= 0; i-- {
		t := targets[i]
		if !t.isTodo {
			skips.add("event(s)")
			continue
		}
		loc, ok := a.store.Locate(t.uid)
		if !ok {
			skips.add("missing")
			continue
		}
		if a.calReadOnly(loc.CalID) {
			skips.add("read-only")
			continue
		}
		td := findTodo(loc.Object, t.uid)
		if td == nil || td.Completed() {
			skips.add("already done")
			continue
		}
		if a.hasIncompleteChildren(t.uid) {
			skips.add("folder(s) with open subtasks")
			continue
		}
		var newObj *model.Parsed
		var err error
		seriesDone := true // plain tasks: this write always completes them
		if td.Recurring {
			newObj, seriesDone, err = model.AdvanceRecurringTodo(loc.Object, t.uid, a.now, a.loc)
		} else {
			newObj, err = model.SetTodoCompleted(loc.Object, t.uid, true, a.now, a.loc)
		}
		if err != nil {
			fail("Complete failed: " + err.Error())
			return
		}
		applied, err := a.store.PutIfUnchanged(ctx, loc.CalID, loc.Name, newObj, loc.Prev)
		if err != nil {
			fail("Complete failed: " + err.Error())
			return
		}
		if !applied {
			fail("An item changed on the server — nothing completed; retry")
			return
		}
		calID, name, prev := loc.CalID, loc.Name, loc.Prev
		rollback = append(rollback, func() { _, _ = a.store.Restore(ctx, calID, name, prev) })
		ops = append(ops, undoOp{calID: calID, name: name, prev: prev})
		// Only pin the item visible when it's actually Completed() now — a
		// recurring todo that merely advanced is still incomplete, so it stays
		// visible on its own (render.go's filter already short-circuits on
		// !Completed()); pinning it here would just be a no-op, but tracking it
		// as "sticky" when it isn't done is misleading bookkeeping.
		if seriesDone && !a.showCompleted {
			sticky = append(sticky, t.uid)
		}
		done++
	}
	if done == 0 {
		// Nothing changed — keep the selection so the user can adjust it.
		a.flash(bulkSummary("completed", 0, skips))
		return
	}
	for _, uid := range sticky {
		a.stickyDone[uid] = true
	}
	a.pushUndo("bulk complete", "", ops...)
	a.exitSelect()
	a.refreshKeepingDrill("")
	a.flash(bulkSummary("completed", done, skips) + undoHint)
}
