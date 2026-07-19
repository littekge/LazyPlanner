# Pass 17 — Import empty-href silent data-loss + IANA-TZID VALUE=PERIOD RDATE mis-zoned to floating; first audits of color.go and windowszones.go

- **Date:** 2026-07-18
- **Prior pass:** Pass 16 (unhealed encoder classes brick the resource + CLI subcommand flag ergonomics + `:config`-reload dropped warning + double-click wrong-row) — HIGH 2 · MED 2 · LOW 2 (all fixed)
- **This pass:** HIGH 0 · MED 2 · LOW 0 (both confirmed with runnable failing-test repros executed and observed red)
- **Status (2026-07-18): ALL RESOLVED.** Both MED fixed repro-first and all four canary holes
  closed, one full-gate commit each (see `log.md`); the body below is kept as the as-found evidence.
  Fixes: tz `resolveDateTimeValues` drops the stale VALUE=PERIOD param + `resolveDateTime` gained an
  IANA-TZID `LoadLocation` recovery branch (`rdate_period_tzid_test.go`); the Import loop mirrors
  `reconcileCalendar`'s empty-href skip (`import_emptyhref_test.go`). Canary guards:
  `TestReadOnlyDegradedDownloadKeptVsDeleted`, `TestSplitAtSeriesEndKeepsFutureBounded`,
  `TestDayAgendaExcludesTodoDueAtDayEnd`, `TestLoadPartialParseThenErrorIsZero` — each verified to
  fail under its exact mutation. Two corrections to the as-found notes surfaced at fix time: the
  COUNT-clamp boundary triggers at occ *past* the last occurrence (not at it), and the `state.Load`
  canary needed a later-field type mismatch (the suggested trailing-garbage repro is rejected by
  `checkValid` before decode and would itself have escaped).

This pass took the ledger's stalest / never-audited cells the plan named: the **Import ingest path**
(marked recent but only ever encode-fuzzed at pass 15 — its fault paths untested), a whole-app
**feature-promise spec-diff** (stale since pass 13, three passes of new promises unverified), the
never-in-ledger **color parser** (`model/color.go`), the never-directly-audited **Windows→IANA zone
table** (`windowszones.go`) + TZID fallback, the oldest headless surface **BuildTree** (stale since
pass 5), and an adversarial re-fuzz of the **state-file parser** (stale since pass 12).

The two findings are both MED and both a *missing-guard-that-a-sibling-path-already-has* shape: the
Import loop lacks `reconcileCalendar`'s empty-href skip, and `resolveDateTime` has a Windows-name
TZID recovery branch but no IANA-TZID one. The color parser, BuildTree re-fuzz, and Windows-zone
mapping produced no finding (the two never-audited parsers held up). The heavier news is the
canary sweep: **all four canaries escaped**, exposing four live test-coverage holes — including an
upper-bound twin of a pass-14 escape and a read-only twin of an already-covered sync guard.

---

## Coverage exercised

| Surface | Package | Method | Result |
|---|---|---|---|
| Import ingest path (Import loop: downloadResilient / PullRemoteBatch / SetCalendarMeta) | internal/sync | fault-injection | 1 MED (empty-href / basename collision silently overwrites, reports success) |
| Feature-promise conformance vs main.md/CLAUDE.md | (whole app) | spec-diff | no *new* drift (passes 14/15/16 promises hold); the tz VALUE=PERIOD gap is recorded as the tz finding, not separate |
| Color parsing (ParseHexColor / NearestANSI16 / ReadableFg / Luminance) | internal/model | input-edge | no finding (malformed length / non-hex / out-of-range / empty input rejected/clamped, no panic/OOB) |
| Windows→IANA zone mapping + TZID resolution | internal/model | fuzz | no crash/panic on adversarial/unresolvable TZIDs; **1 MED functional gap** on the resolveDateTime path (IANA-TZID VALUE=PERIOD RDATE mis-zoned to floating) |
| Subtask tree build (BuildTree cycle/orphan/deep-chain) | internal/model | fuzz | no finding (classification robust across 12 passes of model evolution); 1 canary ESCAPE on the recur-split COUNT clamp |
| State-file load/parse (widths / hidden-cals / hour-zoom adversarial values) | internal/state | fuzz | no parse finding; 1 canary ESCAPE (Load drops the json.Unmarshal error check untested) |

---

## Confirmed findings (each carries a failing-test repro that was run and observed red)

### MED

1. **Import lacks the empty-href guard `reconcileCalendar` has — a malformed server response
   silently drops/collides objects while reporting success** — `internal/sync/import.go:93`. The
   Import object loop (import.go:92-100) writes every downloaded object without checking for an
   empty resource `Path`, unlike the sibling sync path `reconcileCalendar` (sync.go:439-445), which
   explicitly skips empty-href objects via `errEmptyHref`. A malformed/hostile CalDAV server can
   return REPORT (or GET) responses with empty `<href/>` elements — go-webdav's `Response.Path()`
   returns `("", nil)` with no error for a 200 propstat whose single href is empty — so
   `DownloadAll` yields `caldav.Object`s with `Path==""`. Import feeds these to
   `resourceFileName("")`, which produces the placeholder name `resource.ics`, and stores them with
   `Href==""`. Multiple empty-href objects therefore all collide on the single name `resource.ics`
   in `PullRemoteBatch` and silently overwrite each other (each is clean/`Dirty==false`, so no
   `ErrKeptLocalEdit` fires), yet each overwrite is counted as a successful pull. The same collision
   applies to any two distinct server hrefs that share a basename (`resourceFileName` uses
   `path.Base`), though empty-href is the concretely reachable trigger. The lost object is never
   recovered: on the next sync, `reconcileCalendar` skips the empty-href server object and leaves
   the local `resource.ics` as an inert href-less pull-orphan, so it is permanently absent while the
   import claimed success.
   Repro: `internal/sync/import_emptyhref_repro_test.go`
   (`TestImportEmptyHrefCollisionRepro`), ran red — two empty-href objects imported;
   `res.Objects==2` but only 1 resource stored, 0 skips recorded (test log:
   `res.Objects=2 stored=1 skipped=0`). One imported object silently lost while Import returned
   success.
   Fix direction: mirror `reconcileCalendar`'s guard (sync.go:439-446) — skip `obj.Path==""` in the
   import loop and record it in `res.Skipped` with `errEmptyHref` instead of adding it to pulls.

2. **`RDATE;VALUE=PERIOD` carrying an IANA TZID is silently mis-zoned to floating time** —
   `internal/model/tz.go:68`. `resolveDateTimeValues` sets the sub-prop's `Value` to the period
   start (`periodStart(part)`) but leaves the `VALUE=PERIOD` param on the sub-prop, so go-ical's
   `prop.DateTime` rejects it (its switch has no `ValuePeriod` case — `vendor/.../go-ical/ical.go`).
   `resolveDateTime` then has no IANA-TZID recovery branch: it maps only Windows zone names via
   `windowsToIANA` (which returns `""` for an IANA name like `America/New_York`) and never calls
   `time.LoadLocation` on an IANA TZID directly, so an `RDATE;VALUE=PERIOD` with a real IANA TZID
   falls through to the last-resort floating fallback and is interpreted in the calendar's fallback
   `loc` instead of its TZID — a wrong absolute occurrence time, silently. A Windows-name TZID on
   the same PERIOD value hits the `windowsToIANA`→`LoadLocation` branch and resolves *correctly*, so
   the two paths disagree and the standard IANA case is the broken one.
   Repro: `internal/model/rdate_period_tzid_repro_test.go` (`TestRDatePeriodIANATZIDRepro`,
   left in place), ran red — an RDATE prop `{Value:"20260101T100000/PT1H",
   Params: VALUE=PERIOD, TZID=America/New_York}` resolved via `resolveDateTimeValues(prop, time.UTC)`
   gave `got = 2026-01-01T10:00:00Z` (floating/UTC) instead of `want = 2026-01-01T15:00:00Z`
   (10:00 EST) — a 5-hour error. Swapping TZID to `Eastern Standard Time` (Windows) yields the
   correct 15:00Z. go1.26.
   Fix direction: strip the `VALUE=PERIOD` param on the period-start sub-prop before re-parse, and
   add an IANA-TZID `time.LoadLocation` recovery branch to `resolveDateTime` (parallel to the
   Windows-name branch) so the standard case is zoned like the Windows one.

---

## Surfaces swept with no finding

- **Color parsing** (`internal/model/color.go`, input-edge, first audit) — `ParseHexColor` /
  `NearestANSI16` / `ReadableFg` / `Luminance` parse untrusted server-supplied CALENDAR-COLOR / hex
  strings. Boundary-swept malformed length, non-hex digits, out-of-range channels, and empty input
  to `NearestANSI16` — no panic / out-of-bounds / mis-parse; the parser rejects or clamps as
  expected. Distinct cell from the pass-12 caldav color-PROPFIND decode.
- **Windows→IANA zone table + TZID fallback** (`windowszones.go` / `tz.go`, fuzz, first direct
  audit) — no crash/panic on adversarial or unresolvable TZIDs; the floating fallback holds. The one
  functional gap on this path (IANA-TZID VALUE=PERIOD) is recorded as finding 2.
- **BuildTree cycle/orphan/deep-chain classification** (`internal/model`, fuzz, stalest headless
  surface, since pass 5) — re-fuzzed against 12 passes of model evolution; the parent-chain
  memoized cycle classification remains robust, no finding.
- **State-file parse** (`internal/state`, adversarial-value fuzz, since pass 12) — negative/overflow
  widths, unknown-calendar hidden entries after the later calendar-id changes: no parse finding. (A
  *test-net* hole in `Load`'s error handling surfaced as a canary escape — see below.)
- **Whole-app spec-diff** — the passes 14/15/16 promises (method-aware redirect policy,
  heal-set-mirrors-validateComponent, RDATE/EXDATE multi-value independence) verified against the
  implementing code: the redirect and reconcile promises hold; the RDATE/EXDATE comma-list
  independence holds; the VALUE=PERIOD-IANA-TZID mis-zone is a live gap in the same resolveDateTime
  path, recorded as finding 2 rather than a separate spec-drift entry.

---

## Mutation-canary results — 4 of 4 escaped (all OPEN test-coverage holes)

Canaries probe the *test net*; an escape means the code is correct today but a plausible future
regression on that exact path would ship silently. None were fixed this pass.

- **ESCAPE** — `internal/sync/sync.go` `reconcileReadOnly` (~line 514): inverting the degraded-
  download guard `case !onServer && unfetched[r.Href]:` → `!unfetched[r.Href]` —
  `go test ./internal/sync/` PASSED (`ok ... 0.149s`). The read-*write* twin of this guard IS
  covered (`TestDegradedDownloadNotTreatedAsDeletion` /
  `TestDegradedDownloadDirtyResourceNotFalselyConflicted` / `...CTag...` in
  `degraded_download_deletion_test.go`), but the read-only path's equivalent guard has no test
  combining a read-only calendar with a degraded/partial download. The only read-only test
  (`TestSyncReadOnlyDiscardsStuckAndMirrors`) uses a dirty-stuck resource (Discard branch) and a
  new-on-server resource (pull branch); it never exercises a previously-synced clean read-only
  resource that is either server-deleted or unfetched, so both sides of the inverted case go
  unobserved. A regression would false-delete a still-present read-only resource whose GET merely
  failed (`unfetched==true`) or leak a genuine server deletion (`unfetched==false`).
  Suggested guard: a read-only + degraded-download test asserting an unfetched resource is kept and
  a truly server-deleted clean one is Forgotten.
- **ESCAPE** — `internal/model/recur_edit.go` `NewSeriesFrom` (this-&-future split): weakening the
  future-series COUNT clamp `if remaining < 1 { remaining = 1 }` → `< 0` — `go test
  ./internal/model/` PASSED (`ok, 0.021s`; `-run 'Split|Series|Recur|Count'` also PASSED). When the
  split point lands at/after the last occurrence, `pastCount == roption.Count` so `remaining`
  computes to 0; the original clamp forced it to 1, but `< 0` lets 0 through, and rrule-go treats
  `COUNT=0` as *unbounded* — the future series recurs forever, breaking the "two halves sum to the
  original count" invariant. The split tests (`recur_split_exdate_test.go` /
  `recur_split_rdate_test.go`) guard COUNT preservation for pre-split EXDATE/RDATE cases but none
  covers the `remaining==0` boundary (split at/after the final occurrence).
  Suggested guard: split a COUNT-bounded series at its last occurrence and assert the future half
  yields exactly one occurrence, not infinite.
- **ESCAPE** — `internal/model/agenda.go` `DayAgenda` (todo due-window *upper* bound): flipping
  `t.Due.Before(dayEnd)` → `!t.Due.After(dayEnd)` (exclusive → inclusive) — `go test
  ./internal/model/` PASSED (`ok, 0.052s`; `-run Agenda -v` shows `TestDayAgenda` and
  `TestDayAgendaIncludesTodoDueAtMidnight` both PASS). This is the *upper*-bound twin of the pass-14
  escape (which pinned the inclusive *lower* bound). A todo due exactly at `dayEnd` (00:00 of the
  next day — the natural due time for a date-only/all-day todo) would leak onto both this day's and
  the next day's agenda (a duplicate). `TestDayAgenda`'s only excluded todo sits at `dayEnd+1h`, so
  it stays excluded under both bounds and the boundary itself is never asserted.
  Suggested guard: add a todo due exactly at `dayEnd` and assert it is *excluded* from the current
  day.
- **ESCAPE** — `internal/state/state.go` `Load()` (~line 52): dropping the `json.Unmarshal` error
  check (`if err := json.Unmarshal(...); err != nil { return State{} }` → ignore err) —
  `go test ./internal/state/` PASSED (all 4 tests). The only malformed-input test
  (`TestLoadBadFileIsZero`) feeds `"{ not json"`, which fails Unmarshal *before* it mutates the
  destination struct, so `s` stays zero whether or not the error is checked — the zero-State
  assertion holds identically for the buggy code (`TestLoadCapsEndlessFile`'s 4 MB likewise fails to
  parse and leaves `s` zero). No test exercises a file that *partially* parses then errors (e.g.
  `{"left_width":5} trailing`), where the dropped check would surface a populated State that should
  have been rejected as zero.
  Suggested guard: feed a partially-valid-then-trailing-garbage file and assert `Load` returns a
  zero State.

---

## Convergence

| Severity | Pass 16 | Pass 17 | Trend |
|---|---|---|---|
| HIGH | 2 | 0 | ↓ |
| MED  | 2 | 2 | → |
| LOW  | 2 | 0 | ↓ |
| **Total** | **6** | **2** | ↓ |

Severity is **trending down**: HIGH fell 2→0 (the first HIGH-free pass in the recent streak) and
total fell 6→2. That is genuine progress on the *findings* axis. But the pass is **not converged**:
two MED remain (both unfixed, both a missing-guard-that-a-sibling-has shape), and — the more
telling signal — **all four mutation canaries escaped**, the worst canary result in the recent
record. Two of those escapes are *twins of already-known patterns*: the `DayAgenda` upper-bound
escape mirrors the pass-14 lower-bound escape (the same function, the opposite boundary, still
untested), and the `reconcileReadOnly` degraded-download escape mirrors a guard that IS covered on
the read-write path. That recurrence says the test net is being extended point-by-point rather than
by boundary-class, so twin holes keep surviving.

---

## Residual risk

- **2 MED confirmed, both UNFIXED, both with red repros.** (1) A malformed/hostile server's
  empty-href (or basename-colliding) objects are silently overwritten and lost during Import while
  it reports success — reachable from a non-conforming CalDAV server, item-level data loss, and
  invisible (the sibling sync path already guards this; Import simply doesn't). (2) An
  `RDATE;VALUE=PERIOD` with an IANA TZID is silently interpreted as floating time — a wrong absolute
  occurrence, and inconsistent with the Windows-name TZID path which is correct; reachable from any
  real `.ics` that uses period RDATEs with IANA zones (Google/Outlook-style).
- **All four canaries escaped — four live test-net holes**, none guarded: read-only degraded-
  download false-deletion/leak (`reconcileReadOnly`), this-&-future split COUNT collapse to
  unbounded (`NewSeriesFrom` `remaining==0`), `DayAgenda` dayEnd inclusive-upper-bound duplicate
  (twin of the pass-14 lower-bound escape), and `state.Load`'s dropped json error check
  (partial-parse-then-error surfaces a populated State). A boolean/comparison regression on any of
  these ships silently today.
- **The sync core's deep concurrency / TOCTOU was deliberately deferred this pass** (last deep audit
  pass 11); the pass-17 targets skewed to model-side parsers and one write path. That is the main
  heavier surface still cooling — flagged, not cleared.
- **Surfaces that produced no finding are warm, not proven correct**: the first audits of
  `color.go` (input-edge) and `windowszones.go`/TZID fallback (fuzz), the BuildTree re-fuzz, and the
  state-parser adversarial re-sweep each found no defect through their lens — absence of a finding
  is not absence of a bug.
- **Repro hygiene:** `internal/model/rdate_period_tzid_repro_test.go` is left in the tree and is
  currently RED — it (and any Import repro added) will break `make check` until the fix lands; keep
  it as the regression guard once finding 2 is fixed. The Import repro
  (`import_emptyhref_repro_test.go`) was written, run red, and described in the finding; reconcile
  its presence before the next gate.
- **Not covered / still deferred:** full `sync-collection` token-delta sync (unbuilt); the Raspberry
  Pi hardware target (needs a physical Pi); the sync-core deep TOCTOU (pass 11). The pass 15/16
  caldav/store/sync/ui cells were deliberately left to cool.

**Recommendation: more passes recommended** — two MED confirmed (both unfixed) and all four canaries
escaped, including twin holes of prior-pass patterns. Fix repro-first: mirror `reconcileCalendar`'s
empty-href skip in the Import loop; strip the `VALUE=PERIOD` param and add an IANA-TZID
`LoadLocation` recovery branch in `resolveDateTime`; and close the four canary holes with boundary
tests (read-only degraded-download, split-at-last-occurrence COUNT, `DayAgenda` dayEnd exclusion,
`state.Load` partial-parse). Consider whether the recurring *twin-boundary* escapes warrant a
"test both boundaries of every window / mirror a guard onto its sibling path" note.
