# Pass 24 — post-arc surfaces, CANCELLED mid-run

**Date**: 2026-07-26
**Branch / baseline**: `ai-workspace` @ `61bb158`
**Methods planned**: fuzz, spec-diff, fault-injection, race, data-loss, input-edge
**Prior pass (23)**: 6 HIGH / 6 MED / 3 LOW

> This is an evidence report, not a verdict. It is also an **incomplete** pass: the owner cancelled the
> run partway through, and the remaining budget was spent fixing what it had already found rather than
> finishing it. Everything below is bounded by that.

---

## Why the pass is incomplete

The run (`wf_eed53390-0f8`) was launched with no bounds, resumed once, and then exhausted the session's
usage budget mid-flight. It never reached its own verify/canary/synthesis stages, so:

- **No adversarial verification.** PROTOCOL rule 3's N-skeptics-must-fail-to-refute gate never ran on
  the findings below. They are *not* protocol-grade "confirmed".
- **No mutation canaries.** `canarySummary` is empty. The test net was not measured this pass.
- **No `enforcement` block, no residual-risk synthesis.** This report is hand-written from the run's
  journal plus direct re-execution of the reproductions it left behind.

What *is* solid: every finding below shipped a reproduction that was **executed directly** and observed
to fail at `61bb158`, then observed to pass after the fix, then mutation-checked by hand. That satisfies
PROTOCOL rule 4 (a repro or it didn't happen) and rule 5 (a regression test per fix), but not rule 3.

**Cost note for the next pass.** The workflow's agent count is findings-driven and uncapped:
`12 + 4F` with the defaults (`maxTargets=6`, `skeptics=3`, `maxCanaries=4`), because every finding
spawns 3 refuters plus a repro agent. Six `never`/`stale` targets produced roughly 43 raw findings and
185 agents. Bound the next run (`maxTargets`, `skeptics`) or add a per-target findings cap to the script
before launching.

## Targets the Plan phase chose

Notably it ranked the code that landed the same day (wall-clock recurrence expansion, gap-safe decode,
`installProcessZone`) as `never`-audited and made it target #1 — the ledger did *not* hide fresh code.

1. `internal/model` — fuzz — wall-clock expansion + gap-safe decode core
2. `cmd/lazyplanner`, `internal/config`, `internal/ui` — spec-diff — the one-process-zone promise
3. `internal/caldav` — fault-injection — write paths, bare-write / resource-is-gone classes
4. `internal/sync`, `internal/ui`, `internal/store` — race — background sync vs the new decode path
5. `internal/ui`, `internal/store` — data-loss — undo replay, `sp`/`sd`, completion toggle
6. `internal/model`, `internal/ui` — input-edge — day bucketing in DST-gap zones

Named blind spot from the plan: `internal/ui/colorpicker.go` and `help.go` still have no ledger row.

---

## Results — 5 defects reproduced (1 HIGH / 4 MED), 4 fixed

Four probes the run also left behind **passed** and are recorded as no-finding: an EXDATE+COUNT foreign
VTODO through the real Space path, a month/day-view probe across a gap day, and two all-day grab probes
across a gap day.

| # | Sev | Surface | Status |
|---|---|---|---|
| H1 | HIGH | `internal/caldav` | fixed — `c63dacf` |
| M1 | MED | `internal/sync` | fixed — `f3c1f5b` |
| M2 | MED | `internal/ui` | fixed — `bb7be2d` |
| M3 | MED | `internal/ui` | fixed — `bb543c6` |
| M4 | MED | `internal/model` | **OPEN** — see Residual risk |

### H1 — an href can move an authenticated write off the endpoint's origin

RFC 3986 reads a leading `//` as the start of an authority, so a server-supplied href of
`//evil.host/x.ics` is a protocol-relative URL, not a path. `Client.resolve` did
`endpoint.ResolveReference(url.Parse(ref))`, which replaced the endpoint's host with it. Observed:
`PutObject` delivered the account's Basic-auth app password **and** the full calendar body to a foreign
host, returned that host's ETag with `err=nil` — so the store marked the resource cleanly pushed — while
the honest endpoint received zero requests. `DeleteObject` and MKCALENDAR share the shape, and
`DownloadAll` surfaces such an href verbatim as `Resource.Href`, so it persists in the sidecar and every
later write re-targets that host.

Fixed in two layers: `Client.resolve` enforces same-origin (covering hrefs already poisoned in a
sidecar), and `DownloadAll` drops an authority-bearing href at ingest. Each layer kills a different
subset of the guards. Accepted cost: a deployment whose server advertises hrefs on a *different* origin
than the configured endpoint now errors instead of following them.

### M1 — a tombstone never converges when the resource is already gone server-side

`pushDelete`'s 412 branch conflated "the server version could not be fetched this pass" with "the
resource is genuinely gone". Over three syncs the tombstone stayed pending every time, and because the
accompanying skip suppresses the CTag cache, the calendar re-downloaded in full forever and never
stopped reporting local changes. Reconcile already draws the distinction with its `unfetched` map;
`pushDelete` never received it — the "mirror a guard onto every sibling path" rule.

### M2 — `sd` did not re-anchor a recurring todo's day-pinning rule

Not a new class: the existing guardrail "Moving a recurring item's anchor must re-anchor its day-pinning
`BY*`" reached through a door its sweep list never named. The fix now lives in `applyTodoField`, the
quick-set chokepoint, and the guardrail gained the lesson (enumerate the doors, not the features) plus a
day-change gate so a time-only set on a *Custom rule (kept)* todo is not refused.

### M3 — a task's LOCATION erased by a quick-set and by a no-op form save

Iron-rule violation. `TodoDraft` carries `Location` and `applyTodo` writes it, but `draftFromTodo`
omitted it and the task form had no Location input at all, so `setTextOrDel` deleted the property. The
event form has had the field all along.

---

## Residual risk

### M4 (OPEN) — a timed recurring todo bakes a DST gap into its anchor, permanently

**Not arc-introduced**: verified byte-identical at `08d75c3`, before the 2026-07-26 wall-clock work.
Same reachability the day-loss bug had (server-authored TZID items).

A recurring todo whose wall clock falls inside a spring-forward gap — e.g.
`DUE;TZID=America/New_York:20260307T023000` with `FREQ=DAILY`, where 2026-03-08 02:30 does not exist —
advances to 01:30 instead of 02:30, and **that shifted time is written back as the new anchor**, so
every later occurrence inherits it. Measured over four advances: `013000` on 03-08, 03-09, 03-10, 03-11.
Persisted locally and pushed to the server.

**Why it was not fixed here.** The drift is structural, not a missing guard. `AdvanceRecurringTodo`
re-anchors the series on the *resolved instant*, and the stored anchor is the only memory of the
authored wall clock. Storing the snapped post-gap instant instead just drifts the other way; storing the
nominal wall clock as text does not help either, because decoding it round-trips through a `time.Time`
in a zone where it does not exist. A real fix means carrying the recurrence anchor as a **wall clock**
rather than an instant — touching `componentRecurrenceSet`, `AdvanceRecurringTodo` and the anchor
writers. That was judged too wide to land immediately before the v1.5.0 release.

Reproduction (executed, failing) is preserved at
`~/.claude/projects/<project>/audit-pass24-preserved/repros/internal/model/dstgap_timed_repro_test.go`.

### Carried, untouched by this pass

- Everything the pass never reached: no adversarial verification, no canaries, no race or fault-injection
  results beyond H1, and targets 2/4/5/6 produced no reported findings only because the pipeline stopped.
- `internal/ui/colorpicker.go` and `help.go` — still no ledger row, still never audited.
- The day-*bucketing* half of the DST-gap class (`model.DayStart`, quick-add relative dates) — ledger
  item 4, still open.

## Recommendation

`more_passes_recommended`. This pass cannot support any convergence claim: PROTOCOL's criteria 2, 3 and 5
all require evidence it never produced (two consecutive no-HIGH passes, no new root-cause class, and a
canary escape rate near zero). It also found a **HIGH** on its first look at `internal/caldav` write
paths — a surface flagged as unswept for two consecutive prior passes — which is one more confirmation
that `never`/`stale` rows are high-yield.

The next pass should re-run bounded, start from the four targets that were planned but never produced
verified results, and add the missing canary stage.
