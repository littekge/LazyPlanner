# Coverage ledger

The living record of which surfaces have been audited, by what method, and when —
the input the `hardening-audit` workflow reads to pick the *least-audited* surfaces
next. Keep it honest: `status` reflects real coverage, and blind spots are listed,
not hidden. See `PROTOCOL.md`.

`status`: **recent** = covered by a strong method in the last pass or two ·
**stale** = audited a while ago or only weakly/indirectly · **never** = no real audit.

| Surface | Package | Methods used | Last pass | Status |
|---|---|---|---|---|
| iCalendar decode/ingest | internal/model | fuzz, heal-on-ingest | 4 | recent |
| Recurrence expansion (read) | internal/model | fuzz, tz/DST sweep, scale-bound | 4,5,8 | recent |
| Recurrence write-side (mutate/split/advance) | internal/model | fuzz, deep audit, spec-diff | 9,14 | recent (2 MED fixed pass 14: this-&-future split added a phantom trailing occurrence past a pre-split EXDATE — now counts RRULE iterations via rruleIterationsBefore — and duplicated a trailing RDATE across both halves — now partitioned via filterRDates; both restore main.md:362. Codified as a Hard-won guardrail) |
| Subtask tree build | internal/model | fuzz, scale | 4,5 | recent |
| Quick-add parser | internal/model | fuzz, input-edge | 4,14 | recent (MED fixed pass 14: an invalid day-of-month in the slashed `2/30` and month-name `feb 30` forms was silently normalized by time.Date to a wrong date and rolled a year forward — validYMD/rollForwardMonthDay now reject it, matching the ISO form and the leave-text-in-title principle) |
| Timezone / DST | internal/model | exhaustive sweep, fuzz | 8,14 | recent (MED fixed pass 14: resolveDateTime parsed only a single date-time, so an RFC-5545-valid comma-listed multi-valued RDATE/EXDATE — or VALUE=PERIOD RDATE — errored and Occurrences collapsed the whole RRULE series to its base instance; resolveDateTimeValues now splits per value. Codified as a Hard-won guardrail) |
| CalDAV network boundary | internal/caldav | fault-injection, panic-guard | 4,7 | recent |
| Sync engine (data-loss / TOCTOU) | internal/sync | deep audit | 3,11 | recent (HIGH fixed: PullRemoteBatch skips a Dirty resource via store.ErrKeptLocalEdit) |
| Sync reconcile state machine (reconcileCalendar/reconcileReadOnly case matrix, keep-both, Forget branches) | internal/sync, internal/store | data-loss | 13,14 | recent (HIGH fixed pass 13: degraded fetch no longer inferred as deletion. Pass 14 MED fixed: pushDelete's 412 branch cleared the tombstone unconditionally, silently dropping a delete-vs-server-change conflict when the server version was unparseable or absent from a degraded download — it now clears the tombstone only after resurrect+flag, else keeps it and records a skip. Pass 14 LOW fixed: keep-local of a server-deleted conflict never converged — ResolveKeepLocal now clears the Href on a ServerDeleted conflict so reconcile re-creates the item instead of re-raising the conflict) |
| CalDAV request-construction (MKCALENDAR/PROPPATCH/DELETE bodies, resolve()/href, color/name validation, idempotency) | internal/caldav | fault-injection | 13 | recent (2 MED fixed: DELETE now idempotent on 404/410, MKCALENDAR idempotent on 405 — no more pending-delete/pending-create wedge) |
| Sync concurrency | internal/sync | -race stress | 3,11 | recent (re-run post batching/CTag; no new race; the store-level clobber finding is fixed) |
| CTag incremental short-circuit (skip DownloadAll) | internal/sync | data-loss | 11 | recent (no data-loss found in the skip decision itself) |
| Background sync goroutines (startPeriodicSync timer + flushOnQuit quit push) | internal/ui, internal/sync | race | 11 | recent (no deadlock/race found) |
| Store filesystem robustness | internal/store | deep audit (paths, revert, rollback) | 9 | recent |
| Local disk / config input boundaries | internal/config, internal/state, internal/store | deep audit, size caps, fuzz | 9,13 | recent (pass 13 fuzzed TOML parse / first-run round-trip / password_command stdout — no finding; canary hole below: config-read size cap untested) |
| State-file load/parse (widths/hidden-cals/hour-zoom) | internal/state | fuzz (adversarial values), deep audit, size cap | 9,12 | recent (no parse finding; canary hole CLOSED: Save() temp+rename atomicity now tested) |
| Quick-field edits (sp/sd) — Locate→Put write | internal/ui, internal/model | data-loss | 12 | recent (HIGH STATUS-flatten + MED COMPLETED-restamp + MED TOCTOU all fixed) |
| Completion toggle (Space) + recurring-todo advance — Locate→Put | internal/ui | data-loss | 12 | recent (MED TOCTOU fixed: PutIfUnchanged) |
| Session undo stack (pushUndo / prev-snapshot restore replay) | internal/ui, internal/store | data-loss | 12 | recent (HIGH undo-of-synced-delete + MED undo-of-synced-edit fixed: RestoreDirty) |
| Calendar-color + display-name PROPFIND parsing (discoverColors/SyncCalendarName) | internal/caldav | fault-injection | 12 | recent (MED href-key encoding mismatch fixed: hrefKey decodes; also fixed the privileges fail-open + CTag miss) |
| `:calendar` command argument parsing (rename/color/hide/show) | internal/ui | input-edge | 12 | recent (no new finding) |
| Bulk-pull batching / scale | internal/store, internal/sync | benchmarks, data-loss | 5,11 | recent (HIGH fixed: PullRemoteBatch no longer clobbers a href-less pull-orphan's local edit) |
| Grab-mode temporal-manipulation state machine (per-nudge commit, snapshot/2-resource revert) | internal/ui | data-loss, input-edge | 11 | recent (MED+LOW x2 fixed: cancelGrab surfaces revert errors; PutIfUnchanged version-check; HasDue re-check) |
| Recurrence-edit UI orchestration (scope picker + this-&-future split/detach) | internal/ui | data-loss | 11 | recent (HIGH x2 + MED fixed: commitSplit/commitDetach rollback; DetachTodoOccurrence preserves props) |
| UI draw paths (custom widgets) | internal/ui | display stress | 6,14 | recent (pass 14 re-swept the newer widgets — agendaboard/itemforms — no crash/freeze; no finding) |
| UI input handlers (keys/chords/commands) | internal/ui | deep audit, input-edge | 9,13,14 | recent (pass 14 input-edged the raw non-command keypress/chord dispatch (keys.go navigation, grab activation, mode/drill transitions at boundary states) — no finding) |
| CLI wiring | cmd/lazyplanner | deep audit | 9 | recent |
| Mouse handling | internal/ui | input-edge | 10 | recent |
| `:config` reload / $EDITOR flow | internal/ui, internal/config | fault-injection | 10 | recent (MED fixed: $EDITOR shell-split) |
| Store write pipeline atomicity (.ics + sidecar temp/rename) under disk fault | internal/store | fault-injection | 10 | recent (MED fixed: content-hash reconcile; delete-half left to safe re-pull) |
| Yank/paste cross-list move & copy rollback | internal/ui | data-loss | 10 | recent (HIGH+MED fixed: per-component isolate/remove) |
| Feature-promise conformance vs main.md/CLAUDE.md | (whole app) | spec-diff | 10,13 | recent (2 MED fixed: applyMutation (edit form + recur-scoped saves) and reparentSelected H/L now route through PutIfUnchanged — the "no Locate→Put clobber sites remain" invariant is now true again) |
| Full `sync-collection` incremental (token delta) | internal/sync | — (deliberately deferred) | — | never |
| go-ical semantic encoder constraints (DTEND/DUE+DURATION, empty VTIMEZONE, VJOURNAL/VFREEBUSY nesting) | internal/model | fuzz (re-encode round-trip) | 10 | recent (4 HIGH + 1 MED fixed: ingest healers) |
| Raspberry Pi target (on-device timing / kiosk) | (hardware) | — | — | never |

## Declared blind spots (not covered by any pass)

- **Raspberry Pi on real hardware** — on-device timing, kiosk/autologin, bare-TTY
  color. Needs a physical Pi; the sole known-never surface with product risk.
- **Full `sync-collection` incremental sync** — a deliberate feature deferral, not a
  bug (the CTag short-circuit is in place); audit once implemented.
- **`DayAgenda` inclusive dayStart boundary** (pass-14 canary escape) — RESOLVED (pass 14):
  `TestDayAgendaIncludesTodoDueAtMidnight` now pins a todo due exactly at 00:00, verified to
  fail under the `Before→After` mutation. See the pass-14 canary section.
- **go-ical semantic encoder healing** — RESOLVED (pass 10 fix): the five
  decode-but-unencodable classes (VEVENT DTEND+DURATION, VTODO DUE+DURATION, VTODO
  DURATION-without-DTSTART, empty VTIMEZONE incl. the `stripForbiddenNesting` self-
  inflict, VJOURNAL/VFREEBUSY nesting) are now healed on ingest with regression tests.

## Escaped mutation canaries — pass 13 (4 of 4 escaped → all CLOSED 2026-07-16)

Code was correct; each path was unguarded so a plausible regression would ship silently.
Each is now closed with a boundary test verified to fail under its mutation before adding.

- **`internal/model/timegrid.go` `LayoutDay`** (cluster-flush / lane-reuse at a touching
  boundary): a `!start.Before`→`start.After` or `!le.After`→`le.Before` flip folds a
  touching occurrence into the prior cluster and inflates a standalone block's `Lanes`.
  CLOSED — `TestLayoutDayTouchingBoundary` asserts lane-minimality at the touching edge.
- **`internal/sync/sync.go` `Sync`** (CTag-cache guard): `len(res.Skipped) == skipsBefore`
  → `>=` collapses to always-cache, so a per-resource failure caches the CTag anyway and
  the next sync never retries. CLOSED — `TestDegradedDownloadDoesNotCacheCTagSoNextSyncRetries`
  (added with the HIGH #1 fix) asserts the CTag is not cached after a skip.
- **`internal/caldav/object.go` `DeleteObject`**: dropping the empty-ETag `If-Match: *`
  fallback turns a conditional delete into a blind unconditional DELETE. CLOSED —
  `TestDeleteObjectEmptyETagSendsIfMatchStar` inspects the outgoing header.
- **`internal/config/config.go` `Load`**: dropping `io.LimitReader(f, maxConfigBytes)`
  removes the 4 MiB read cap. CLOSED — `TestLoadCapsReadSize` feeds an oversized file
  (valid before the cap, garbage after) so an uncapped read would error.

## Escaped mutation canary — pass 14 (1 of 3 escaped → now CLOSED)

The escape was a test-coverage hole (code correct, path unguarded); it is now closed.

- **`internal/model/agenda.go` `DayAgenda`** (todo due-time lower bound): flipping the
  inclusive lower bound `!t.Due.Before(dayStart)` (Due ≥ dayStart) → `t.Due.After(dayStart)`
  (Due > dayStart) drops any todo due *exactly* at the start of the day (midnight) — the
  natural due time for a date-only / all-day todo — silently vanishing it from that day's
  agenda. `TestDayAgenda`'s todos were due at 09:00 (inside the window) and dayEnd+1h
  (outside); none sat exactly on dayStart, so the flipped boundary was never exercised.
  CLOSED (pass 14): `TestDayAgendaIncludesTodoDueAtMidnight` pins a todo due exactly at
  dayStart, verified to fail under the mutation and pass after reverting.
- The other two canaries were **caught**: `internal/ui/grab.go` `grabNudge` J/K resize min-
  duration guard (`TestGrabResizeRejectsZeroDuration`), and `internal/sync/sync.go` CTag-cache
  guard (`TestDegradedDownloadDoesNotCacheCTagSoNextSyncRetries`, added with pass 13's HIGH #1).

## Pass 11 — RESOLVED (all 7 findings + both canary holes fixed 2026-07-15)

Every pass-11 finding was fixed with an adversarially-verified regression test and
its own commit; the full gate + `-race` on store/sync/ui pass. See `log.md`.

- **3 HIGH data-loss** (the shared "multi-write op with no rollback" class):
  `PullRemoteBatch` now skips a Dirty resource (`store.ErrKeptLocalEdit`) instead of
  clobbering a concurrent local edit; `commitSplit` restores the master when the
  future write fails; `commitDetach` (extracted from `editTodoDetachForm`) restores
  the series when the standalone write fails. All three now match the sibling
  `beginGrabFuture` rollback.
- **2 MED:** `cancelGrab` captures and surfaces revert errors (and restores before
  deleting so a failed un-cap can't compound); the todo detach uses the new
  `model.DetachTodoOccurrence`, which clones the original component so unmodeled iCal
  props (VALARM/X-/etc.) survive (iron rule).
- **2 LOW:** `grabNudge` commits via the new `store.PutIfUnchanged` (version-checked
  write; aborts without reverting on a concurrent pull); the todo nudge re-checks
  `HasDue` after re-locate.
- **2 escaped mutation canaries → closed:** boundary tests added for the month-grid
  event-drill `j`/`k` guard (`internal/ui/calendarview.go`, both KeyRune + arrow
  paths) and `clampIndex` at `i == n` (`internal/ui/edit.go`); each verified to fail
  under its mutation. (The both-sides-changed conflict-comparison canary in
  `internal/sync/sync.go` was already caught by 5 tests.)

## Pass 12 — RESOLVED (all 7 findings + all 3 canary holes fixed 2026-07-15)

Pass 12 audited the pass-11 named follow-up (the Locate→Put no-version-check pattern
outside grab) plus the session undo stack, the calendar color/name PROPFIND decode,
and (fuzz/edge) the state-file parse and `:calendar` arg parsing. 7 findings (2 HIGH,
5 MED) were confirmed with executed repros; **all are now fixed**, each adversarially
verified with a regression test and its own commit; full gate + `-race` on
caldav/state/store/sync/ui pass. See `log.md`.

- **HIGH — quick sp/sd flattened STATUS + dropped PERCENT-COMPLETE / restamped COMPLETED
  (MED):** `EditTodo` now calls `setCompleted` only when completed-ness actually changes
  (`isCompletedStatus`), preserving a foreign client's IN-PROCESS/CANCELLED status,
  PERCENT-COMPLETE, and the original COMPLETED timestamp.
- **HIGH — undo of a synced delete lost the item / MED — undo of a synced edit didn't
  stick:** new `store.RestoreDirty` marks the resurrection/revert Dirty; `undoLast` uses
  it, so sync pushes it or raises a keep-both conflict instead of Forgetting/pulling-back.
  Verbatim `Restore` stays for the rollback paths.
- **MED x2 — the systemic Locate→Put:** `applyTodoField` (sp/sd), `toggleComplete` (Space),
  and `advanceRecurringTodo` now commit via `store.PutIfUnchanged(loc.Prev)`, aborting on
  `applied==false`. All three of the systemic sites the pass-11/12 reports named are closed.
- **MED — raw-href keying:** new `caldav.hrefKey` decodes the href like go-webdav derives
  `Calendar.Path`; the color, privilege (discover + reactive re-check), and CTag maps plus
  the lookup key all use it. This also fixed the privileges **fail-open** (read-only shares
  looked writable) and the CTag miss.

No parse-level finding on the state file or `:calendar` arg parsing.

**All 3 escaped mutation canaries → closed** (code was correct; tests added, each verified
to fail under its mutation): `privileges.go` `writable()` per-grant table (write /
write-content / bind / all independently); `state.go` `Save` temp+rename atomicity; `grab.go`
K-resize rejecting a zero-duration event.

## Systemic follow-up — the Locate→Put no-version-check pattern (RESOLVED again, pass 13)

Pass 11 fixed grab; pass 12 fixed three sites (quick-field `sp`/`sd`, `Space`
completion, recurring-todo advance) via `store.PutIfUnchanged` and declared the class
"structurally closed". **Pass 13's spec-diff proved that claim FALSE** — the sweep found
two remaining unconverted sites sharing the exact TOCTOU clobber class, now both fixed:
- **`applyMutation`** (`internal/ui/edit.go`) — the shared tail of every form Save and
  the `commitMutation`/`commitMutationKeepingDrill` callers (edit form + all
  recurrence-scoped saves). FIXED: version-checks an edit (`prev != nil`) via
  `store.PutIfUnchanged`, surfaces a stale skip; creations still use plain `Put`.
- **`reparentSelected`** (`internal/ui/edit.go`, `H`/`L` indent/outdent). FIXED: commits
  via `store.PutIfUnchanged(loc.Prev)` with an `applied==false` retry flash.

Regression tests: `internal/ui/editclobber_test.go` (drives the real `applyMutation`)
and `internal/store/reparent_noclobber_test.go`. The lesson stands: "closed" requires an
exhaustive site sweep, not a spot check — do the sweep before declaring the class shut.
No known Locate→Put clobber sites remain after this exhaustive sweep.

## Delete-half of the write-atomicity finding (intentionally not "healed")

- A crash between an `.ics` **delete** and its tombstone write re-pulls the item on
  next sync — safe and recoverable. Synthesizing a tombstone from a missing-`.ics`-
  with-href would risk deleting server data whenever a `.ics` merely went missing, so
  the safe re-pull is kept by design (not a gap to close).

> The workflow updates this table and this list at the end of each pass. Hand-edits
> are welcome — it's a plain table on purpose.
