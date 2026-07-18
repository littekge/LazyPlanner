# Pass 16 — unhealed encoder classes (VTIMEZONE / VJOURNAL / VFREEBUSY) brick the resource + CLI subcommand flag ergonomics + `:config`-reload dropped warning + double-click wrong-row

- **Date:** 2026-07-18
- **Prior pass:** Pass 15 (CalDAV write-redirect silent success + load-time stale-temp sweep eats real resources + import UID-less-sibling drop) — HIGH 2 · MED 1 · LOW 0
- **This pass:** HIGH 2 · MED 2 · LOW 2 (all 6 confirmed with runnable failing-test repros executed and observed red)
- **Status (2026-07-18): findings are AS-FOUND and UNFIXED** — this is the synthesis/evidence
  report, not a verdict or a fix pass. Each finding below carries a red repro; disposition
  (fix repro-first, or accept as residual) is the owner's next step.

This pass took the ledger's top stale/single-method cells the plan named: **mouse handling**
(input-edge, stale since pass 10), the **`:config`/$EDITOR reload** fault paths (fault-injection,
stale since pass 10), the **CLI subcommand flag dispatch** (input-edge, method-monoculture since
pass 9), the **CTag incremental short-circuit** (fault-injection, single-method since pass 11),
the **background sync goroutines** (race, unre-swept since the pass 14–15 write-path changes), and
a fresh **go-ical encoder round-trip fuzz** (stale since pass 10).

The headline: the fresh encoder round-trip fuzz surfaced **two more HIGH of the exact
decode-but-unencodable class** that pass 10 was supposed to have closed — the pass-10 healers
cover VEVENT/VTODO and empty/nested VTIMEZONE, but a **VTIMEZONE missing its required properties**
and a **VJOURNAL/VFREEBUSY missing DTSTAMP/UID (or with duplicate single-valued props)** are
unhealed, so the whole resource — including every valid sibling component — becomes unwritable at
Encode() time. VTIMEZONEs are ubiquitous in real Google/Outlook `.ics`, so this is plausible in
the field.

---

## Coverage exercised

| Surface | Package | Method | Result |
|---|---|---|---|
| go-ical semantic encoder constraints (re-encode round-trip) | internal/model | fuzz | 2 HIGH (VTIMEZONE-required-props + VJOURNAL/VFREEBUSY unhealed → Encode bricks the resource) |
| `:config` reload / $EDITOR suspend-and-reload flow | internal/ui, internal/config | fault-injection | 1 MED (config.Load warning discarded on reload); 1 canary ESCAPE |
| CLI wiring / subcommand arg+flag dispatch | cmd/lazyplanner | input-edge | 1 MED (`-h` exits non-zero + spurious error) + 1 LOW (bad flag printed twice); 1 canary ESCAPE |
| Mouse handling (SetMouseCapture, click routing) | internal/ui | input-edge | 1 LOW (double-click edits the previously-selected row) |
| CTag incremental short-circuit (skip DownloadAll decision) | internal/sync | fault-injection | no finding (skip is fail-safe under a stale/absent/lying CTag) |
| Background sync goroutines (startPeriodicSync timer + flushOnQuit) | internal/ui, internal/sync | race | no race/deadlock under -race against the pass 14–15 write paths |

---

## Confirmed findings (each carries a failing-test repro that was run and observed red)

### HIGH

1. **Malformed VTIMEZONE (missing TZID, or a STANDARD/DAYLIGHT sub-component missing
   DTSTART / TZOFFSETTO / TZOFFSETFROM) decodes but cannot re-encode, bricking the whole
   resource** — `internal/model/decode.go:224`. The ingest healers cover empty and
   illegally-nested VTIMEZONEs (`dropEmptyTimezones`, `stripForbiddenNesting`) but not a
   VTIMEZONE missing its required `TZID`, nor a STANDARD/DAYLIGHT block missing a go-ical
   exactly-one property. Such an object decodes cleanly and is retained verbatim in
   `Calendar.Children`, but go-ical's `checkComponent` (`exactlyOneProps` for
   CompTimezone/CompTimezoneStandard/CompTimezoneDaylight) rejects it at encode time, so
   `Parsed.Encode()` fails for the **entire** resource — including every valid sibling
   VEVENT/VTODO. This is the identical bug class already treated as HIGH for empty VTIMEZONEs /
   DTEND+DURATION / missing VERSION+PRODID; VTIMEZONEs are ubiquitous in real `.ics`, and the
   natural heal is to drop the un-encodable VTIMEZONE like an empty one (zones resolve from the
   embedded tz db). The write path (`store/mutate.go`, `sync/sync.go`, `model/edit.go`) all call
   `Encode()`, so any edit to a sibling event becomes unsavable.
   Repro: `internal/model/malformedvtimezone_repro_test.go`
   (`TestMalformedVTimezoneBlocksEncode`, table-driven, three subcases), ran red — all three fail:
   `ical: failed to encode "STANDARD": want exactly one "TZOFFSETFROM" property, got 0` (and the
   DTSTART / VTIMEZONE-TZID variants). go1.26.4.
   Fix direction: heal at the `model.Parse` ingest boundary — drop the un-encodable VTIMEZONE
   while preserving the sibling components (add-only / never-mangle per the iron rule).

2. **VJOURNAL/VFREEBUSY missing DTSTAMP/UID, or carrying duplicate single-valued props, decodes
   but cannot re-encode** — `internal/model/decode.go:91`. `ensureDTStamp` is invoked only for
   CompEvent and CompToDo in Parse's switch, and `singleValuedProps` has no entry for CompJournal
   or CompFreeBusy, so those components' encoder constraints are unhealed. go-ical's
   `checkComponent` requires exactly-one DTSTAMP and UID for VJOURNAL/VFREEBUSY and at-most-one
   for their descriptive props. The app clearly expects VJOURNALs to coexist and stay
   round-trippable (`stripForbiddenNesting` has explicit CompJournal/CompFreeBusy allow-sets and
   `TestHealVJournalNesting` guards keeping a sibling VEVENT encodable), yet a VJOURNAL that merely
   lacks DTSTAMP or has a duplicate SUMMARY re-breaks exactly what that heal was meant to protect —
   the whole resource fails `Encode()`, blocking the write.
   Repro: `internal/model/repro_vjournal_test.go`
   (`TestReproVJournalReencode`, `TestReproVJournalDuplicateSummary`), ran red —
   `want exactly one "DTSTAMP" property, got 0` and `want at most one "SUMMARY" property, got 2`.
   go1.26.4.
   Fix direction: extend `ensureDTStamp` and `singleValuedProps`/dedup to cover
   CompJournal/CompFreeBusy at ingest.

### MED

3. **`config.Load()` warning is discarded on `:config` reload — permission + typo warnings
   silently lost** — `cmd/lazyplanner/main.go:185`. `editConfigFn`'s reload path reads
   `cfg, _, _, err := config.Load()`, throwing away Load's third return (the warning string).
   Only warnings produced by `buildSyncFn` (password_command / client-construction) reach
   `ConfigReload.Warning`; every warning Load itself computes — a `default_view` typo flagged by
   `appearanceWarnings`, or a world-readable password file flagged by `permissionWarning` — is
   dropped. The user is told "config reloaded" while their setting is silently ignored and (the
   mildly security-relevant case) their password file is now readable by other users with no
   notice. This defeats the documented intent of `appearanceWarnings` ("naming it makes the
   mistake visible instead of silent") for the reload entry point.
   Repro: `cmd/lazyplanner/configreloadwarn_repro_test.go`
   (`TestConfigReloadPreservesLoadWarning`), ran red — `config.Load()` returned a non-empty
   warning covering **both** modes ("...is 0644; it may hold a password — chmod 600 it;
   default_view \"wek\" is unknown; using the default"), but the returned `ui.ConfigReload` had
   `Warning==""`. Skips on Windows (permission warning is Unix-only).
   Fix direction: fold `config.Load`'s warning into `ConfigReload.Warning` on the reload path.

4. **Advertised subcommand `-h`/`--help` exits non-zero and prints a spurious
   "flag: help requested" line** — `cmd/lazyplanner/import.go:24` (same pattern in
   sync/calendar subcommands via `addConnFlags`). The top-level usage tells users to "Run a
   subcommand with -h for its flags", but every subcommand handler returns the raw error from
   `fs.Parse`. `flag.Parse` returns `flag.ErrHelp` for `-h`/`--help`; that error propagates to
   `report()` (main.go:67), which prints `lazyplanner: flag: help requested` to stderr and
   returns exit code 1 — polluting help output with a bogus error line and failing the
   convention that requesting help succeeds.
   Repro: `cmd/lazyplanner/helpflag_repro_test.go` (`TestSubcommandHelpFlagExitsZero`), ran red —
   `run([]string{"import","-h"})` returned 1 (want 0) and emitted the spurious line after the
   correct usage block.
   Fix direction: special-case `flag.ErrHelp` in the subcommand handlers / `report()` — exit 0,
   suppress the error line.

### LOW

5. **Bad/unknown subcommand flags print the error message twice** —
   `cmd/lazyplanner/import.go:24` (all `addConnFlags` subcommands). With
   `flag.ContinueOnError`, `fs.Parse` already prints the parse error + usage to stderr before
   returning; each handler then returns that same error to `report()`, which prints it again
   prefixed with `lazyplanner:`. The user sees the diagnostic duplicated.
   Repro: `cmd/lazyplanner/badflag_repro_test.go` (`TestSubcommandBadFlagPrintsErrorOnce`), ran
   red — `run([]string{"import","-badflag"})` emitted "flag provided but not defined: -badflag"
   twice (`strings.Count == 2`, want 1).
   Fix direction: `fs.SetOutput(io.Discard)` or don't re-report parse errors (mirror the
   `flag.ErrHelp` handling finding 4 needs).

6. **Double-click edits the previously-selected row, not the row under the cursor** —
   `internal/ui/mouse.go:47`. On a `MouseLeftDoubleClick` the capture calls `editSelected()`
   *before* tview processes the current event, so it acts on the selection left by the preceding
   single click; if the two clicks land on different rows (the mouse moved within the double-click
   interval), the edit form opens for the wrong item. The code comment at mouse.go:49-50 assumes
   "the preceding single click has already moved the selection under the cursor", which only holds
   when both clicks share a row. Recoverable: it only opens a form (no silent write); the user can
   cancel.
   Repro: `internal/ui/dblclick_repro_test.go` (`TestDoubleClickEditsRowUnderCursor`), ran red —
   in modeTasks with rows "Alpha"(A) and "Beta"(B), a double-click over row B opens the edit form
   for row A ("Alpha").
   Fix direction: on a double-click, first let the selection move to the row under the cursor
   (or read the clicked coordinate) before calling `editSelected()`.

---

## Mutation-canary results — 2 of 4 escaped (both test-coverage holes on this pass's CLI/config surfaces)

Canaries probe the *test net*; an escape means the code is correct today but a plausible future
regression on that exact path would ship silently.

- **ESCAPE** — `internal/config/config.go` `Server.Configured()` (~line 240): flipping
  `s.URL != "" && s.Username != ""` → `|| ` — `go test ./internal/config/` PASSED
  (incl. `TestLoadOverlaysFile`, `TestAccountIDStableAndDistinct`). The suite only ever calls
  `Configured()` on a fully-populated server, so `true` stays true under `||`; nothing asserts
  `false` for a partial (URL-only / username-only) config. A regression would treat an incomplete
  connection as configured and sync against it. Suggested guard: assert `Configured()==false` for
  URL-only and username-only servers.
- **ESCAPE** — `cmd/lazyplanner/conn.go` `connFlags.client()` (~line 32): flipping the
  credential-required guard `*url=="" || *username=="" || *password==""` → `&& ` —
  `go test ./cmd/lazyplanner/` PASSED (all 4 tests). Those tests exercise only
  `run()`/`printUsage()`/`editorCommand()` in main.go; conn.go/import.go/sync.go/calendar.go have
  **no direct tests**, so the whole credential-validation guard is uncovered. Under the flip a
  user with URL+username but no password slips past and builds a client with empty credentials.
  Suggested guard: a `connFlags.client()` test asserting an error for each partial-credential
  combination.
- **CAUGHT** — `internal/ui/calendarview.go` `drawDayItems` (~line 307): off-by-one
  `if n <= avail` → `if n < avail` — `go test ./internal/ui/` FAILED
  (`TestCalendarViewDrawsMonth`: an exactly-fitting item pushed into a spurious "+N more").
- **CAUGHT** — `internal/sync/sync.go` `reconcileCalendar` (line ~416): inverting the
  both-sides-changed conflict comparison `serverObj.ETag != r.ETag` → `==` —
  `go test ./internal/sync/` FAILED across 7 tests (`TestSyncPushesLocalEdit`,
  `TestSyncConflictKeepsBoth`, `TestSyncPushDoesNotClobberConcurrentEdit`,
  `TestSyncRefetchesOn412`, `TestSyncUnparseableServerConflictNotTreatedAsDeletion`,
  `TestReproPullBatchClobbersConcurrentEditToOrphan`, `TestUndoOfSyncedEditSurvivesNextSync`).

---

## Convergence

| Severity | Pass 15 | Pass 16 | Trend |
|---|---|---|---|
| HIGH | 2 | 2 | → |
| MED  | 1 | 2 | ↑ |
| LOW  | 0 | 2 | ↑ |
| **Total** | **3** | **6** | ↑ |

**Recurring root-cause class — the pass-10 encoder-heal set is component-incomplete.** The two
HIGH this pass are the *same decode-but-unencodable class* pass 10 declared closed for
VEVENT/VTODO + empty/nested VTIMEZONE: the healers simply don't cover VTIMEZONE-required-props or
VJOURNAL/VFREEBUSY. This is a coding-*practice* pattern (heal every component go-ical's encoder
constrains, not just the two we edit) — a candidate for a Hard-won guardrail if a fix pass
confirms it, per the recurring-class rule. Recorded per `PROTOCOL.md`.

HIGH held steady at 2 (still non-zero — the no-HIGH streak stays broken), and MED/LOW both rose;
total climbed 3→6. Against every criterion the pass is **not converged**: HIGH stayed non-zero,
two canaries escaped, and a recurring encoder-heal class re-appeared two passes after it was
declared closed. Severity is **not** trending down.

---

## Residual risk

- **6 confirmed findings remain UNFIXED**, all with red repros. Two are HIGH: a malformed but
  common VTIMEZONE, or a VJOURNAL/VFREEBUSY missing DTSTAMP/UID, decodes on ingest but makes the
  **entire resource** (including valid sibling events) unsavable at Encode() — an edit to a good
  event silently can't be written. Two MED: `:config` reload silently drops config.Load's
  appearance-typo / world-readable-password-file warnings; subcommand `-h`/`--help` exits 1 with a
  spurious error line. Two LOW: a bad subcommand flag is printed twice; a double-click can open the
  edit form for the wrong (previously-selected) row.
- **The internal/model encoder-heal boundary is the pass's weak spot** — the pass-10 fix set is
  incomplete by component, and real-world `.ics` (Google/Outlook VTIMEZONEs; hand-edited VJOURNAL
  notes) hit exactly the uncovered components. The natural fixes are localized (drop the
  un-encodable VTIMEZONE; run ensureDTStamp/dedup over CompJournal/CompFreeBusy) but each needs
  its own repro-first commit; the repros are already in the tree.
- **Two escaped canaries are live test-coverage holes** on the CLI/config surfaces:
  `Server.Configured()` (partial-config never asserted) and `connFlags.client()` (the entire CLI
  connection-flag validation path is untested — conn.go/import.go/sync.go/calendar.go have no
  direct package tests). A boolean-flip regression in either ships silently.
- **No finding on the CTag fault-injection or the background-sync -race sweep** — the CTag skip
  decision is fail-safe under a stale/duplicate/absent/lying server CTag, and the periodic-timer +
  quit-flush goroutines showed no race/deadlock interleaving with the pass 14–15 write paths.
  These two cells are now genuinely warm; absence of a finding is not proof of correctness, only
  that these lenses found nothing.
- **Repro hygiene / build note:** three proposed regression test files are left in the tree and
  currently RED — `internal/model/malformedvtimezone_repro_test.go`,
  `internal/model/repro_vjournal_test.go`, and `cmd/lazyplanner/helpflag_repro_test.go` (all three
  present at session start per `git status`). The other three repros
  (`internal/ui/dblclick_repro_test.go`, `cmd/lazyplanner/configreloadwarn_repro_test.go`,
  `cmd/lazyplanner/badflag_repro_test.go`) were written and run red by the auditor; verify their
  presence and reconcile all six (keep as guards once fixed, or delete) before the next gate — the
  RED files will break `make check`. `go` is not on the default PATH on the audit host (a toolchain
  was installed under the scratchpad); run `go test` with that PATH prepended.
- **Not covered this pass / still stale-or-never:** full `sync-collection` token-delta sync
  (deliberately unbuilt); the Raspberry Pi hardware target (needs a physical Pi); a whole-app
  spec-diff (last at pass 13). The internal/caldav/store/sync boundaries hardened at pass 15 and
  the internal/model recurrence/quick-add/tz cells (pass 14) were deliberately left to cool.

**Recommendation: more passes recommended** — two HIGH, two MED, and two LOW confirmed (all
unfixed), two escaped canaries, HIGH held non-zero, and a recurring encoder-heal class re-emerged.
Fix repro-first: extend the ingest healers to cover VTIMEZONE-required-props and
VJOURNAL/VFREEBUSY (drop-or-heal), and consider codifying "heal every encoder-constrained
component" as a Hard-won guardrail; fold config.Load's warning into the `:config` reload result;
special-case `flag.ErrHelp` and stop re-reporting subcommand parse errors; move the double-click
edit after the selection is updated; and close the two CLI/config canary holes with boundary
tests.
