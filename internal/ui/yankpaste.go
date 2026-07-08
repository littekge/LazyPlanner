package ui

import (
	"context"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// Yank/paste moves a task (and its whole subtree) between lists or parents. `y`
// yanks the selected task; `p` pastes it under the current selection — a plain
// re-parent within the same list, or a cross-list move (recreate in the target
// calendar, delete from the source) when the lists differ. Because subtask links
// are UID-based, descendants follow the moved root without rewriting their links.

// yankTask records the selected task as the move source.
func (a *app) yankTask() {
	t, ok := a.currentTarget()
	if !ok || !t.isTodo {
		a.flash("Select a task to yank (y)")
		return
	}
	a.yankUID = t.uid
	if loc, ok := a.store.Locate(t.uid); ok {
		a.flash("Yanked \"" + oneLine(summaryOf(loc.Object, t.uid)) + "\" — p to paste")
	} else {
		a.flash("Yanked — p to paste")
	}
}

// pasteTask moves the yanked task under the current tree selection (or to the
// list's top level when the root is selected).
func (a *app) pasteTask() {
	if a.yankUID == "" {
		a.flash("Nothing yanked (y to yank a task)")
		return
	}
	if a.mode != modeTasks {
		a.flash("Switch to a task list (t) to paste")
		return
	}
	src, ok := a.store.Locate(a.yankUID)
	if !ok {
		a.flash("Yanked task no longer exists")
		a.yankUID = ""
		return
	}

	targetCal := a.selectedTasklistID()
	if targetCal == "" {
		a.flash("No task list selected")
		return
	}
	targetParent := ""
	if node := a.tree.GetCurrentNode(); node != nil {
		if tt, ok := node.GetReference().(*model.Todo); ok {
			targetParent = tt.UID
		}
	}

	// Guard against moving a task onto itself or into its own subtree (a cycle).
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
}

// reparentTo re-links the yanked task to targetParent within its own list; its
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
	a.yankUID = ""
	a.refresh(uid)
	a.flash("Moved (u to undo)")
}

// moveSubtree relocates the task rooted at uid and all its descendants from
// srcCal to dstCal: each resource is recreated in the target and deleted from the
// source, in one compound undo step. The moved root also adopts targetParent.
func (a *app) moveSubtree(uid, targetParent, srcCal, dstCal string) {
	if !a.guardWrite(srcCal) || !a.guardWrite(dstCal) {
		return
	}
	uids := append([]string{uid}, a.descendants(uid)...)
	ctx := context.Background()
	var ops []undoOp
	for _, u := range uids {
		loc, ok := a.store.Locate(u)
		if !ok {
			continue
		}
		obj := loc.Object
		if u == uid { // the moved root adopts the paste target as its parent
			edited, err := model.SetTodoParent(loc.Object, u, targetParent, a.now, a.loc)
			if err != nil {
				a.flash("Move failed: " + err.Error())
				return
			}
			obj = edited
		}
		name := store.ResourceName(u)
		if _, err := a.store.Put(ctx, dstCal, name, obj); err != nil {
			a.flash("Move failed: " + err.Error())
			return
		}
		if err := a.store.Delete(ctx, srcCal, loc.Name); err != nil {
			a.flash("Move failed: " + err.Error())
			return
		}
		// Undo reverses both writes: delete the new copy, restore the original.
		ops = append(ops,
			undoOp{calID: dstCal, name: name, prev: nil},
			undoOp{calID: srcCal, name: loc.Name, prev: loc.Prev},
		)
	}
	a.pushUndo("move task", uid, ops...)
	a.yankUID = ""
	a.refresh(uid)
	a.flash("Moved to another list (u to undo)")
}
