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
| Recurrence write-side (mutate/split/advance) | internal/model | fuzz, deep audit | 9 | recent |
| Subtask tree build | internal/model | fuzz, scale | 4,5 | recent |
| Quick-add parser | internal/model | fuzz | 4 | recent |
| Timezone / DST | internal/model | exhaustive sweep | 8 | recent |
| CalDAV network boundary | internal/caldav | fault-injection, panic-guard | 4,7 | recent |
| Sync engine (data-loss / TOCTOU) | internal/sync | deep audit | 3 | recent |
| Sync concurrency | internal/sync | -race stress | 3 | recent |
| Store filesystem robustness | internal/store | deep audit (paths, revert, rollback) | 9 | recent |
| Local disk / config input boundaries | internal/config, internal/state, internal/store | deep audit, size caps | 9 | recent |
| Bulk-pull batching / scale | internal/store, internal/sync | benchmarks | 5 | recent |
| UI draw paths (custom widgets) | internal/ui | display stress | 6 | recent |
| UI input handlers (keys/chords/commands) | internal/ui | deep audit | 9 | recent |
| CLI wiring | cmd/lazyplanner | deep audit | 9 | recent |
| Mouse handling | internal/ui | input-edge | 10 | recent |
| `:config` reload / $EDITOR flow | internal/ui, internal/config | fault-injection | 10 | recent (MED fixed: $EDITOR shell-split) |
| Store write pipeline atomicity (.ics + sidecar temp/rename) under disk fault | internal/store | fault-injection | 10 | recent (MED fixed: content-hash reconcile; delete-half left to safe re-pull) |
| Yank/paste cross-list move & copy rollback | internal/ui | data-loss | 10 | recent (HIGH+MED fixed: per-component isolate/remove) |
| Feature-promise conformance vs main.md/CLAUDE.md | (whole app) | spec-diff | 10 | recent |
| Full `sync-collection` incremental (token delta) | internal/sync | — (deliberately deferred) | — | never |
| go-ical semantic encoder constraints (DTEND/DUE+DURATION, empty VTIMEZONE, VJOURNAL/VFREEBUSY nesting) | internal/model | fuzz (re-encode round-trip) | 10 | recent (4 HIGH + 1 MED fixed: ingest healers) |
| Raspberry Pi target (on-device timing / kiosk) | (hardware) | — | — | never |

## Declared blind spots (not covered by any pass)

- **Raspberry Pi on real hardware** — on-device timing, kiosk/autologin, bare-TTY
  color. Needs a physical Pi; the sole known-never surface with product risk.
- **Full `sync-collection` incremental sync** — a deliberate feature deferral, not a
  bug (the CTag short-circuit is in place); audit once implemented.
- **go-ical semantic encoder healing** — RESOLVED (pass 10 fix): the five
  decode-but-unencodable classes (VEVENT DTEND+DURATION, VTODO DUE+DURATION, VTODO
  DURATION-without-DTSTART, empty VTIMEZONE incl. the `stripForbiddenNesting` self-
  inflict, VJOURNAL/VFREEBUSY nesting) are now healed on ingest with regression tests.

## Resolved this pass (were blind spots / open findings)

- **All 9 pass-10 findings fixed** (5 HIGH, 4 MED), each with a green regression test.
- **The 3 mutation-canary escapes are closed** with a test apiece — backward search
  wrap (`searchNext(-1)`), PRIORITY `>9` clamp, and the href-less pull-orphan
  `HasPendingChanges`/`HasLocalChanges` clause. The priority test was mutation-verified
  (fails under the canary's mutation, passes on correct code).

## Delete-half of the write-atomicity finding (intentionally not "healed")

- A crash between an `.ics` **delete** and its tombstone write re-pulls the item on
  next sync — safe and recoverable. Synthesizing a tombstone from a missing-`.ics`-
  with-href would risk deleting server data whenever a `.ics` merely went missing, so
  the safe re-pull is kept by design (not a gap to close).

> The workflow updates this table and this list at the end of each pass. Hand-edits
> are welcome — it's a plain table on purpose.
