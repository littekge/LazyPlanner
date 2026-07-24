# v1.4.0 — SELECT Mode: Design

> Approved design for the vim-style multi-select layer (owner-settled 2026-07-23).
> Scope source: `main.md` § Build Plan → v1.4.0. Implementation plan follows separately.

## Purpose

Select a contiguous range of items — tasks in the tree, days in the calendar, or
events within a drilled day — then run one bulk action over the whole range:
complete, delete, yank/copy, or grab. Benchmark workflows (owner): **tree
cleanup** (bulk complete/delete a run of finished tasks) and **tree
reorganization** (yank a run of siblings, paste under a new parent).

The design core is **mode composition**: SELECT is one more derived interaction
mode on the existing `interactionMode` seam — a `selecting` bool plus an anchor,
never a parallel mode enum (hard-won guardrail). Modes nest: DRILL → SELECT →
GRAB.

## Settled decisions (owner, 2026-07-23)

| Decision | Choice |
|---|---|
| Selection model | **Contiguous range only** (vim `V` line-visual feel): anchor + cursor; movement extends. No toggle-set. |
| Entry key | **`V`** (Shift-v). `v` keeps cycling calendar views. |
| Contexts | Task tree, un-drilled calendar days, drilled-day items. **Agenda excluded.** |
| Bulk ops | Complete (`Space`), delete (`d`), yank/copy (`y`/`Y`), grab (`m`). Quick-sets (`s`), re-parent (`H`/`L`) stay single-item. |
| Recurring events | **Skipped by bulk ops** with a count flash ("N recurring skipped — act on them individually"). Recurring *todos* participate via their single-live-instance semantics. |
| Architecture | **Derived range, anchor-only state** (Approach 1): the range is computed from anchor → cursor on demand; nothing else is stored. |

## Architecture: derived range

App-level state: `selecting bool` + one anchor per context —

- tree: anchor **UID** (row index looked up live, like `currentTreeUID`),
- calendar: anchor **date**,
- drilled day: anchor **item identity** (`uid` + `occStart`).

The selected range is always *derived*: "everything between the anchor and the
current cursor in visible order", recomputed for every draw and materialized to
`[]editTarget` only when an op fires. Consequences:

- **Refresh-proof by construction** — no stored set to re-map when a sync
  rebuilds a view (the v1.0.1 cursor-reset bug class can't recur here). If the
  anchor itself vanishes (item deleted remotely mid-select), SELECT exits with
  a flash rather than guessing.
- **Draw-lock safe** — each view receives the range as a plain field/callback
  (the existing `itemColor` wiring pattern); draw paths never call app-lock
  methods (hard-won guardrail).

## Mode semantics

- **Enter**: `V` from the task tree, an un-drilled calendar grid, or a drilled
  day; the anchor is the current cursor position. Anywhere else (Calendars /
  Tasks / Agenda overview panels) flashes a hint.
- **Badge**: `interactionMode()` gains one branch; priority order is innermost
  mode wins: `RESIZE → GRAB → form (DRILL/NORMAL) → SELECT → DRILL → NORMAL`.
  A bulk grab therefore shows GRAB; a confirm dialog shows the form badge.
- **Exit**: `Esc` (or `V` again) exits one nesting level — back to DRILL if
  entered from a drilled day, else NORMAL. A completed bulk op exits SELECT
  (vim: the action ends visual mode).
- **Nested grab unwind**: `Enter` commits the grab and exits both GRAB and
  SELECT. `Esc` reverts the grab and returns to SELECT with the range intact
  (retry-friendly).

## Key layer

`handleSelectKey` slots into `globalKeys` as a peer of `handleGrabKey` /
`handleResizeKey` (after the modal branch). It is mostly pass-through:

- **Motion passes through**: counts, `hjkl`/arrows, `gg`/`G`; in the calendar
  also `f`/`b` (the day range is a date interval and may span periods). Moving
  the cursor *is* extending the range.
- **Handled**: `Space`, `d`, `y`, `Y`, `m`, `Esc`, `V`.
- **Swallowed (inert)**: context switches (`c`/`t`/`a`, `Tab`, `[`/`]`,
  `{`/`}`, `v`) and structure/data mutators (fold keys, `.`, `H`/`L`, `e`,
  `u`, `r`, `/`, create/edit prefixes) — they would change the visible order
  or the data under the range. Esc first, then act. Search-extend (`/`) is a
  possible later addition.

## Range per context

**Task tree** (benchmark context). Range = the *visible rows* between anchor
and cursor, inclusive, in display order. The list-name root is never
selectable. Complete acts on each selected row individually under the existing
single-item rules (a folder with incomplete children is skipped and counted).
Delete and yank act on rows **with their subtrees**, deduped: a selected row
whose ancestor is also selected is absorbed into the ancestor's subtree.

**Calendar days** (un-drilled month/week/day). Range = the date interval
`[min(anchor, cursor), max(anchor, cursor)]`. Ops materialize every visible
item occurring on those days — hidden calendars excluded; a multi-day event
spanning several selected days counts once.

**Drilled day**. Range = the day's items between anchor and cursor in the
drill-cycling order (the same linear order `Enter` cycles; the 2D time-grid
uses its underlying linear item order).

**Yank is tree-only** in v1.4.0 — the clipboard is a task-subtree concept;
`y`/`Y` in the calendar contexts flash "yank works in the task tree".

## Bulk-op engine

One shared pipeline: **materialize → filter → execute → summarize**.

1. **Materialize** the range into `[]editTarget` (uid, occStart, isTodo).
2. **Filter**, counting every skip for the summary: recurring events (all
   ops), read-only-calendar items (`guardWrite` per item), undated tasks
   (grab), folders with incomplete children (complete).
3. **Execute** on the `moveSubtree` template: each write goes through
   `PutIfUnchanged`, accumulating `rollback []func()` (reversals, run
   newest-first) and `ops []undoOp`. Any hard failure **or stale write** rolls
   back everything committed so far and flashes — all-or-nothing; a stale
   result (concurrent sync) aborts rather than clobbers. On success, **one
   compound `pushUndo` step**: `u` restores the entire bulk op.
4. **Summarize & exit**: one flash (`12 completed · 2 skipped (recurring)`),
   SELECT exits, the view refreshes.

Op-specific notes:

- **Delete** keeps a single confirm naming the full count including subtasks
  (`Delete 8 items (14 with subtasks)?`). Item deletes remain undoable, so the
  ordinary confirm (not type-to-confirm) is correct.
- **Yank/copy** builds one multi-subtree clipboard payload; `p`/`P` paste all
  subtrees together (order preserved), and the clipboard persists for
  multi-paste as today. Copy remaps UIDs/RELATED-TO per subtree via the
  existing `model.CopyTodo`.
- **Complete** on a recurring todo advances it (existing single-live-instance
  semantics apply per item).

## Bulk grab: uniform date-shift

Single-item grab has per-type key meanings (events ±hour, tasks ±day), which is
incoherent over a mixed selection. Bulk grab is therefore a **pure date
shift**, identical in all three contexts: `h`/`l` = ±1 day, `j`/`k` = ±1 week,
times-of-day untouched, `J`/`K` (resize) disabled. Mechanics mirror single
grab: per-item snapshots at entry; every nudge recomputes and commits each item
version-checked (`PutIfUnchanged`); `Enter` keeps (one compound undo step);
`Esc` restores all snapshots and returns to SELECT; a stale write mid-grab
aborts the whole grab without reverting (consistent with `abortGrabStale`).

## Rendering & status

- Each view receives the range as a plain field (no app-lock calls from draw
  paths). Tree rows in range: theme-adaptive reverse-video (`selectionStyle`,
  per the legibility guardrail). Month-grid days in range: each draws the
  outline box, the cursor day keeping the focused style. Drilled items in
  range: reverse-video, cursor item still visually distinguished.
- Status bar: the general status line shows `N selected` while ranging; the
  hints line shows the SELECT controls (`Space done · d delete · y yank ·
  m grab · Esc`); the badge gains a `SELECT` chip (fits `modeIndicatorWidth`).

## Testing

Per the audit guardrails (test both sides of every boundary; mirror guards
onto sibling paths):

- **Mode**: badge cases for SELECT and the nesting chain (DRILL → SELECT →
  GRAB shows GRAB; Esc unwinds exactly one level); `modedeadlock_test.go`
  stays green.
- **Range derivation tables** per context: reversed anchor/cursor, `gg`/`G`
  extremes, single-item range, ancestor-dedupe, anchor-vanished-on-sync exits
  cleanly.
- **Bulk ops**: rollback on injected mid-batch failure; stale abort; one
  undo step restores everything; skip counts (recurring / read-only / undated
  / folder); delete confirm counts; multi-payload yank + paste; bulk-grab
  shift and Esc-revert; a `-race` sync-during-select stress in the
  `TestConcurrentSyncAndEditsRace` mold.
- **Display**: `displaystress_test.go` extended to draw every view with a
  non-empty range over hostile content across the standard geometries;
  reverse-video legibility tests for the new visuals (the selection-style
  guardrail class).

## Out of scope (v1.4.0)

- Toggle-set / non-contiguous selection (the derived range materializes into a
  set if this is ever added — no rework wasted).
- Agenda-list SELECT.
- Bulk quick-sets (`sp`/`sd`), bulk re-parent (`H`/`L`), bulk edit (`e`).
- Search-extend (`/` as a range motion).
- Scope-prompt-once for recurring events in bulk ops.
