package ui

import (
	"context"
	"fmt"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// Yank/paste operates on one or more tasks and their whole subtrees. `y` cuts
// (move) and `Y` copies (duplicate); `p` pastes under the current tree
// selection and `P` at the list's top level. The clipboard persists after a
// paste, so a selection can be pasted repeatedly. A move within a list is a
// re-parent; across lists it recreates in the target and deletes from the
// source. A copy recreates the subtree with fresh UIDs (parent links
// remapped), leaving the original. Subtask links are UID-based, so
// descendants follow their root without link rewrites.
//
// The clipboard holds a slice of task roots (yankUIDs) rather than a single
// UID: SELECT's bulk yank (bulkYank, bulkops.go) can put several tree roots on
// it at once, and paste() applies the same move/copy to every root as ONE
// shared-rollback operation — a failure on root N rolls back roots 1..N-1 too,
// and success pushes exactly one undo step. A single-item yank (setClip) is
// just the one-element case of the same clipboard and the same paste path.

// setClip records the selected task on the clipboard; cut = move on paste, else
// copy. Both work on the current target in any view (tree, calendar, agenda).
func (a *app) setClip(cut bool) {
	t, ok := a.currentTarget()
	if !ok || !t.isTodo {
		if cut {
			a.flash("Select a task to cut (y)")
		} else {
			a.flash("Select a task to copy (Y)")
		}
		return
	}
	a.yankUIDs = []string{t.uid}
	a.yankCut = cut
	verb := "Copied"
	if cut {
		verb = "Cut"
	}
	name := ""
	if loc, ok := a.store.Locate(t.uid); ok {
		name = " \"" + oneLine(summaryOf(loc.Object, t.uid)) + "\""
	}
	a.flash(verb + name + " — p paste under · P paste at top")
}

func (a *app) yankTask() { a.setClip(true) }  // y: cut (move)
func (a *app) copyTask() { a.setClip(false) } // Y: copy (duplicate)

// pasteUnderSelection (p) pastes under the highlighted task; pasteAtTop (P)
// pastes at the list's top level.
func (a *app) pasteUnderSelection() {
	// Resolve the parent via currentTarget (the same "selected task" every other
	// action uses) rather than reading the tree node directly, so this stays correct
	// if paste is ever allowed outside the tree. paste() still gates to Tasks mode.
	targetParent := ""
	if t, ok := a.currentTarget(); ok && t.isTodo {
		targetParent = t.uid
	}
	a.paste(targetParent)
}

func (a *app) pasteAtTop() { a.paste("") }

// paste puts the clipboard under targetParent (empty = top level) in the
// selected list, as a move (cut) or duplicate (copy). The clipboard is kept
// so it can be pasted again. A single-root clipboard takes the original
// single-item path unchanged (reparentTo / moveSubtree / copySubtree); a
// multi-root clipboard (SELECT's bulk yank) takes pasteMultiRoot, which
// applies the same move/copy to every root as ONE shared-rollback operation.
func (a *app) paste(targetParent string) {
	if len(a.yankUIDs) == 0 {
		a.flash("Nothing on the clipboard (y cut / Y copy a task)")
		return
	}
	if a.mode != modeTasks {
		a.flash("Switch to a task list (t) to paste")
		return
	}
	targetCal := a.selectedTasklistID()
	if targetCal == "" {
		a.flash("No task list selected")
		return
	}
	if len(a.yankUIDs) > 1 {
		a.pasteMultiRoot(targetParent, targetCal)
		return
	}

	uid := a.yankUIDs[0]
	src, ok := a.store.Locate(uid)
	if !ok {
		a.flash("Clipboard task no longer exists")
		a.yankUIDs = nil
		return
	}

	if a.yankCut {
		// A move can't land on itself or inside its own subtree (a cycle). A copy
		// is a fresh independent subtree, so those are harmless there.
		if targetParent == uid {
			a.flash("Can't paste a task onto itself")
			return
		}
		for _, d := range a.descendants(uid) {
			if d == targetParent {
				a.flash("Can't paste a task into its own subtree")
				return
			}
		}
		if targetCal == src.CalID {
			a.reparentTo(src, targetParent)
		} else {
			a.moveSubtree(uid, targetParent, src.CalID, targetCal)
		}
		return
	}
	a.copySubtree(uid, targetParent, targetCal)
}

// reparentTo re-links the clipboard task to targetParent within its own list; its
// children come along unchanged (their UID-based links still resolve).
func (a *app) reparentTo(src store.Located, targetParent string) {
	uid := a.yankUIDs[0]
	if !a.guardWrite(src.CalID) {
		return
	}
	if td := findTodo(src.Object, uid); td != nil && td.ParentUID == targetParent {
		a.flash("Already there")
		return
	}
	obj, err := model.SetTodoParent(src.Object, uid, targetParent, a.now, a.loc)
	if err != nil {
		a.flashErr("Move", err)
		return
	}
	if _, err := a.store.Put(context.Background(), src.CalID, src.Name, obj); err != nil {
		a.flash("Move failed: " + err.Error())
		return
	}
	a.pushUndo("move task", uid, undoOp{calID: src.CalID, name: src.Name, prev: src.Prev})
	a.refresh(uid) // clipboard kept — p again to move it elsewhere
	a.flash("Moved (u to undo · still on clipboard)")
}

// pasteMultiRoot is paste()'s path for a multi-root clipboard (SELECT's bulk
// yank): every root is validated before any write runs, then moved/copied
// under ONE shared ops/rollback pair, so a cycle or a vanished root refuses
// the whole paste up front, and a write failure partway through rolls back
// every root already written in this call. Exactly one undo step on success.
func (a *app) pasteMultiRoot(targetParent, targetCal string) {
	if !a.guardWrite(targetCal) {
		return
	}

	// Validate every root up front — a cycle, a vanished root, or a read-only
	// cross-list source refuses the whole paste before any write, so a
	// multi-root paste can't half-apply.
	for _, root := range a.yankUIDs {
		loc, ok := a.store.Locate(root)
		if !ok {
			a.flash("A clipboard task no longer exists")
			a.yankUIDs = nil
			return
		}
		if a.yankCut {
			if targetParent == root {
				a.flash("Can't paste a task onto itself")
				return
			}
			for _, d := range a.descendants(root) {
				if d == targetParent {
					a.flash("Can't paste a task into its own subtree")
					return
				}
			}
			// A same-list move's source guard is covered by the guardWrite(targetCal)
			// above (source == target there); a cross-list move also touches the
			// source calendar, which needs its own check.
			if loc.CalID != targetCal && a.calReadOnly(loc.CalID) {
				a.flash("That calendar is read-only")
				return
			}
		}
	}

	var ops []undoOp      // user-facing undo (u), pushed only on full success
	var rollback []func() // reversals of committed writes, run newest-first on failure
	label := "copy task"
	if a.yankCut {
		label = "move task"
	}
	lastRoot := a.yankUIDs[len(a.yankUIDs)-1]

	for _, root := range a.yankUIDs {
		// Re-Locate right before the write (never reuse the validation-time
		// snapshot): an earlier root in this same loop may have rewritten a
		// resource this root co-resides in.
		src, ok := a.store.Locate(root)
		if !ok {
			for i := len(rollback) - 1; i >= 0; i-- {
				rollback[i]()
			}
			a.refresh(root)
			a.flash("A clipboard task vanished mid-paste — nothing applied")
			return
		}
		var err error
		switch {
		case !a.yankCut:
			err = a.copySubtreeOps(root, targetParent, targetCal, &ops, &rollback)
		case targetCal == src.CalID:
			if td := findTodo(src.Object, root); td != nil && td.ParentUID == targetParent {
				continue // already there — nothing to write for this root
			}
			err = a.reparentOps(src, root, targetParent, &ops, &rollback)
		default:
			err = a.moveSubtreeOps(root, targetParent, src.CalID, targetCal, &ops, &rollback)
		}
		if err != nil {
			for i := len(rollback) - 1; i >= 0; i-- {
				rollback[i]()
			}
			a.refresh(root)
			verb := "Copy"
			if a.yankCut {
				verb = "Move"
			}
			a.flashErr(verb, err)
			return
		}
	}
	if len(ops) == 0 {
		a.flash("Already there")
		return
	}
	a.pushUndo(label, lastRoot, ops...)
	a.refresh(lastRoot)
	if a.yankCut {
		a.flash(fmt.Sprintf("Moved %d task(s) (u to undo · still on clipboard)", len(a.yankUIDs)))
	} else {
		a.flash(fmt.Sprintf("Pasted %d copy(ies) (u to undo · p to paste again)", len(a.yankUIDs)))
	}
}

// reparentOps re-links uid to targetParent within its own list (loc.CalID),
// appending its undo/rollback onto the shared pair so a multi-root same-list
// paste is all-or-nothing across every root. Uses PutIfUnchanged against the
// Locate'd Prev — never a bare Put — per the version-check guardrail: this is
// new code for the multi-root path, so unlike the old single-item reparentTo
// it must not silently clobber a concurrent sync pull.
func (a *app) reparentOps(loc store.Located, uid, targetParent string, ops *[]undoOp, rollback *[]func()) error {
	obj, err := model.SetTodoParent(loc.Object, uid, targetParent, a.now, a.loc)
	if err != nil {
		return err
	}
	ctx := context.Background()
	applied, err := a.store.PutIfUnchanged(ctx, loc.CalID, loc.Name, obj, loc.Prev)
	if err != nil {
		return err
	}
	if !applied {
		return fmt.Errorf("an item changed on the server — retry")
	}
	calID, name, prev := loc.CalID, loc.Name, loc.Prev
	*rollback = append(*rollback, func() { _, _ = a.store.Restore(ctx, calID, name, prev) })
	*ops = append(*ops, undoOp{calID: calID, name: name, prev: prev})
	return nil
}

// moveSubtree relocates the task rooted at uid and all its descendants from
// srcCal to dstCal in one compound undo step, all-or-nothing (see
// moveSubtreeOps). Thin single-item wrapper: guards both calendars, then
// shares one ops/rollback pair of its own with the core.
func (a *app) moveSubtree(uid, targetParent, srcCal, dstCal string) {
	if !a.guardWrite(srcCal) || !a.guardWrite(dstCal) {
		return
	}
	var ops []undoOp
	var rollback []func()
	if err := a.moveSubtreeOps(uid, targetParent, srcCal, dstCal, &ops, &rollback); err != nil {
		for i := len(rollback) - 1; i >= 0; i-- {
			rollback[i]()
		}
		a.refresh(uid) // clipboard kept so the user can retry
		a.flashErr("Move", err)
		return
	}
	a.pushUndo("move task", uid, ops...)
	a.refresh(uid) // clipboard kept — p again to move it elsewhere
	a.flash("Moved to another list (u to undo · still on clipboard)")
}

// moveSubtreeOps is the write half of a subtree move: relocate uid and all its
// descendants from srcCal to dstCal, appending onto the caller's shared ops
// (undo) and rollback (failure reversal) slices. Extracted from moveSubtree so
// a multi-root paste can share ONE ops/rollback pair across every root — a
// failure on root N rolls back roots 1..N-1 too. Returns the first error, if
// any; the caller runs the rollback slice on failure and pushes undo/flashes
// on success. Callers must confirm both calendars are writable (guardWrite or
// the equivalent read-only check) before calling.
func (a *app) moveSubtreeOps(uid, targetParent, srcCal, dstCal string, ops *[]undoOp, rollback *[]func()) error {
	uids := append([]string{uid}, a.descendants(uid)...)
	ctx := context.Background()

	for _, u := range uids {
		loc, ok := a.store.Locate(u)
		if !ok {
			continue
		}
		obj := loc.Object
		if u == uid { // the moved root adopts the paste target as its parent
			edited, err := model.SetTodoParent(loc.Object, u, targetParent, a.now, a.loc)
			if err != nil {
				return err
			}
			obj = edited
		}
		// Move only the selected item: isolate it so a bundled source resource
		// doesn't drag its co-resident file-mates to the destination.
		single, err := model.IsolateComponent(obj, u, a.loc)
		if err != nil {
			return err
		}
		name := store.ResourceName(u)
		if _, err := a.store.Put(ctx, dstCal, name, single); err != nil {
			return err
		}
		// Reversal for the Put: drop the just-created copy (never synced → Forget
		// leaves no tombstone).
		*rollback = append(*rollback, func() { _ = a.store.Forget(ctx, dstCal, name) })

		// Source side: remove just u. If other items still share the resource,
		// rewrite it without u; only delete the file when u was its last item — so a
		// co-resident bystander is never erased from the source.
		reduced, remaining, err := model.RemoveComponent(loc.Object, u, a.loc)
		if err != nil {
			return err
		}
		if remaining {
			// Version-checked, never a bare Put: a sync pull updating a co-resident
			// bystander between this loop's Locate and the rewrite must fail the move
			// (caller rolls back) rather than be silently overwritten.
			applied, err := a.store.PutIfUnchanged(ctx, srcCal, loc.Name, reduced, loc.Prev)
			if err != nil {
				return err
			}
			if !applied {
				return fmt.Errorf("an item changed on the server — retry")
			}
		} else if err := a.store.Delete(ctx, srcCal, loc.Name); err != nil {
			return err
		}
		// Reversal restores the original resource (full, with u) whether it was
		// rewritten or deleted.
		srcName, srcPrev := loc.Name, loc.Prev
		*rollback = append(*rollback, func() { _, _ = a.store.Restore(ctx, srcCal, srcName, srcPrev) })

		// Undo reverses both writes: delete the new copy, restore the original.
		*ops = append(*ops,
			undoOp{calID: dstCal, name: name, prev: nil},
			undoOp{calID: srcCal, name: loc.Name, prev: loc.Prev},
		)
	}
	return nil
}

// copySubtree duplicates the task rooted at rootUID and all its descendants
// into dstCal under targetParent, all-or-nothing (see copySubtreeOps). Thin
// single-item wrapper: guards the destination, then shares one ops/rollback
// pair of its own with the core.
func (a *app) copySubtree(rootUID, targetParent, dstCal string) {
	if !a.guardWrite(dstCal) {
		return
	}
	var ops []undoOp
	var rollback []func()
	if err := a.copySubtreeOps(rootUID, targetParent, dstCal, &ops, &rollback); err != nil {
		for i := len(rollback) - 1; i >= 0; i-- {
			rollback[i]()
		}
		a.refresh(rootUID)
		a.flashErr("Copy", err)
		return
	}
	a.pushUndo("copy task", rootUID, ops...)
	a.refresh(rootUID) // clipboard kept — p again to paste another copy
	a.flash("Pasted a copy (u to undo · p to paste again)")
}

// copySubtreeOps is the write half of a subtree copy: duplicate rootUID and
// all its descendants into dstCal under targetParent with fresh UIDs (each
// child's parent link remapped to its copy), appending onto the caller's
// shared ops/rollback slices. Extracted from copySubtree for the same
// shared-rollback reason as moveSubtreeOps. The originals are never touched,
// so there is no source-side reversal — only the new copies need rolling
// back. Callers must confirm dstCal is writable (guardWrite or the equivalent
// read-only check) before calling.
func (a *app) copySubtreeOps(rootUID, targetParent, dstCal string, ops *[]undoOp, rollback *[]func()) error {
	uids := append([]string{rootUID}, a.descendants(rootUID)...)
	newUID := make(map[string]string, len(uids))
	for _, u := range uids {
		newUID[u] = model.NewUID()
	}
	ctx := context.Background()

	for _, u := range uids {
		loc, ok := a.store.Locate(u)
		if !ok {
			continue
		}
		td := findTodo(loc.Object, u)
		if td == nil {
			continue
		}
		parent := targetParent // the copied root adopts the paste target
		if u != rootUID {
			parent = newUID[td.ParentUID] // descendants point at their copied parent
		}
		// Isolate u first so a bundled source resource (multiple co-resident todos)
		// doesn't duplicate its file-mates into the destination with their original UIDs.
		single, err := model.IsolateComponent(loc.Object, u, a.loc)
		if err != nil {
			return err
		}
		obj, err := model.CopyTodo(single, u, newUID[u], parent, a.now, a.loc)
		if err != nil {
			return err
		}
		name := store.ResourceName(newUID[u])
		if _, err := a.store.Put(ctx, dstCal, name, obj); err != nil {
			return err
		}
		*rollback = append(*rollback, func() { _ = a.store.Forget(ctx, dstCal, name) })
		// Undo just deletes each new copy (the originals were never touched).
		*ops = append(*ops, undoOp{calID: dstCal, name: name, prev: nil})
	}
	return nil
}
