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
| Mouse handling | internal/ui | light (build step 10) | 10 | stale |
| `:config` reload / $EDITOR flow | internal/ui, internal/config | light | 10 | stale |
| Full `sync-collection` incremental (token delta) | internal/sync | — (deliberately deferred) | — | never |
| go-ical semantic encoder constraints (DUE+DURATION, empty VCALENDAR, VTIMEZONE child) | internal/model | not auto-healed | 4 | stale |
| Raspberry Pi target (on-device timing / kiosk) | (hardware) | — | — | never |

## Declared blind spots (not covered by any pass)

- **Raspberry Pi on real hardware** — on-device timing, kiosk/autologin, bare-TTY
  color. Needs a physical Pi; the sole known-never surface with product risk.
- **Full `sync-collection` incremental sync** — a deliberate feature deferral, not a
  bug (the CTag short-circuit is in place); audit once implemented.
- **go-ical semantic encoder healing** — very low reachability; documented, not fixed.

> The workflow updates this table and this list at the end of each pass. Hand-edits
> are welcome — it's a plain table on purpose.
