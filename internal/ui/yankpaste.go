package ui

import (
	"context"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// Yank/paste operates on a task and its whole subtree. `y` cuts (move) and `Y`
// copies (duplicate); `p` pastes under the current tree selection and `P` at the
// list's top level. The clipboard persists after a paste, so an item can be
// pasted repeatedly. A move within a list is a re-parent; across lists it
// recreates in the target and deletes from the source. A copy recreates the
// subtree with fresh UIDs (parent links remapped), leaving the original. Subtask
// links are UID-based, so descendants follow the root without link rewrites.

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
	a.yankUID = t.uid
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

// paste puts the clipboard task under targetParent (empty = top level) in the
// selected list, as a move (cut) or duplicate (copy). The clipboard is kept so
// the item can be pasted again.
func (a *app) paste(targetParent string) {
	if a.yankUID == "" {
		a.flash("Nothing on the clipboard (y cut / Y copy a task)")
		return
	}
	if a.mode != modeTasks {
		a.flash("Switch to a task list (t) to paste")
		return
	}
	src, ok := a.store.Locate(a.yankUID)
	if !ok {
		a.flash("Clipboard task no longer exists")
		a.yankUID = ""
		return
	}
	targetCal := a.selectedTasklistID()
	if targetCal == "" {
		a.flash("No task list selected")
		return
	}

	if a.yankCut {
		// A move can't land on itself or inside its own subtree (a cycle). A copy
		// is a fresh independent subtree, so those are harmless there.
		if targetParent == a.yankUID {
			a.flash("Can't paste a task onto itself")
			return
		}
		for _, d := range a.descendants(a.yankUID) {
			if d == targetParent {
				a.flash("Can't paste a task into its own subtree")
				return
			}
		}
		if targetCal == src.CalID {
			a.reparentTo(src, targetParent)
		} else {
			a.moveSubtree(a.yankUID, targetParent, src.CalID, targetCal)
		}
		return
	}
	a.copySubtree(a.yankUID, targetParent, targetCal)
}

// reparentTo re-links the clipboard task to targetParent within its own list; its
// children come along unchanged (their UID-based links still resolve).
func (a *app) reparentTo(src store.Located, targetParent string) {
	uid := a.yankUID
	if !a.guardWrite(src.CalID) {
		return
	}
	if td := findTodo(src.Object, uid); td != nil && td.ParentUID == targetParent {
		a.flash("Already there")
		return
	}
	obj, err := model.SetTodoParent(src.Object, uid, targetParent, a.now, a.loc)
	if err != nil {
		a.flash(err.Error())
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

// moveSubtree relocates the task rooted at uid and all its descendants from
// srcCal to dstCal: each resource is recreated in the target and deleted from the
// source, in one compound undo step. The moved root also adopts targetParent.
// The move is all-or-nothing: if any step fails partway, the already-moved nodes
// are rolled back so the subtree never ends up split across the two lists.
func (a *app) moveSubtree(uid, targetParent, srcCal, dstCal string) {
	if !a.guardWrite(srcCal) || !a.guardWrite(dstCal) {
		return
	}
	uids := append([]string{uid}, a.descendants(uid)...)
	ctx := context.Background()

	var ops []undoOp      // user-facing undo (u), pushed only on full success
	var rollback []func() // reversals of committed writes, run newest-first on failure
	fail := func(msg string) {
		for i := len(rollback) - 1; i >= 0; i-- {
			rollback[i]()
		}
		a.refresh(uid) // clipboard kept so the user can retry
		a.flash(msg)
	}

	for _, u := range uids {
		loc, ok := a.store.Locate(u)
		if !ok {
			continue
		}
		obj := loc.Object
		if u == uid { // the moved root adopts the paste target as its parent
			edited, err := model.SetTodoParent(loc.Object, u, targetParent, a.now, a.loc)
			if err != nil {
				fail("Move failed: " + err.Error())
				return
			}
			obj = edited
		}
		name := store.ResourceName(u)
		if _, err := a.store.Put(ctx, dstCal, name, obj); err != nil {
			fail("Move failed: " + err.Error())
			return
		}
		// Reversal for the Put: drop the just-created copy (never synced → Forget
		// leaves no tombstone).
		rollback = append(rollback, func() { _ = a.store.Forget(ctx, dstCal, name) })

		if err := a.store.Delete(ctx, srcCal, loc.Name); err != nil {
			fail("Move failed: " + err.Error())
			return
		}
		// Reversal for the Delete: restore the original (clears its tombstone).
		srcName, srcPrev := loc.Name, loc.Prev
		rollback = append(rollback, func() { _, _ = a.store.Restore(ctx, srcCal, srcName, srcPrev) })

		// Undo reverses both writes: delete the new copy, restore the original.
		ops = append(ops,
			undoOp{calID: dstCal, name: name, prev: nil},
			undoOp{calID: srcCal, name: loc.Name, prev: loc.Prev},
		)
	}
	a.pushUndo("move task", uid, ops...)
	a.refresh(uid) // clipboard kept — p again to move it elsewhere
	a.flash("Moved to another list (u to undo · still on clipboard)")
}

// copySubtree duplicates the task rooted at rootUID and all its descendants into
// dstCal under targetParent, with fresh UIDs (each child's parent link remapped
// to its copy). The originals are untouched; the whole copy is one undo step and
// rolls back on a partial failure.
func (a *app) copySubtree(rootUID, targetParent, dstCal string) {
	if !a.guardWrite(dstCal) {
		return
	}
	uids := append([]string{rootUID}, a.descendants(rootUID)...)
	newUID := make(map[string]string, len(uids))
	for _, u := range uids {
		newUID[u] = model.NewUID()
	}
	ctx := context.Background()

	var ops []undoOp
	var rollback []func()
	fail := func(msg string) {
		for i := len(rollback) - 1; i >= 0; i-- {
			rollback[i]()
		}
		a.refresh(rootUID)
		a.flash(msg)
	}

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
		obj, err := model.CopyTodo(loc.Object, u, newUID[u], parent, a.now, a.loc)
		if err != nil {
			fail("Copy failed: " + err.Error())
			return
		}
		name := store.ResourceName(newUID[u])
		if _, err := a.store.Put(ctx, dstCal, name, obj); err != nil {
			fail("Copy failed: " + err.Error())
			return
		}
		rollback = append(rollback, func() { _ = a.store.Forget(ctx, dstCal, name) })
		// Undo just deletes each new copy (the originals were never touched).
		ops = append(ops, undoOp{calID: dstCal, name: name, prev: nil})
	}
	a.pushUndo("copy task", rootUID, ops...)
	a.refresh(rootUID) // clipboard kept — p again to paste another copy
	a.flash("Pasted a copy (u to undo · p to paste again)")
}
