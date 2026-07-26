# Pass 23 — never-audited parser/UI-orchestration surfaces + the deferred spec-diff

**Date**: 2026-07-25
**Branch / baseline**: `ai-workspace` @ `ba274c1`
**Methods this pass**: fuzz (×2), data-loss, spec-diff, input-edge (×2)
**Prior pass (22)**: 0 HIGH / 1 MED / 0 LOW

> This is an evidence report, not a verdict. "No finding" on a target is bounded evidence about that
> surface under that one method — never a statement about the package, and never a claim the code is clean.

---

## Why these six targets

Five of the six had **never** been audited by any pass; the sixth (feature-promise spec-diff) was last run
at pass 20 and was explicitly skipped by passes 21 and 22 — it appeared in pass 22's own residual list.

| # | Surface | Package | Method | Prior coverage |
|---|---|---|---|---|
| 1 | RRULE decomposition — `RecurSpecFromRule` / `decodeMonthly` / `decodeYearly` / `nthMatchesAnchor` | internal/model | fuzz | **never** |
| 2 | Conflict-resolution UI orchestration — `showConflicts` / `populateConflicts` / `chooseResolution` | internal/ui | data-loss | **never** (store-side `ResolveKeep*` only, pass 20) |
| 3 | Sidecar metadata **parse** — `readSidecar` → reconcile inputs | internal/store | fuzz | **never** (write atomicity only: 10/15/22) |
| 4 | Feature-promise conformance vs `main.md` (v1.5.0 phase-2/3 + the pass-21/22 fix arcs) | (whole app) | spec-diff | pass 20, skipped 21 & 22 |
| 5 | `:goto` / `:view` / `:search` argument parsing + search cycling | internal/ui | input-edge | **never** (one pass-10 canary hole) |
| 6 | Day agenda + time-grid layout — `DayAgenda`, `LayoutDay`, `layoutEnd` | internal/model | input-edge | **never** (two escaped canaries, 14/17) |

The decode side of recurrence (#1) was the standout gap: its *write*-side twin (`ReanchoredRecurrence`)
has been audited five times, while the function that turns an arbitrary foreign-server RRULE into the
editable vocabulary had zero coverage — and a wrong `ok=true` there means the next save rewrites the
user's recurrence into a different rule.

---

## Results — 15 CONFIRMED findings (6 HIGH / 6 MED / 3 LOW)

All 15 carry a repro test that was **written, run RED, and left in the tree**. This synthesis
independently re-ran every one of them against `ba274c1`: **15/15 files exist, 15/15 fail.** No finding is
counted on assertion alone.

**Consequence: `go test ./...` / `make check` is currently RED.** The repros must land in the same commits
as their fixes, or be gated/deleted if a fix is deferred.

### HIGH

| # | Finding | File | Repro (verified RED) |
|---|---|---|---|
| H1 | `decodeYearly` accepts `FREQ=YEARLY;BYMONTHDAY=<n>` without BYMONTH as plain yearly — it actually fires 12×/year, and a grab day-move makes the event vanish | `internal/model/recurdecompose.go:138` | `internal/model/yearly_bymonthday_repro_test.go` |
| H2 | A **failed** "Keep local" still resolves the conflict in memory — server version discarded, next sync overwrites the server | `internal/store/conflict.go:124` (reached from `internal/ui/conflicts.go:69`) | `internal/store/resolvefail_repro_test.go` |
| H3 | A sidecar that fails to parse silently discards **all** sync metadata, then overwrites the corrupt file | `internal/store/sidecar.go:104` / `store.go:158-162` | `internal/store/sidecar_corrupt_repro_test.go` |
| H4 | Tombstone map keys are unvalidated file names and reach `filepath.Join(root, calID, name)` — a sidecar can make sync **write outside the cache root** | `internal/store/sidecar.go:48` → `sync.go:703` / `sync.go:595` | `internal/sync/tombstone_traversal_repro_test.go` |
| H5 | `allowedChildren` covers only **known** containers — a phantom under `X-*`/`VAVAILABILITY`/nested `VCALENDAR` still bricks the whole resource (**4th reopening** of the heal-set class) | `internal/model/decode.go:234` | `internal/model/unknown_container_repro_test.go` |
| H6 | `LayoutDay`'s lane packing is O(n²) — a *bounded* recurrence expansion still freezes the UI on every draw and keypress | `internal/model/timegrid.go:64` | `internal/model/layoutday_scale_repro_test.go` |

### MED

| # | Finding | File | Repro (verified RED) |
|---|---|---|---|
| M1 | A negative `INTERVAL` is silently dropped, so an unbuildable rule is declared representable and a grab move rewrites it into a real infinite series | `internal/model/recurdecompose.go:49` | `internal/model/negative_interval_repro_test.go` |
| M2 | Conflict stash is not byte-lossless — JSON serialization rewrites non-UTF-8 server data to U+FFFD | `internal/store/sidecar.go:72` | `internal/store/conflict_bytelossless_repro_test.go` |
| M3 | The pass-21 aggregate `StepBudget` is minted **per store call**, not per redraw — per-day UI loops multiply it back into a multi-second (up to ~2 min) UI-thread freeze | `internal/store/store.go:375` (+ `render.go:189`, `selection.go:281`) | `internal/ui/redraw_budget_repro_test.go` |
| M4 | The pass-22 "Ends on date includes the end day for timed items" fix **never fires on the real UI path** — the form hands `readCustomRecur` a midnight, date-only anchor | `internal/ui/recurcustom.go:258` (+ `itemforms.go:51/241`) | `internal/ui/repro_endsondate_uipath_test.go` |
| M5 | Zero-length all-day event is dropped from the week/day all-day band but still occupies a slot in the drill list | `internal/ui/render.go:239` | `internal/ui/zerolen_allday_repro_test.go` |
| M6 | More overlap lanes than column cells — event blocks paint into the neighbouring day column and outside the pane's own rect | `internal/ui/timegridview.go:746` | `internal/ui/lanebleed_repro_test.go` |

### LOW

| # | Finding | File | Repro (verified RED) |
|---|---|---|---|
| L1 | Whitespace-only search query slips past both empty-query guards; `n`/`N` then match every row | `internal/ui/search.go:69` | `internal/ui/search_blank_repro_test.go` |
| L2 | `searchNext` keeps a positional index into a recomputed match list, so `n` stalls (or skips) after the collection changes | `internal/ui/search.go:97` | `internal/ui/search_staleidx_repro_test.go` |
| L3 | User-typed query/command text is concatenated **unescaped** into the dynamic-color status bar | `internal/ui/search.go:81` (+ `command.go:73/86/267/279`) | `internal/ui/statusbar_tagescape_repro_test.go` |

---

## Finding detail (the ones that carry a class, not just a bug)

### H1 — `FREQ=YEARLY;BYMONTHDAY=15` is not yearly

Per RFC 5545 and rrule-go's `buildRRule` (vendor `rrule.go:181-193`), a YEARLY rule carrying BYMONTHDAY
with **no** BYMONTH does *not* default BYMONTH to DTSTART's month: the yearly iterator walks all twelve
months and filters by day. `decodeYearly` checks BYMONTH and BYMONTHDAY independently against the anchor,
sees the day matches, and declares the rule representable — collapsing it to a bare
`RecurSpec{Freq: FreqYearly}`.

Observed in the repro (`DTSTART:20260715T090000Z`):

- 12 occurrences over one year (2026-07-15, 08-15, … 2027-06-15) while `RecurrenceSummary` reports
  `"Yearly on Jul 15"`.
- `RecurSpec.ROption().RRuleString()` = `"FREQ=YEARLY"` — 11 occurrences/year silently lost on any
  re-serialization (73 → 7 over six years).
- `ReanchoredRecurrence(ev, +1 day)` returns `(nil, false)` — "no day-pinning `BY*`, the anchor move
  carries the day" — so `internal/ui/grab.go:303-310` does not block, sets `d.Recur = nil`, rewrites
  DTSTART to 20260716 and leaves `BYMONTHDAY=15`. **July occurrences after the nudge: 0.** The event the
  user moved one day disappeared; next instance is Aug 15.

This is the documented *"never leave DTSTART contradicting its own BY*"* guardrail — reached from the
**decode** side, which that guardrail's five prior audits never covered.

Blast radius for the fix: `TestRecurSpecFromRuleAnchorConsistent`
(`internal/model/recurdecompose_test.go:145`) currently asserts `FREQ=YEARLY;BYMONTHDAY=22` **is**
representable. A correct fix breaks it; that case must move to the rejection table, not be preserved.

### H2 — the store commits a conflict resolution the UI reports as failed

`ResolveKeepLocal` applies its full in-memory mutation at conflict.go:109-123 (adopt the server ETag, set
Dirty, clear Conflicted, clear Href when ServerDeleted, `delete(cs.conflicts, name)`) and *then* returns
`writeSidecar`'s error at 124-126 with **no revert**. Every peer write path in the store reverts on exactly
this failure — `writeResourceLocked` (mutate.go:117-126) via `stageResourceLocked`'s revert closure,
`removeLocked` (mutate.go:335-343) via `revertMutation`; `TestPutSingleFailureUsesCleanError` proves `Put`
rolls back under the identical sabotage.

Observed with the sidecar path clobbered by a directory (standing in for ENOSPC/EACCES/read-only mount):

```
ResolveKeepLocal returned: updating sidecar for "cal1": rename …: file exists
after failed resolve: Conflicts=0  ETag="srv-2"  Conflicted=false  Dirty=true  pending=true
```

The ETag has advanced to the server's, so the next conditional PUT's If-Match **matches** and silently
overwrites the server's diverging version. On the ServerDeleted flavour the Href is cleared too, so the
next sync takes the create path and uploads a duplicate. Meanwhile `internal/ui/conflicts.go:75-78` flashes
"Resolve failed: …" and returns **without** `populateConflicts`/refresh — the stale row stays on screen and
re-selecting it errors `store: no conflict for cal1/e_test.ics`.

Adjacent, same ordering, not asserted by this repro: `MarkConflict` (conflict.go:43-56) also commits the
stash and the Conflicted copy-on-write before `writeSidecar`, with no rollback.

### H3 + H4 + M2 — the sidecar read path

The sidecar is a plain on-disk JSON file in a vdir shared with other tools. Only its *write* atomicity had
ever been audited. Reading it adversarially produced three findings at once:

- **H3, all-or-nothing discard.** `readSidecar`'s `json.Unmarshal` is atomic: one wrong-typed field
  (`"dirty": 1`) throws the entire struct away, and `loadCalendar` substitutes `&sidecar{}`. Observed:
  `Dirty=false`, `ETag=""`, `Href=""`, `Tombstones()=[]`, `ReadOnly=false` — and after the first mutation
  the file on disk is reduced to `{"ctag":…, "resources":{"u1.ics":{"hash":…}}}`, i.e. display_name,
  sync_token, href, read_only **and the tombstone map** are all destroyed, worse than the finding claimed.
  Downstream: a clean, href-less local resource is the codebase's own documented "pull orphan to re-pull",
  so the unsynced edit is overwritten (violating *"Sync never silently overwrites"*), and the lost tombstone
  lets the deleted item resurrect. The crash-window heal (`meta.Hash != h ⇒ dirty`) cannot rescue it: on
  parse failure `sc.Resources` is nil, so `meta.Hash == ""` and the heal is skipped — the repro deliberately
  includes the *correct* content hash to prove this.
- **H4, path traversal.** Resource names are safe only because they come from `os.ReadDir`. A tombstone's
  name is a raw JSON **object key**, carried unvalidated into `store.Tombstone.Name` and reaching
  `filepath.Join(s.root, calID, name)` via two ordinary, non-exotic paths: `pushDelete`'s 412
  (delete-vs-remote-change) → `ResurrectTombstone` → `writeResourceLocked` → `writeFileAtomic`, and
  `handleDeleteForbidden`'s confirmed-read-only 403 → `pullInto(…, t.Name, …)`. Observed: a tombstone keyed
  `"../../../../../victim.txt"` caused sync to overwrite a file **two levels above the data dir** with
  server-supplied iCalendar, and the in-memory index gained a resource named by the traversal path — which
  `writeSidecar` then persists back, making the escape sticky across restarts. Fix in two places: reject or
  `SafeName` non-single-element tombstone keys in `loadCalendar`, **and** a defense-in-depth single-element
  check in `stageResourceLocked`/`writeResourceLocked`.
- **M2, stash not byte-lossless.** `conflictMeta.ServerData` is a Go `string` persisted through
  `json.MarshalIndent`, and encoding/json substitutes U+FFFD for every invalid UTF-8 byte. A Latin-1 `.ics`
  from the server (`SUMMARY:caf\xe9 meeting`) comes back 2 bytes longer with the byte replaced. The
  corruption is **invisible in-process** — `Conflicts()` before a reload returns the original bytes; the
  store must be reopened to see it. `ResolveKeepServer` then writes the mojibake as the only surviving copy
  of the server's diverging content and returns nil. Both `sidecar.go:68-70` and `conflict.go:23-25`
  document this stash as "lossless".

### H5 — the heal-set class reopens a fourth time

The pass-21 fix completed `allowedChildren` for the containers go-ical *knows about*
(VALARM/STANDARD/DAYLIGHT), and the guardrail was written as "`allowedChildren` must carry an entry for
every container `checkComponent` recurses into". **That formulation is unsatisfiable**:
`stripForbiddenChildren` (decode.go:234) prunes only when the container name is *in the map*
(`if allowed, ok := allowedChildren[comp.Name]; ok`), while `encodeComponent` (vendor encoder.go:220/244)
recurses into **every** child at every depth regardless of whether `checkComponent` has a case for it.

So a phantom under a container go-ical has no case for is neither stripped nor reached by the top-level-only
heals. All three shapes confirmed:

```
phantom VEVENT under X-CUSTOM-CONTAINER  → ical: failed to encode "VEVENT": want exactly one "DTSTAMP" property, got 0
phantom VTODO under VAVAILABILITY        → ical: failed to encode "VTODO":  want exactly one "DTSTAMP" property, got 0
empty nested VCALENDAR                   → ical: failed to encode VCALENDAR: calendar is empty
```

In each case `Decode` succeeds and the valid sibling VEVENT **is** surfaced (the `len(p.Events)==0` guard
never tripped), so the user sees a normal item — which is then permanently uneditable, uncompletable,
un-grabbable and unpushable, because `store.writeResource` encodes before writing (mutate.go:145) and sync
push encodes at sync.go:609/637. The phantom is not addressable in the UI (Parse walks only
`cal.Children`), so the brick is invisible.

**Guardrail correction required (CLAUDE.md):** the strip must be **deny-by-default** — a container with no
`allowedChildren` entry has its children stripped — rather than an enumeration that can always be one
container behind. RFC 9073 (`PARTICIPANT`, `VLOCATION`) and `VAVAILABILITY` are concrete near-term
additions that would otherwise reopen this a fifth time.

### M3 + M4 — two recent fixes that do not reach production paths

Both are spec-diff catches of the same shape: **the fix is correct, the wiring never delivers it, and the
regression test bypasses the defect.** Worth naming as a pattern for future fix arcs.

- **M3**: the pass-21 aggregate `StepBudget` is minted inside `Store.EventOccurrencesVisible`
  (store.go:375) — i.e. once *per call*. `dayItemsForDays` (render.go:189-193) and `daysRange`
  (selection.go:279, up to `maxSelectDays`=366) each issue **one call per day**, so every day gets a fresh
  2<<20 ceiling. Measured with ~60 far-anchored `FREQ=SECONDLY` events: one budgeted 7-day query 274 ms; the
  7-day week rebuild **1.85 s** (6.8×); a 30-day SELECT materialization **7.8 s**; extrapolated 366-day
  range **~1 m 40 s** — all on the tview event loop, all returning zero occurrences.
  `internal/model/aggregate_cap_test.go` exercises only a single call and so passes.
- **M4**: `readCustomRecur` builds UNTIL from `anchor.Hour()/Minute()/Second()`, which is correct **only if
  the anchor carries the item's wall-clock time**. Both production `anchorFn`s derive it with
  `parseDateField(startDate)` / `parseDateField(dueDate)` (itemforms.go:241, :51) and never consult the
  Start-time / Due-time field, so `anchor.Hour()` is always 0 and UNTIL is still stored at midnight. Driving
  the **real** form wiring (dropdown → `wireRepeatCustom` → modal → OK → `readEventDraft`) yields
  `UNTIL = 2026-07-25 00:00` and occurrences `[07-20 … 07-24]` for a 15:00 daily event ending "on 07-25".
  The pass-22 guard `internal/ui/ends_on_date_test.go` passes a 15:00 anchor the wiring cannot produce, so
  it is green while the reported bug is live. Nothing downstream rescues it —
  `RepeatChoices.Resolve` returns the stored spec verbatim (recurfield.go:114-121).

### H6 + M5 + M6 — the layout surface

`LayoutDay` bounds nothing. The model bounds *expansion* explicitly (`maxOccurrencesPerEvent`=10000,
`maxAggregateOccurrenceSteps`=2<<20) so the UI stays responsive, but the bounded output then goes through a
Θ(n²) lane search (timegrid.go:63-74: for each occurrence, linear scan of `laneEnds`, which grows to peak
concurrency), and `navCells` (timegridview.go:278) is a second O(items × placements) scan on the same data.
It sits on **both** hot paths — inside `Draw` (timegridview.go:663) and on navigation.

Measured: 3 resources of `FREQ=SECONDLY;COUNT=10000` + `DURATION:P1D` → 30,000 same-day occurrences;
expansion 16.6 ms (*the pathological-rule guard reports success*), `LayoutDay` 1.14 s. Isolated scaling:
n=20,000 → 0.57 s, n=100,000 → 9.7 s (5× n ⇒ ~18× time). ~209 such resources pass the aggregate step
budget, which is ~2.09 M occurrences on one day — a redraw that never returns.

The two MEDs are correctness bugs on the same surface: a zero-length all-day event is bucketed onto **no**
day by `splitOccs`'s half-open loop while `DayAgenda` has an explicit zero-length case (so it is in the
month grid and the drill list but invisible in week/day — selectable, editable and deletable with no
on-screen block), and `drawBlock` floors `laneW` at 1 without clamping `bx`/`bw`, so high-lane blocks paint
into the next day's column (events read as being on the wrong day) or past the primitive's rect into the
neighbouring pane.

---

## Mutation canaries — 4 injected, 1 escaped

Full text in `docs/audit/COVERAGE.md` § *Escaped mutation canaries — pass 23*.

- **ESCAPE (OPEN)** — `internal/model/recur_edit.go` `NewSeriesFrom` (~488): `t.Unix() <= occ.Unix()` →
  `t.Unix() < occ.Unix()` left the **whole repo** green. The mutant is a genuine defect: on "edit this and
  future occurrences" applied to an occurrence that already had a per-instance override, the stale override
  at the split instant is carried into the new series and re-keyed to the new UID, where it outranks the
  master's freshly-mutated instance — the user's edit vanishes and the pre-split customization resurrects.
  The code comment states the opposite intent explicitly. Hole:
  `TestSplitCarriesFutureOverride` is the only override-carry-forward test and only asserts the
  strictly-after case. Closure: assert an override exactly **at** `occ` is dropped, and that the split-point
  occurrence expands with the draft's summary.
- **CAUGHT, but only cross-package (surface-local hole, OPEN)** — `internal/store/remote.go` `CommitPush`:
  `if cur == pushed` → `if cur.Name == pushed.Name` (always true, making the concurrent-edit branch dead
  code) left `go test ./internal/store/` **fully green** — all 41 store tests pass. Exactly one test in the
  repo caught it, one package away (`internal/sync` `TestSyncPushDoesNotClobberConcurrentEdit`). The full
  gate has teeth; the *package* suite does not, so an inner-loop `go test ./internal/store/` shows green on a
  silent lost update, and the ledger overstates `internal/store`'s own hardening for the
  "concurrent writes are version-checked" invariant. All three CommitPush tests in the package cover the
  `cur == nil` mid-push-DELETE path; none covers `cur != pushed` mid-push-EDIT.
- **CAUGHT** — `internal/ui/timegridview.go` `navCell.overlaps` half-open → closed:
  `TestTimeGridSpatialDrillNav` failed with a precise message. Rests on a single assertion.
- **CAUGHT** — `internal/model/timegrid.go` `LayoutDay` lane-reuse boundary `!le.After` → `Before`: two
  independent failures including the dedicated `TestLayoutDayTouchingBoundary`. Strong net.

---

## Convergence

| Pass | HIGH | MED | LOW |
|---|---|---|---|
| 22 | 0 | 1 | 0 |
| **23** | **6** | **6** | **3** |

Severity is **sharply up, not trending down.** The honest reading is that this is a *measurement* effect,
not a regression in the code: pass 22 targeted surfaces that had already been hardened three or four times,
while pass 23 deliberately spent five of six slots on surfaces with **zero** prior audits. Five of the six
targets produced findings; the sixth (the deferred spec-diff) produced three, including a fourth reopening
of the heal-set class and two fixes from the immediately preceding passes that never reach production code.

What that says about the ledger: `never`-status rows are not low-yield. The two surfaces the plan flagged as
highest-value-unaudited (RRULE decomposition, sidecar parse) both returned HIGHs immediately, and one
returned an **out-of-cache-root write**.

---

## Residual risk

- **All 15 findings are unfixed and their repros are RED in the tree** — the gate is red until the fix arc
  lands. Nothing in this pass has been remediated.
- **Two classes have now demonstrably survived their own guardrails**: the heal-set class (4th reopening —
  the guardrail's "enumerate every container" formulation is unsatisfiable and needs restating as
  deny-by-default) and the "a fix lands but the production wiring never reaches it, and the regression test
  bypasses the defect" pattern (M3 and M4, from passes 21 and 22 respectively). Neither is closed by finding
  it here.
- **Methods not exercised**: race and fault-injection (last run pass 22). A concurrency or I/O-fault defect
  introduced since then would be missed.
- **The `internal/store` canary hole is open**: the store-side half of the "concurrent writes are
  version-checked" invariant is asserted only from `internal/sync`.
- **Carried forward uncovered**: `internal/caldav` write paths for the bare-write / resource-is-gone class;
  direct iCalendar decode/ingest fuzz (last direct fuzz pass 4 — only the standing seed corpus runs each
  gate); quick-add grammar (19); timezone/DST + Windows→IANA (17); colour parsing (17); `BuildTree` (17);
  the pass-11/12-era UI data-loss surfaces (single-item grab, recurrence-edit scope picker, quick-field
  sp/sd, completion toggle + recurring advance, undo stack, bulk-pull batching); the pass-18-era
  multi-account surfaces; UI display stress (14) — including `render.go`'s `calItems`, changed by the
  pass-21 StepBudget work — and mouse handling (18); `internal/ui/colorpicker.go` and `help.go`, which
  still have no ledger row.
- **Permanent gaps unchanged**: Raspberry Pi on real hardware (on-device timing, kiosk/autologin, bare-TTY
  colour) cannot be audited headlessly; full `sync-collection` incremental sync is a deliberate deferral.
- **Scope**: six of ~53 inventoried surfaces, one method each. Everything else is unexamined *this pass*.

## Recommendation

**`more_passes_recommended`** — 6 HIGH and 6 MED confirmed, a canary escaped, a surface-local canary hole
opened, and several high-value surfaces remain `never`/`stale`.

Suggested order for the fix arc (repro-first, one commit per fix, full gate each commit):

1. **H4** (write outside the cache root) — smallest fix, largest blast radius; two guards, both cheap.
2. **H3** (sidecar all-or-nothing discard) — fail-safe instead of fail-clean; quarantine the original bytes.
3. **H2** (unreverted `ResolveKeepLocal`, plus the same ordering in `MarkConflict`) — mirror
   `stageResourceLocked`'s revert closure, and make the UI refresh on error.
4. **H5** (deny-by-default strip) + restate the CLAUDE.md guardrail in the same increment.
5. **H1 + M1** together (`decodeYearly` / non-positive INTERVAL) — one function, and H1's fix must move
   `TestRecurSpecFromRuleAnchorConsistent`'s `BYMONTHDAY=22` case to the rejection table.
6. **H6 + M6 + M5** (layout: min-heap lane assignment, clamp `drawBlock`, zero-length all-day bucket).
7. **M3 + M4** (deliver the pass-21/22 fixes on the real paths) — and add the *wiring* to the regression
   tests, not just the unit.
8. **L1–L3** and the escaped canary's closure test.

Next pass's targets should include: the escaped-canary surface (override partitioning at the split point),
the `internal/store` CommitPush mid-push-EDIT hole, `internal/caldav` write paths for the bare-write class,
and a race/fault-injection sweep over whatever the fix arc touches.

---

## Resolution (2026-07-25)

All 15 confirmed findings were fixed repro-first in the same session as the audit, plus the escaped
canary, two coverage holes, and **four defects the audit did not find** (surfaced by the fix arc itself).
Full gate green; additionally verified across four timezones. 18 commits.

### The 15 findings

| Sev | Finding | Commit | Resolution |
|-----|---------|--------|------------|
| HIGH | Tombstone keys escape the cache root | `0a36ef3` | Mirrors the pass-9 traversal guard via a shared `validPathElement`; `readSidecar` drops unsafe keys at the single load choke point, individually rather than failing the load. |
| HIGH | Corrupt sidecar discards all sync state | `d55c1f3` | Field-by-field salvage + "unknown ≠ empty" (partial entries load Dirty) + quarantine of the original bytes to `.lazyplanner.json.corrupt`. Surfaced via the existing `LoadError` channel. |
| HIGH | A failed resolve still resolves in memory | `daaef8d` | Restores resource + stashed conflict on write failure, per the package's `revertMutation` idiom. `ResolveKeepServer` did not share it but got guards; `MarkConflict` deliberately left (reverting there is the data-losing direction). |
| HIGH | `allowedChildren` allow-by-default (4th reopening) | `0151ebd` | Deny-by-default **restricted to `encoderValidatedComponents`**. Blanket deny was implemented first and rejected: it dropped valid RFC 7953 VAVAILABILITY/AVAILABLE data. |
| HIGH | `YEARLY;BYMONTHDAY` without BYMONTH | `33b3f42` | Decomposer rejects it (→ *Custom rule (kept)*). It fires 12×/year; a grab day-move had made the event vanish. |
| HIGH | `LayoutDay` O(n²) | `1ec474d` `524c6e4` `311139c` | **Three** quadratics, not one: model lane packing → sweep line (~90–405×); `navCells` per-keypress scan → index (~40×); `Draw`'s `inSelRange` → index (15.5×→4.2× growth). Each found while fixing the previous. |
| MED | StepBudget minted per store call | `e236972` | Batched to one range query per redraw. Threading the budget through the per-day loop was rejected — it would have truncated legitimate calendars (~33M steps vs a 2M ceiling). |
| MED | Pass-22 timed-UNTIL never reaches production | `836bc6c` | Both `wireRepeatCustom` anchorFns read only the date field; `recurAnchor` now folds in the time field. |
| MED | Negative `INTERVAL` accepted | `33b3f42` | Rejected. Boundary verified against vendored rrule-go: negative is unbuildable, `0` normalizes to 1. |
| MED | Conflict stash not byte-lossless | `b6a105b` | Base64 (`server_data_b64`) with read-only legacy-field migration. |
| MED | Lane bleed past column/pane | `a669f19` | Geometry clamped to column and pane rect; overflow collapses proportionally rather than dropping events. |
| MED | Zero-length all-day dropped from band | `d1750ce` | `splitOccs`'s hand-rolled day walk replaced by the shared `OverlapsDay` predicate. |
| LOW ×3 | Blank search · stale `searchIdx` · unescaped status text | `517eacc` | One `blankQuery` decision point; `n`/`N` re-derive position by UID; `tview.Escape` at the boundary. |

### Test-net work

| Item | Commit | Resolution |
|------|--------|------------|
| Escaped canary — `NewSeriesFrom` split boundary | `96179a2` | All three positions pinned (before / **at** / after); split-point occurrence must carry the draft's summary. |
| Hole — `internal/store` blind to a `CommitPush` lost update | `96179a2` | Store-local deterministic + race tests. Soft spot: the race sibling is vacuously green under a fixed launch order. |
| Hole — four hand-mirrored go-ical tables untested | `96179a2` | `encoderdrift_test.go` parses vendored `encoder.go` with go/ast and fails on drift. **This converts the manual re-diff that failed in three of four heal-set reopenings into a gate check.** |
| Tag-escape class — 9 further sites | `bb76bd1` | The audit cited 5; the class spanned `calendar.go`, `edit.go`, `quickfield.go`, `yankpaste.go`. |
| Zone-dependent tests | `bb76bd1` | Two tests failed east of UTC at HEAD; CI's UTC hid them. |

### Four defects the audit did NOT find

1. **All-day "Ends on date D" dropped D** (`e079e50`) — **two** bugs whose signs flip with the UTC offset: a DATE `UNTIL` read as UTC midnight, and `dateOnlyUntil` truncating the *UTC* render (storing the wrong day in the `.ics`). Two further sites in the family; fixing only the read side would have made a split emit D **twice**. The recurring-todo twin reported its series exhausted and marked itself done. **This falsifies the pass-22 close-out**, which recorded all-day as correct.
2. **`navCells` quadratic** (`524c6e4`) and 3. **`inSelRange` quadratic** (`311139c`) — the reported `LayoutDay` HIGH was one of three.
4. **Two zone-dependent tests** (`bb76bd1`) — green in CI, red for any developer east of UTC.

### Divergences from the audit's stated fix direction

- **Heal-set:** narrowed to encoder-validated types rather than blanket deny-by-default. `checkComponent` has **no default case**, so a type it lacks a case for cannot fail encoding — stripping it destroys valid data for nothing. Premise re-verified against `encodeProp`'s other failure paths (param double-quote rejected at decode; CR/LF healed by `sanitizePropValues`).
- **StepBudget:** batched rather than threaded, to avoid starving legitimate calendars.
- **`MarkConflict`:** deliberately not "fixed" — reverting there discards the server's stashed version. Self-heals via the next 412.

### Guardrails codified (PROTOCOL rule 9)

Heal-set strip rewritten to deny-by-default-restricted (the old "enumerate every container" wording was **unsatisfiable**, which is *why* the class reopened four times) · Scale invariants now four hot paths, plus "bounding what a stage produces does not bound what the next stage does with it" · **a UI-reachable bug's regression test must drive the real entry point** (confirmed twice consecutively: pass-21 StepBudget, pass-22 UNTIL) · escape uncontrolled text at the call site · build test times in the code's zone.

### Carried residual — Pass 24 targets

1. **UNVERIFIED PRODUCT-BUG LEAD (highest value).** The zone agent, killed mid-investigation by a session limit, reported it had "confirmed a genuine product bug" reproducing under `TZ=UTC` and was about to check the todo form. Not reproduced or located; its committed edits are a coherent *test*-bug fix and the suite is green in four zones. Either a separate defect exists (likely in a create form) or the test edits mask something. **Not a closed item.**
2. `internal/caldav` write paths — unswept for the bare-write class for a **second** consecutive pass.
3. Race and fault-injection not exercised at all this pass.
4. `go test -race ./internal/model/` fails on `TestAggregateRecurrenceCapBounded` — a pass-21 **absolute wall-clock** budget that `-race` overhead exceeds. Pre-existing (fails at `ba274c1`); convert to growth-ratio style.
5. A full refresh still mints ~4 budgets (bounded constant, no longer day-scaled); write-side `safeAfter` has no aggregate budget (bulk grab = N × 1M steps).
6. `internal/store` decodes with hard-coded `time.Local`; `allowedChildren` is not covered by the drift tripwire (go-ical's nesting rules aren't an extractable table); `internal/ui/conflicts.go` still does not refresh on a failed resolve.
7. Decomposer asymmetry: `FREQ=WEEKLY;BYDAY=TU,TH` with a Monday anchor is accepted though the anchor is outside its own set — monthly/yearly reject the equivalent.
8. **Owner decision outstanding:** a calendar with an unparseable sidecar becomes temporarily **read-only in the UI** until the next sync. Protects against editing a genuinely read-only calendar offline, but blocks offline edits in an offline-first app. One line to revert.

### Honest note on scope

Nine of the items above were **not** in the audit's finding list. The arc expanded past its brief because
each verification surfaced more, and it was stopped deliberately rather than on exhaustion: from the
`inSelRange` fix onward, further discoveries were **recorded here rather than fixed**. The finding list a
pass produces is a floor, not a ceiling.
