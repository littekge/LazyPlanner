# Rigorous type-to-confirm for collection deletes — design

> Status: approved 2026-07-23. One of the two v1.3.0 pre-release UI items.
> Companion feature (custom-recurrence form redesign) is a separate spec.

## Problem

Deleting a calendar or task list is **not undoable** — `deleteCollection`
(`internal/ui/calendar.go`) pushes no undo op, unlike item deletes, yet it gates
on the ordinary one-button `confirm` (a single `Delete` press). A stray keystroke
can wipe a whole collection and everything in it, locally and — on the next sync —
on the server, with no recovery. Item deletes, by contrast, are undoable (`u`),
so their light confirm is appropriate.

The fix: give collection deletes a **stronger, distinct confirmation** — the user
must type the collection's name before the delete can fire.

## Scope

- **In scope**: collection deletes only — a calendar (`d`/`D` on the focused
  Calendars pane) or a task list (focused Tasks pane), routed through
  `deleteCollection`. Applies to **empty and non-empty** collections alike, since
  none is undoable.
- **Out of scope**: item (event/task/subtask) deletes — they stay on the ordinary
  `confirm`, because they are undoable and a heavyweight gate there is friction for
  no safety gain. No changes to `store` or `model`. The existing
  `confirm`/`confirmOK`/`styleModal` helpers are untouched (still used by item
  deletes, the recurrence scope picker, conflict resolve, etc.).

## Design

### Dialog

`deleteCollection` keeps its existing early guards — must be on the Calendars or
Tasks pane, something selected, the collection not read-only — then, instead of
calling `a.confirm(...)`, opens a small `caretForm` (the same primitive as the
calendar edit form, so it inherits the shared popup chrome, the focus-stack, and
the NORMAL/DRILL navigation model).

```
┌─ ⚠ Delete list "School" — cannot be undone ──────┐
│ ▸ Type name to confirm: [ School█            ]   │
│                                                  │
│              [ Delete ]   [ Cancel ]             │
└──────────────────────────────────────────────────┘
```

- **Title** carries the gravity, mode-aware (calendar vs. list):
  `⚠ Delete calendar "<name>" — cannot be undone` /
  `⚠ Delete list "<name>" — cannot be undone`. The item count folds into the
  title when the collection is non-empty and space permits (e.g.
  `… "School" (12 items) …`).
- One input field, then **Delete** and **Cancel** buttons.
- The warning text lives in the **title + input label**, not a separate text line:
  `openModal` type-asserts its primitive to `*caretForm` to wire DRILL navigation
  and `appFocus`, so wrapping the form in a `Flex` to add a `TextView` blurb would
  break form navigation. Title + label is how the existing calendar edit form
  conveys its context, so this is the idiomatic, low-risk path.

### Interaction

The form opens in NORMAL with the caret on the input field (standard form flow, no
special-casing):

- `Enter` drills into the field → type the name → `Enter` commits and advances,
  landing in NORMAL on the **Delete** button → `Enter` fires it.
- `Esc`, the **Cancel** button, and `SetCancelFunc` all close the form with no
  action.

### Match + validation

A small pure, unit-testable helper:

```go
// collectionDeleteNameMatches reports whether the typed text confirms deletion of
// the collection named name: trim + case-sensitive (both sides trimmed so a stored
// stray space can't make a name impossible to type).
func collectionDeleteNameMatches(typed, name string) bool {
    return strings.TrimSpace(typed) == strings.TrimSpace(name)
}
```

On **Delete**:

- **Match** → run the existing deletion body unchanged: `MarkCalendarDeleted` →
  `refresh("")` → `scheduleSyncDebounced()` → flash `Deleted "<name>"`; close the
  modal.
- **Mismatch** → `a.flash("Name doesn't match — type it exactly to delete")` and
  **return without closing**, so the form stays open for another attempt. This
  mirrors the calendar form's existing validate-and-flash convention (e.g.
  `"A name is required"`, `"Invalid color …"`).

## Files

- `internal/ui/calendar.go` — replace the `a.confirm(...)` call in
  `deleteCollection` with a new helper (e.g. `promptDeleteCollection`) that builds
  and opens the `caretForm`; add `collectionDeleteNameMatches`. Reuses the
  `pageForm` modal page (no other form is open when `d` fires on a pane).
- No changes to `internal/store` or `internal/model`.

## Testing (repro-first)

- **Unit table** for `collectionDeleteNameMatches`: exact match, trimmed match
  (leading/trailing whitespace), wrong case (rejected), empty typed (rejected),
  name with internal spaces.
- **UI test** in `internal/ui`: open the form on a calendar that has items; press
  **Delete** with a **wrong** name → assert the calendar still exists and the form
  is still open; type the **correct** name and press **Delete** → assert the
  calendar is gone and the modal closed. Follows the existing form-test patterns.
- The shared `caretForm` is already covered by `displaystress_test.go`, so no new
  display-stress harness is needed.

## Docs ripple (same increment)

- `main.md` — move the "rigorous confirm" item from v1.3.0's "Planned before
  release" into the shipped Post-Build list; update the collection-delete prose
  (Creation / Folders sections mention deletion) to describe the type-to-confirm.
- `README.md` — note that deleting a calendar/list requires typing its name to
  confirm (update the delete keybinding row / Managing Calendars prose).
- `:help` (`internal/ui/help.go`) — nuance the delete row if needed.
- `log.md` — dated completion entry.

## Non-goals / deferred

- No trash/undo for collection deletes (undo stays session-scoped and item-level;
  persistent trash remains a deferred idea).
- Match strictness is fixed at trim + case-sensitive; not configurable.
