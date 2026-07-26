# LazyPlanner — Working Notes

> **Purpose**: short-term working memory — the state of a task in progress, written only when a session ends **mid-arc**. Record what's in progress, the remaining steps, blockers, and any temporary context the next session needs to pick the work back up. **The healthy steady state of this file is empty** (nothing below this header). Date every entry. When the task completes, delete its notes in the same increment that writes the `log.md` completion entry — resolution belongs to `log.md`, design to `main.md`; nothing accumulates here. A note that survives more than a few sessions is a misplaced `main.md` fact — move it there.

---

## 2026-07-26 — Two arc-introduced regressions to fix, then release

**Pick this up first.** The TZID-anchored-recurrence arc (commits `9d3ccd7..680f10a`, `666f829`) fixed three
real recurrence defects but introduced two regressions of its own. Both are recorded in full in
`docs/audit/COVERAGE.md` "Carried residual → Pass 24 targets" items **2 and 3** — read those, they carry the
repro detail. Summary and marching orders below.

### 1. HIGH — a recurring item loses the day its zone's DST change starts on

- **Symptom**: ticking a daily task due `2026-03-07` in `America/Havana` rolls it to `03-09`, skipping a
  day. Persisted locally and pushed to the server.
- **Cause**: rrule-go normalizes a non-existent local midnight *backwards*, so the gap day is never
  generated. Anchor iteration trusts `time.Date` normalization.
- **Zones**: `America/Havana`, `America/Santiago`, `Atlantic/Azores` — those whose DST transition is *at*
  local midnight. **Not `America/New_York`** (transitions at 02:00, so local midnight always exists), which
  is this machine's zone, so the owner is not affected.
- **Why it appeared now**: pre-arc it was reachable only for *server-authored* TZID items. Moving
  app-authored anchors out of gapless UTC widened it to everything.
- **Fix direction**: make anchor iteration gap-safe rather than trusting `time.Date` normalization.
- **Owner's decision (2026-07-26)**: fix this before release. It is data corruption and it is ours.

### 2. MED — `store` and the UI disagree about the local zone

- **Cause**: `store.loadResource` decodes with `time.Local` while `a.loc` is now `config.LocalZone()`
  (introduced in `86f594f`).
- **Diverges on**: `TZ=` set-but-empty, `TZ=:Asia/Tokyo`, `TZ=/abs/path`, and a stale-but-loadable
  `/etc/timezone`.
- **Symptoms**: an all-day event rendering on two days, floating times off by the offset, "today"
  resolving to the wrong day.
- **Owner's decision (2026-07-26)**: **size the fix before committing to it.** It touches a core decode
  path, so it could be two lines or it could ripple. Impact on this host is near zero (both resolve to
  `America/New_York`). If the fix is not small, **leave it as a documented residual** — that is an
  acceptable ending, not a failure.

### Release context the next session needs

- **v1.5.0's release gate is already satisfied** (`main.md` line ~550: phase 3 required one deep pass;
  pass 19 delivered it and four more followed). Passes 20–23 were discretionary.
- Only **two** coverage rows are state `never`, and both are permanently accepted gaps (full
  `sync-collection` delta sync, deferred by design; Raspberry Pi hardware, unauditable headlessly).
- **The finish line is: fix regression 1, size regression 2, final gate, then the owner merges and tags
  v1.5.0.** Everything else on the board is discretionary hardening.
- The owner has limited time remaining on this project. Do not open new hardening work, and do not start
  a new audit pass, without asking.

### Deliberately NOT to be implemented

`docs/superpowers/specs/2026-07-26-document-health-design.md` and
`docs/superpowers/plans/2026-07-26-document-health.md` are a complete, reviewed design and plan for
preventing documentation growth. **The owner decided on 2026-07-26 not to build it**: the mechanism
amortizes over many future sessions, and the project is reaching its final state. The plan file carries a
banner saying so. Leave both in place as a record of a considered decision — do not execute them.

A second design (reducing context clutter by letting agents read documents selectively) was scoped and
dropped for the same reason. It was never written up.
