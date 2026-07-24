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

// bulkDeleteRoots dedupes the range for subtree-shaped ops: a selected row
// whose ancestor is also selected is absorbed (its subtree travels with the
// ancestor), and recurring events / read-only / missing items are filtered out
// with counts. Also used by bulk yank.
//
// Two passes, deliberately: absorption must be checked against the *surviving*
// selection, not the raw targets. A child whose only selected ancestor gets
// filtered out (read-only/missing) would otherwise be silently absorbed into a
// delete that never happens to that ancestor — dropped from roots, uncounted
// in skips, and the confirm's count would no longer match what's deleted.
func (a *app) bulkDeleteRoots(targets []editTarget) ([]editTarget, bulkSkip) {
	skips := bulkSkip{}
	parentOf := map[string]string{}
	for _, td := range a.store.Todos() {
		parentOf[td.UID] = td.ParentUID
	}

	survivors := make([]editTarget, 0, len(targets))
	for _, t := range targets {
		if !t.isTodo && t.recurring {
			skips.add("recurring")
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
		survivors = append(survivors, t)
	}

	selected := map[string]bool{}
	for _, t := range survivors {
		if t.isTodo {
			selected[t.uid] = true
		}
	}

	var roots []editTarget
	for _, t := range survivors {
		if t.isTodo {
			absorbed := false
			// ParentUID comes straight from untrusted RELATED-TO data (hand-edited
			// or foreign .ics), so a reciprocal cycle is possible — seen guards
			// against walking it forever, mirroring descendants()'s cycle guard.
			seen := map[string]bool{t.uid: true}
			for p := parentOf[t.uid]; p != "" && !seen[p]; p = parentOf[p] {
				seen[p] = true
				if selected[p] {
					absorbed = true
					break
				}
			}
			if absorbed {
				continue // travels with its selected (surviving) ancestor's subtree
			}
		}
		roots = append(roots, t)
	}
	return roots, skips
}

// bulkYank (y/Y in SELECT) puts the selected task roots on the clipboard —
// tree context only: the clipboard is a task-subtree concept, and paste needs
// a tree target. Roots are the ancestor-deduped range in visible order; each
// root's subtree travels with it on paste, exactly like a single-item yank.
func (a *app) bulkYank(cut bool) {
	if a.selContext() != selTree {
		a.flash("Yank works in the task tree (t)")
		return
	}
	targets := a.selRange()
	if targets == nil {
		a.exitSelect()
		a.flash("Selection no longer valid")
		return
	}
	roots, skips := a.bulkDeleteRoots(targets)
	if len(roots) == 0 {
		a.flash(bulkSummary("on clipboard", 0, skips))
		return
	}
	a.yankUIDs = a.yankUIDs[:0]
	for _, r := range roots {
		a.yankUIDs = append(a.yankUIDs, r.uid)
	}
	a.yankCut = cut
	verb := "Copied"
	if cut {
		verb = "Cut"
	}
	a.exitSelect()
	a.flash(fmt.Sprintf("%s %d task(s) — p paste under · P paste at top", verb, len(a.yankUIDs)))
}

// bulkDelete (d in SELECT) deletes every selected item — tasks with their whole
// subtrees — after one confirm naming the full count. Mirrors deleteWholeObject's
// semantics exactly: whole-resource delete per uid, no scope picker (a recurring
// todo's resource is its series — the spec's settled "natural meaning"; a
// recurring event has no such single-resource meaning in bulk, so it's filtered
// out by bulkDeleteRoots instead). All-or-nothing with rollback; one undo step
// restores everything.
func (a *app) bulkDelete() {
	targets := a.selRange()
	if targets == nil {
		a.exitSelect()
		a.flash("Selection no longer valid")
		return
	}
	roots, skips := a.bulkDeleteRoots(targets)
	if len(roots) == 0 {
		a.flash(bulkSummary("deleted", 0, skips))
		return
	}
	// Expand each task root with its descendants, deduped across roots.
	var uids []string
	seen := map[string]bool{}
	for _, r := range roots {
		for _, u := range append([]string{r.uid}, a.descendants(r.uid)...) {
			if !seen[u] {
				seen[u] = true
				uids = append(uids, u)
			}
		}
	}
	prompt := fmt.Sprintf("Delete %d item(s)?", len(roots))
	if len(uids) > len(roots) {
		prompt = fmt.Sprintf("Delete %d item(s) (%d with subtasks)?", len(roots), len(uids))
	}
	a.confirm(" Delete selection ", prompt, func() {
		ctx := context.Background()
		var ops []undoOp
		var rollback []func()
		deleted := 0
		for _, u := range uids {
			loc, ok := a.store.Locate(u)
			if !ok {
				continue
			}
			if err := a.store.Delete(ctx, loc.CalID, loc.Name); err != nil {
				for i := len(rollback) - 1; i >= 0; i-- {
					rollback[i]()
				}
				a.exitSelect()
				a.refresh("")
				a.flash("Delete failed: " + err.Error())
				return
			}
			calID, name, prev := loc.CalID, loc.Name, loc.Prev
			rollback = append(rollback, func() { _, _ = a.store.Restore(ctx, calID, name, prev) })
			ops = append(ops, undoOp{calID: calID, name: name, prev: prev})
			deleted++
		}
		if deleted == 0 {
			// Nothing was actually deleted — every uid vanished (a race between
			// materializing roots and the confirm firing). Mirrors bulkComplete's
			// done==0 guard: no undo step for a no-op, and the selection is left
			// alone so the user can see what happened and adjust it.
			a.flash(bulkSummary("deleted", 0, skips))
			return
		}
		a.pushUndo("bulk delete", "", ops...)
		a.exitSelect()
		a.refresh("")
		a.flash(bulkSummary("deleted", deleted, skips) + undoHint)
	})
}
