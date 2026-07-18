# Pass 14 — reconcile 412 tombstone drop + recurrence write-side split defects + multi-valued RDATE/EXDATE collapse + quick-add day-range

- **Date:** 2026-07-18
- **Prior pass:** Pass 13 (spec-diff reopened Locate→Put + reconcile degraded-download + CalDAV request-side idempotency) — HIGH 1 · MED 4 · LOW 0
- **This pass:** HIGH 0 · MED 5 · LOW 1 (all 6 confirmed with runnable failing-test repros executed and observed red)
- **Status (2026-07-18): ALL 6 findings + the escaped canary FIXED** repro-first (one commit
  per fix, full gate every commit); the RDATE/EXDATE root-cause class was codified as a
  Hard-won guardrail in `CLAUDE.md`. The findings below are the point-in-time as-found audit
  evidence — every repro was written, run, and observed to fail against the then-current code;
  see `log.md` (2026-07-18 entries) and `COVERAGE.md` for the fixes and their commits.

This pass took the ledger's top stale/never surfaces head-on: the **sync reconcile state
machine** full local×server case matrix (data-loss) — the target `main.md` names as the top
remaining item, of which pass 13 audited only one branch — plus a **fuzz** of the timezone /
TZID resolver + floating-time fallback (6 passes stale, never fuzzed), an **input-edge** sweep
of the **quick-add parser** semantic correctness (only ever crash-fuzzed), an **input-edge**
sweep of the **non-command key/chord dispatch** and a **display-stress** re-sweep of the
newer **UI draw widgets** (agendaboard/itemforms), and a focused **spec-diff** of the
**recurrence write-side** transforms against `main.md`'s precise, testable promises.

The headline: the recurrence write-side spec-diff found **two** distinct ways the
this-and-future split violates `main.md:362`'s "total occurrence count is unchanged" promise
(a phantom EXDATE-driven trailing occurrence and a duplicated trailing RDATE), and the tz
fuzz found that a perfectly valid RFC-5545 multi-valued RDATE/EXDATE silently collapses an
entire recurring series to its base instance.

---

## Coverage exercised

| Surface | Package | Method | Result |
|---|---|---|---|
| Sync reconcile state machine (full local×server matrix: keep-both, Forget, resurrect, keep-local) | internal/sync, internal/store | data-loss | 1 MED + 1 LOW |
| Timezone / TZID resolver + floating-time fallback | internal/model | fuzz | 1 MED (multi-valued RDATE/EXDATE collapse) |
| Quick-add parser semantic correctness (date/time/priority/tag extraction) | internal/model | input-edge | 1 MED (invalid day-of-month) |
| Recurrence write-side transforms vs main.md promises (split/detach/advance) | internal/model | spec-diff | 2 MED (phantom EXDATE split, duplicated RDATE) |
| Non-command key/chord dispatch (keys.go navigation, grab activation, mode/drill transitions) | internal/ui | input-edge | no finding |
| UI custom Draw paths — newer widgets (agendaboard, itemforms) under hostile content × extreme geometry | internal/ui | display stress | no finding (canary hole below) |

---

## Confirmed findings (each carries a failing-test repro that was run and observed red)

### MED

1. **`pushDelete`'s 412 handler clears the tombstone even when it fails to resurrect/flag the
   conflict — silently resolving a delete-vs-server-change conflict** —
   `internal/sync/sync.go:677`. On a conditional DELETE returning 412 (ErrPreconditionFailed),
   `st.ClearTombstone` runs unconditionally *outside* the nested `serverByHref-ok / parse-ok /
   PutRemote-ok` guard. When the server version is unparseable, or (degraded download) the
   resource's individual GET failed so `serverByHref` lacks `t.Href`, the resurrect +
   `stashServerConflict` block is skipped but the tombstone is still erased. Result: no
   conflict recorded, tombstone gone — violating the "never silently overwrite, always flag"
   invariant. In the parse-fail case no `recordSkip` fires either, so the CTag is cached and
   the next sync's short-circuit permanently swallows the server's change. In the degraded
   case the next full sync re-pulls the item clean, silently un-deleting it with no flag.
   Repro: `internal/sync/repro_tombstone412_test.go`
   (`TestReproTombstone412DegradedDownloadSwallowsConflict`), ran red — after Sync,
   `Conflicts=0` AND `Tombstones()` empty. Control: the existing
   `TestSyncTombstoneVsServerEditIsConflict` (server version present + parseable) PASSES,
   confirming only the degraded/parse-skip path swallows it.

2. **`resolveDateTime` parses only a single date-time, so a comma-listed multi-valued
   RDATE/EXDATE collapses a recurring event to its DTSTART base instance** —
   `internal/model/tz.go:21` (call sites `recurrence.go:200-205`). An RFC-5545-valid
   multi-valued `EXDATE:...,...` / `RDATE:...,...` (single line, comma-separated) or a
   `VALUE=PERIOD` RDATE makes `prop.DateTime` and the floating-time fallback both error on the
   extra text; `recurrenceSet` propagates the error and `Event.Occurrences` swallows it by
   degrading to the lone base instance — silently dropping the entire RRULE expansion. No
   crash, no on-disk data loss (Raw round-trips byte-for-byte), but a valid series is
   mis-expanded. `FuzzEventOccurrences` misses it because it only asserts no-error/no-panic and
   this path degrades to no-error.
   Repro: `internal/model/zzz_repro_multi_exdate_test.go`
   (`TestReproMultiValuedEXDATE`, `TestReproMultiValuedRDATE`), ran red — got 1 instance
   (the base) where 3 are expected in both the EXDATE and RDATE cases.

3. **Invalid day-of-month in slashed and month-name dates is silently normalized to a wrong
   date (and rolled a year forward), inconsistent with the ISO form which rejects it** —
   `internal/model/quickadd.go:277` (month-name path at :198). `parseNumericDate` and
   `parseDate` accept any day 1..31 without validating it against the month, hand it to
   `time.Date` which normalizes out-of-range days into a different month, and
   `rollForwardMonthDay` then pushes the normalized date a year forward. The ISO branch
   (`time.ParseInLocation`) instead rejects such dates, so the same logical input parses one
   way as slashed and another as ISO — and the wrong-date outcome directly violates the
   parser's "when in doubt, leave text in the title rather than guess" principle.
   Repro: `internal/model/quickadd_dayrange_repro_test.go`
   (`TestQuickAddInvalidDayRepro`), ran red — `x 2/30` → HasDate=true, Date=2027-03-02,
   Title="x"; `x feb 30` → 2027-03-02; `x 4/31` → 2027-05-01; `x jun 31` → 2027-07-01; while
   `x 2026-02-30` correctly stays in the title (ISO rejects "day out of range").

4. **This-and-future split adds a phantom trailing occurrence when the master has a pre-split
   EXDATE (app-reachable via delete-this-occurrence)** — `internal/model/recur_edit.go:349`.
   `main.md:362` promises "a bounded COUNT preserved across the split so the total occurrence
   count is unchanged", but `NewSeriesFrom` reduces the future series' COUNT by
   `occurrencesBefore()`, which counts the post-EXDATE *visible* recurrence set rather than the
   RRULE *iterations* the capped master consumes. RFC 5545 COUNT bounds the generator and
   EXDATE'd instances still consume COUNT, so every EXDATE before the split point undercounts
   `pastCount` by one, leaving the future COUNT one too high and appending an occurrence the
   original series never had.
   Repro: `internal/model/zzz_repro_exdate_split_phantom_test.go`
   (`TestReproExdateSplitPhantom`), ran red — FREQ=DAILY;COUNT=5, delete day2 via `AddException`
   (visible=4), split at day4: capped=[day1,day3], future=[day4,day5,day6]; split total 5 vs
   pre-split 4, phantom day6 (2026-07-11).

5. **This-and-future split duplicates a trailing RDATE across both halves (CapSeries caps only
   the RRULE, NewSeriesFrom copies RDATE)** — `internal/model/recur_edit.go:302`. `CapSeries`
   caps the past half by setting only RRULE UNTIL/Count and its cleanup loop removes only
   RECURRENCE-ID overrides after the cut — it never drops RDATEs after the cut, and UNTIL
   bounds only the RRULE generator (confirmed in the vendored rrule `Set.Iterator`, which
   merges `set.rdate` independent of UNTIL). `NewSeriesFrom` then copies every master prop
   except RECURRENCE-ID into the future series, carrying the same RDATE. The one-off RDATE
   instant is emitted by BOTH resources — a duplicated occurrence, one more than the original —
   contradicting `main.md:362` and the iron rule (a single unmodeled property becomes two live
   occurrences).
   Repro: `internal/model/zzz_repro_rdate_split_test.go`
   (`TestSplitDoesNotDuplicateTrailingRDate`), ran red — FREQ=WEEKLY;COUNT=4 + trailing
   RDATE:20260907T090000Z (5 total), split at 3rd instance: the RDATE 2026-09-07 appears twice
   across capped+future; split total 6 vs original 5.

### LOW

6. **Keep-local resolution of a server-deleted conflict never converges — re-raises the same
   conflict every sync instead of re-creating the item on the server** —
   `internal/store/conflict.go:106`. For a server-delete-vs-dirty conflict `markConflict`
   stores `serverETag=""`; `ResolveKeepLocal` adopts that empty ETag but leaves `Href`
   non-empty, so the next reconcile hits `case !onServer && r.Dirty` (`sync.go:404`) and
   re-flags the identical server-deleted conflict rather than reaching the create path
   (`r.Href=="" && r.Dirty`, `sync.go:380`). The kept local version is never pushed back; the
   conflict recurs indefinitely and the item can never be resurrected server-side.
   Repro: `internal/sync/keeplocal_serverdeleted_repro_test.go`
   (`TestKeepLocalServerDeletedConverges`), ran red — first sync flags the conflict,
   `ResolveKeepLocal` clears it, second sync re-raises it (`conflicts==1, want 0`) and issues
   no create.

---

## Mutation-canary results — 1 of 3 escaped (a test-coverage hole)

Canaries probe the *test net*; an escape means the code is correct today but a future
regression on that exact path would ship silently.

- **ESCAPE** — `internal/model/agenda.go` `DayAgenda` (todo due-time lower bound): flipping the
  inclusive lower bound `!t.Due.Before(dayStart)` (Due ≥ dayStart) → `t.Due.After(dayStart)`
  (Due > dayStart) — `go test ./internal/model/` PASSED. Drops any todo due exactly at 00:00
  (the natural due time for an all-day / date-only todo) from that day's agenda.
  `TestDayAgenda`'s todos are due at 09:00 and dayEnd+1h — neither sits on dayStart, so the
  flipped boundary is never exercised. Suggested guard: a `TestDayAgenda` case with a todo
  due exactly at midnight, asserting it appears.
- **CAUGHT** — `internal/ui/grab.go` `grabNudge` (J/K resize min-duration guard ~line 282):
  weakening `!d.End.After(d.Start)` → `d.End.Before(d.Start)` (allows End==Start, a
  zero-duration event) — `go test ./internal/ui/` FAILED, `TestGrabResizeRejectsZeroDuration`.
- **CAUGHT** — `internal/sync/sync.go` `Sync` (CTag-cache guard ~line 186): `len(res.Skipped)
  == skipsBefore` → `>=` (always-cache) — `go test ./internal/sync/` FAILED,
  `TestDegradedDownloadDoesNotCacheCTagSoNextSyncRetries` (the guard added with pass 13's HIGH #1).

---

## Convergence

| Severity | Pass 13 | Pass 14 | Trend |
|---|---|---|---|
| HIGH | 1 | 0 | ↓ |
| MED  | 4 | 5 | ↑ |
| LOW  | 0 | 1 | ↑ |
| **Total** | **5** | **6** | ↑ |

HIGH fell to zero (first pass in a while with no HIGH), but MED rose 4→5, a LOW appeared, and
the total ticked up 5→6. The shape is not reassuring: two of the five MED are *distinct*
correctness defects in the same recurrence-split routine (`recur_edit.go`) against an explicit
`main.md` promise; a third (multi-valued RDATE/EXDATE) is a plain RFC-5545 conformance gap the
fuzz corpus never seeded; and the reconcile matrix — audited yet again — still yielded a
silent-conflict-drop cell. New/stale surfaces keep producing findings on contact.
**Not converged.**

---

## Residual risk

- **6 confirmed findings remain UNFIXED**, all with red repros. None is HIGH, but finding #1
  is a silent conflict drop (delete-vs-server-change swallowed on a degraded download or
  unparseable server version), and #2/#4/#5 are silent *wrong-answer* recurrence bugs: a valid
  multi-valued RDATE/EXDATE collapses a whole series, and the this-and-future split both
  invents a phantom trailing occurrence (EXDATE case) and duplicates a trailing RDATE — each
  breaking `main.md:362`'s explicit occurrence-count promise.
- **The recurrence write-side is the pass's weak spot:** two independent split defects
  (`recur_edit.go` COUNT/EXDATE accounting and CapSeries/NewSeriesFrom RDATE handling). Both
  are reachable from the app (delete-this-occurrence then this-and-future edit; or a foreign
  import carrying an RDATE). The fixes are localized (split each RDATE/EXDATE prop on commas
  before resolving; count RRULE iterations not visible occurrences for the cap; drop RDATEs
  across the cut in both halves) but each needs its own repro-first commit.
- **One escaped canary** (`DayAgenda` inclusive dayStart boundary) is a live test-coverage
  hole — correct today, unguarded; the common all-day-todo-at-midnight case would regress
  silently.
- **Repro hygiene / build note:** the confirmed findings left several proposed regression
  test files in the tree (`internal/sync/repro_tombstone412_test.go`,
  `internal/sync/keeplocal_serverdeleted_repro_test.go`,
  `internal/model/zzz_repro_multi_exdate_test.go`,
  `internal/model/quickadd_dayrange_repro_test.go`,
  `internal/model/zzz_repro_exdate_split_phantom_test.go`,
  `internal/model/zzz_repro_rdate_split_test.go`), all currently RED. One auditor reported an
  earlier `keeplocal_serverdeleted_repro_test.go` leftover that did not compile (undefined
  `caldav`, string conversion of `o.Data`) and broke the `internal/sync` build; reconcile these
  files (keep as guards once fixed, or delete) before the next gate so the package builds. Go
  is not installed on the audit host — auditors downloaded go1.26.4 into scratchpad; the owner
  runs the `go test` lines directly.
- **Not covered this pass / still stale-or-never:** CalDAV network response-parse
  fault-injection (stale since pass 7); store write-pipeline atomicity under disk fault (pass
  9/10); `:config`/$EDITOR reload and mouse handling (pass 10); full `sync-collection`
  token-delta sync (deliberately unbuilt); the Raspberry Pi hardware target (needs a physical
  Pi). The reconcile matrix has now been probed across passes 13/14 but not every cell is
  exhaustively enumerated.

**Recommendation: more passes recommended** — five MED and one LOW confirmed (all unfixed),
one escaped canary, and the recurrence write-side surface yielding two distinct correctness
defects against an explicit spec promise. Fix repro-first: split multi-valued RDATE/EXDATE
before resolving; make `pushDelete`'s 412 branch keep the tombstone (and/or `recordSkip`)
unless the conflict is actually resurrected+flagged; count RRULE iterations for the split cap
and drop RDATEs across the cut in both halves; validate quick-add day-of-month against the
month; make `ResolveKeepLocal` clear the Href (or otherwise route to the server re-create
path); and add the `DayAgenda` midnight-boundary guard.
