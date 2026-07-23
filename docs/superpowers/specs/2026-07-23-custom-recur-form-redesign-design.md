# Custom recurrence sub-form redesign — design

> Status: approved 2026-07-23. The second of the two v1.3.0 pre-release UI items
> (the first — rigorous collection-delete confirm — shipped in commits
> 694ee10..dc70344).

## Problem

The Custom… repeat sub-form (`internal/ui/recurcustom.go`) is **static**: it
renders all 13 fields at once regardless of what's relevant. The weekday
checkboxes show for a monthly/yearly rule, the "Monthly by" dropdown shows when
weekly, and both "Until" and "Count" show regardless of the "Ends" choice. The
result is a tall (fixed 22-row), dense wall of mostly-irrelevant fields, and
weekday selection is seven separate checkbox rows to tab through.

Owner-flagged pain points (confirmed 2026-07-23): irrelevant fields shown at
once; the 7 weekday checkboxes are tedious; the form is too tall/dense. (Field
order/flow was **not** a complaint — the sequence stays.)

## Goal

Rework the sub-form's layout for a lighter, less dense feel: show only the fields
relevant to the current selection, and replace the seven weekday checkboxes with
a single compact toggle strip. Pure UI-layer change — the model
(`RecurSpec`, the frequency-gated read) is untouched.

## Design

### Field set & dynamic visibility

Always visible: **Every [N]**, **Unit** (days/weeks/months/years), **Ends**
(Never / On date / After N times). The rest appear conditionally:

| Field | Shows when |
|---|---|
| Weekday strip | Unit = **weeks** |
| Monthly by (dropdown) | Unit = **months** |
| Until (YYYY-MM-DD) | Ends = **On date** |
| Count | Ends = **After N times** |

So the form is **3–5 fields** at any moment instead of 13. Changing **Unit** or
**Ends** re-lays-out the form immediately (fields appear/disappear), preserving
values already entered in fields that stay visible.

Example — daily/never vs. weekly-on-Tue/Thu ending on a date:

```
┌─ Custom repeat ──────┐     ┌─ Custom repeat ─────────────────────┐
│ ▸ Every [1] days     │     │ ▸ Every [1] weeks                   │
│   Ends  [Never ▾]    │     │   Repeat on ‹Mon[Tue]Wed[Thu]Fri…›  │
│                      │     │   Ends  [On date ▾]                 │
│    [ OK ] [ Cancel ] │     │   Until [2026-12-31]                │
└──────────────────────┘     │    [ OK ]  [ Cancel ]               │
                             └─────────────────────────────────────┘
```

### The weekday strip widget

A new custom widget — a single-row primitive implementing `tview.FormItem`, so it
lives inside the `caretForm` and participates in nav, focus, and the caret gutter
like any field. This reuses the app-wide NORMAL/DRILL model the forms were built
around (the owner confirmed DRILL was designed for exactly this):

```
▸ Repeat on:  Mon [Tue] Wed [Thu] Fri  Sat  Sun
```

- **NORMAL** (not drilled): `j`/`k`/arrows step onto it like any field; `Enter`
  **drills in**.
- **DRILL** (drilled in): `←`/`→` (and `h`/`l`) move a cursor between the seven
  days; **Space** toggles the day under the cursor (`[Tue]` = selected); `Enter`
  commits and advances to the next field (auto-drilling per the existing rule);
  `Esc` returns to NORMAL keeping the selection. The mode badge shows **DRILL**
  while inside, same as a text field.
- Selected days render reverse-video via the shared `selectionStyle` (the
  theme-adaptive style the guardrail requires). The day-cursor is rendered
  distinctly from selection (e.g. an accent underline/box) so "which day is
  focused" reads separately from "which days are on".
- Seeded from the incoming rule's weekday set; empty → defaults to the anchor's
  weekday (same fallback as today). At read, the selected days become
  `spec.Weekdays` (Monday-first order, matching `mondayOrder`).

Because it is a real `FormItem`, the caretForm Draw gutter (`▸` caret) and the
DRILL routing extend to it by adding one case to the existing type-switch — no
parallel nav path. It is treated as a drillable item (like a text field) for the
NORMAL `Enter`-to-drill / DRILL-to-`Esc` flow.

### Rebuild mechanism

Persistent widgets, re-laid-out. All field widgets are built once and kept alive
(holding their values). A `layout()` step adds only the currently-relevant subset
to the `caretForm` in order, rebuilding the form's item list and the caret-gutter
`labels[]` slice in sync. When **Unit** or **Ends** changes, their
`SetSelectedFunc` callbacks call `layout()`: `Clear(false)` the form items, re-add
the relevant subset + the OK/Cancel buttons. Values in still-visible fields
survive because the widgets persist. Focus lands on the field that triggered the
relayout (the Unit or Ends dropdown) so the cursor doesn't jump unexpectedly.

Guard against callback re-entry: `SetCurrentOption`/`Clear` can refire the
selected-func; `layout()` sets a re-entry flag while rebuilding (the existing
`wireRepeatCustom` already uses this pattern).

### Height

The strip collapses weekdays from 7 rows to 1, so the tallest configuration
(weekly + On-date = 5 fields) is ~11 rows vs. today's 22. Use a **fixed height
sized to that worst case** rather than resizing the modal on every relayout:
dynamic modal resizing in tview forces a page re-add that flickers and resets
focus. Trailing blank rows in the short cases sit on the terminal-default
background — empty space, not a heavy card — so the form still reads far lighter.
(Dynamic resize is a possible later enhancement, out of scope.)

## Preserved behavior (must keep working)

- The nested-modal focus-stack: opens over the item form via `pageRepeat`;
  OK/Cancel/Esc restore focus to the item form, not the calendar (guardrail).
- Seeding from the current selection (`SeedSpec`), write-back via `SetCustom`,
  Cancel restoring the prior selection (`wireRepeatCustom`).
- `monthlyOptions` anchor-derivation (a monthly rule can't contradict its start
  date) and the anchor-weekday fallback for an empty weekday set.
- Validation: interval ≥ 1, valid `Until` date, count ≥ 1 — same messages, still
  flash-and-stay-open on error.
- The model layer (`RecurSpec`, `readCustomRecur`'s frequency-gated read) is
  untouched; irrelevant-to-frequency fields are simply not shown now, but the read
  stays defensive (reads only what the chosen frequency needs).

## Guardrails triggered by the new widget

- `selectionStyle` on the strip's selected cells, with a reverse-video
  legibility test (the class that has reappeared twice — Lists and DropDowns).
- Display-stress test across 1×1→400×150 for the new `Draw` path (extend
  `displaystress_test.go`), and the dynamic relayout exercised under the same
  harness so a rebuild mid-draw can't panic/freeze.

## Testing (repro-first)

- **Strip widget unit tests:** seed→render, `←`/`→` cursor movement, Space
  toggle, read-back of the selected set, empty→anchor fallback, and the
  reverse-video selection legibility.
- **Relayout tests:** Unit weeks→months hides the strip and shows "Monthly by";
  Ends Never→On date reveals "Until"; values in surviving fields (Every, Ends,
  Until/Count) are preserved across a relayout.
- **Read tests** per frequency/end condition (reuse/extend
  `recurcustom_test.go`).
- **Focus-stack test** still green (nest over the item form, unwind to the form).
- **Display-stress** for the new widget + relayout.

## Files

- `internal/ui/recurcustom.go` — rework `newCustomRepeatForm` into a persistent
  widget set + a `layout()` step; wire Unit/Ends relayout; use the new strip in
  place of the 7 checkboxes.
- New widget file, e.g. `internal/ui/weekdaystrip.go` — the `tview.FormItem`
  implementation (Draw, key handling, seed/read).
- `internal/ui/forms.go` — one case added to the caretForm Draw gutter
  type-switch and to the DRILL drillable-item check for the new widget type.
- Tests: `internal/ui/recurcustom_test.go` (extend), a new
  `internal/ui/weekdaystrip_test.go`, and an addition to `displaystress_test.go`.
- Docs: `main.md`, `README.md` (if wording changes), `:help` (if key note
  needed), `log.md`.

## Non-goals / deferred

- Dynamic modal height resizing on relayout (fixed-to-worst-case for now).
- Any change to the recurrence model, the Repeat dropdown presets, or the
  quick-add recurrence grammar.
- Reordering the field sequence (owner did not flag flow).
