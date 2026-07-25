# Pass 22 — store/config fault-injection + caldav conditional-write + reconcile matrix race + v1.3.0 Custom sub-form input-edge

- **Date:** 2026-07-25
- **Prior pass:** Pass 21 (read-side recurrence expansion + go-ical heal-set recursion + caldav request-construction) — HIGH 1 · MED 2 · LOW 0 (all three FIXED 2026-07-25)
- **This pass:** HIGH 0 · MED 1 · LOW 0 (CONFIRMED, UNFIXED; repro ran RED)
- **Canary escapes:** 3 of 4 (all OPEN)
- **Status (2026-07-25): finding CONFIRMED, UNFIXED.** The body below is the as-found evidence, not a verdict.

This pass took the ledger's least-audited high-value cells left after pass 21: the **pass-20/21 store
write primitives** never fault-injected (only race-stressed), the **caldav write-method conditional
semantics** (the caldav side of the standing cross-package resource-is-gone residual), the never-deeply-
audited **v1.3.0 Custom repeat sub-form field validation**, the **deeper reconcile matrix** (keep-both /
(B) push-vs-pull, flagged "warm but not cleared"), and the never-fault-injected **`password_command` exec
+ config file-read I/O boundary**.

Five method-diverse audits (fault-injection ×2, data-loss, race, input-edge) landed **one confirmed
finding** (a MED on the v1.3.0 Custom sub-form) with a repro that ran RED. Raw severity is trending
**down** (total 3→1, HIGH 1→0), but the mutation-canary phase was unusually bad this pass: **3 of 4
canaries escaped**, all test-coverage holes on real correctness-bearing paths (caldav ETag normalization,
a TUI slice-bounds guard, and the read-only dirty-discard invariant). The surface is not converged.

---

## Coverage exercised

| Surface | Package | Method | Result |
|---|---|---|---|
| Store write primitives (removeLocked/ForgetIfUnchanged compare-and-remove, tombstone create/advance, .ics+sidecar temp/rename) under ENOSPC/rename-fail/partial-write | internal/store | fault-injection | **no finding** (atomic temp+rename and lock-held compare-and-remove degrade/roll back cleanly; these primitives were race-stressed pass 20/21 but never fault-injected — last fault-injection was pass 15, before they existed) |
| CalDAV write-method conditional semantics (PutObject/DeleteObject If-Match/If-None-Match, 412 lost-update surfacing) | internal/caldav | data-loss | **no CONFIRMED finding** (but a `normalizeETag` weak-validator canary ESCAPED — see canaries) |
| v1.3.0 recurrence Custom repeat sub-form (recurcustom.go count/until/unit/monthly-by/ends field validation) | internal/ui, internal/model | input-edge | **1 MED** ("Ends on date" drops the selected end date for timed items) |
| Sync reconcile matrix beyond step-A/read-only-twin (keep-both conflict staging, (B) push-vs-pull racing a concurrent pull) | internal/sync | race | **no CONFIRMED finding** (but a `reconcileReadOnly` dirty-discard canary ESCAPED — see canaries) |
| Config load I/O boundary — `password_command` external-process exec (nonzero-exit/missing-binary/empty-huge-stdout/hang) + unreadable/partial config-file reads | internal/config | fault-injection | **no finding** (a failing/absent password_command surfaces its error and never hangs/crashes startup; an unreadable/oversized config degrades to a surfaced error, not a silent bad parse) |

---

## Confirmed findings (carries a runnable repro; ran RED)

### MED

**1. "Ends on date" silently drops the selected end date's occurrence for TIMED items** —
`internal/ui/recurcustom.go:247`.

The v1.3.0 Custom repeat sub-form's "Ends on date" field is parsed by `readCustomRecur` via
`parseDateField` (internal/ui/edit.go:1179), which returns **midnight-in-loc** of the chosen day, and
stored verbatim as `spec.Until` (ROption, internal/model/quickadd.go:91-93). But `applyRecurrence` only
converts UNTIL to an inclusive date-only value (`dateOnlyUntil`, internal/model/recur_edit.go:353) for
**all-day** anchors. For a **timed** recurring item UNTIL stays at 00:00, so every occurrence on the
user-selected end date — which carries a nonzero time-of-day — falls *after* UNTIL and is excluded.

- **Failure scenario:** User creates a timed daily event starting 2026-07-20 15:00, opens Custom repeat,
  chooses Ends = "On date" and enters 2026-07-25. The stored rule is
  `FREQ=DAILY;UNTIL=20260725T000000Z`, which yields occurrences only through 07-24 (07-25 15:00 >
  UNTIL 00:00). The end date the user explicitly selected is dropped, with no error and no warning. The
  same "Ends on 2026-07-25" produces an *inclusive* series for an all-day item (`dateOnlyUntil` makes
  UNTIL a `VALUE=DATE`), so timed and all-day items disagree on what "Ends on date D" means.
- **Repro (ran RED, then removed to keep the gate green):**
  `TestReproEndsOnDateDropsTimedOccurrence` — place in `internal/model` and run
  `go test ./internal/model/ -run TestReproEndsOnDateDropsTimedOccurrence -v`:

```go
package model

import (
	"testing"
	"time"
)

func TestReproEndsOnDateDropsTimedOccurrence(t *testing.T) {
	loc := time.UTC
	start := time.Date(2026, 7, 20, 15, 0, 0, 0, loc)
	// parseDateField("2026-07-25") -> midnight local of the selected end date.
	until := time.Date(2026, 7, 25, 0, 0, 0, 0, loc)

	spec := RecurSpec{Freq: FreqDaily, Until: &until}
	draft := EventDraft{
		Summary: "Timed daily",
		Start:   start,
		End:     start.Add(time.Hour),
		Recur:   &spec,
	}
	p, err := NewEventObject(draft, start)
	if err != nil {
		t.Fatalf("NewEventObject: %v", err)
	}

	occs, err := p.EventOccurrences(
		time.Date(2026, 7, 20, 0, 0, 0, 0, loc),
		time.Date(2026, 8, 1, 0, 0, 0, 0, loc),
	)
	if err != nil {
		t.Fatalf("EventOccurrences: %v", err)
	}

	target := time.Date(2026, 7, 25, 0, 0, 0, 0, loc)
	found := false
	var got []string
	for _, o := range occs {
		got = append(got, o.Start.Format("2006-01-02"))
		if o.Start.Year() == target.Year() && o.Start.YearDay() == target.YearDay() {
			found = true
		}
	}
	if !found {
		t.Fatalf("selected end date 2026-07-25 was dropped from a timed series; got %v", got)
	}
}
```

  Observed (before any fix):
  ```
  occurrences: [2026-07-20 2026-07-21 2026-07-22 2026-07-23 2026-07-24]
  selected end date 2026-07-25 was dropped from a timed series
  --- FAIL: TestReproEndsOnDateDropsTimedOccurrence
  ```
- **Fix direction (not applied):** when the "Ends on date" field is set, make UNTIL inclusive of the
  whole selected day for a timed anchor — e.g. set Until to end-of-day (23:59:59) or to the anchor's
  clock time on the selected day, so a same-day timed occurrence is retained. This mirrors the all-day
  `dateOnlyUntil` behavior so both item types agree on "Ends on date D".

---

## Mutation canaries — 3 of 4 escaped (all OPEN)

Four canaries injected across `internal/store`, `internal/caldav`, `internal/ui`, `internal/sync`. Only
the store one was caught — an unusually high **3-of-4 escape rate**. Each escape is a test-coverage hole:
the code is correct today, but a plausible regression on that exact path would ship silently.

- **ESCAPE (OPEN)** — `internal/caldav/object.go` `normalizeETag`: deleting the
  `etag = strings.TrimPrefix(etag, "W/")` weak-validator strip passed `go test ./internal/caldav/`. A
  server ETag returned as a weak validator `W/"abc"` is then stored verbatim (the leading `W/` defeats the
  later quote-strip since `etag[0]!='"'`), breaking a subsequent If-Match compare against a bare stored
  ETag and surfacing spurious 412 conflicts. Hole: every ETag-asserting test (`TestPutObjectCreate`,
  `TestPutObjectUpdateSendsQuotedIfMatch`, `TestPutObjectAccepts200OK`) uses only strong, double-quoted
  ETags; a grep for `W/`/`weak`/`normalizeETag` in the caldav test files returns zero. Close with
  `normalizeETag("W/\"abc\"") == "abc"`, verified RED under the removed `TrimPrefix`.
- **ESCAPE (OPEN)** — `internal/ui/selection.go` `drillRange` bounds guard: weakening `idx >= len(items)`
  → `idx > len(items)` passed `go test ./internal/ui/`. When the drilled cursor index equals `len(items)`,
  the guard no longer returns nil, so `ci = len(items)` and the slice `items[ai : ci+1]` =
  `items[ai : len(items)+1]` slices out of range — a runtime panic that freezes the single-threaded TUI on
  a bulk-select over a drilled day at the terminal index. Hole: `drillRange` is never referenced by name in
  the ui tests, and no test drills a day then extends a SELECT range to the terminal index (tests cover
  `daysRange`/`maxSelectDays` but not `drillRange`'s upper-index boundary). Close with a test that drills a
  day and extends the range onto `len(items)`, verified to panic under the mutation.
- **ESCAPE (OPEN)** — `internal/sync/sync.go` `reconcileReadOnly` (~line 513): weakening the read-only
  discard guard `if r.Dirty || r.Href == ""` → `if r.Dirty && r.Href == ""` passed `go test
  ./internal/sync/`. A locally-edited resource that already has an href (a previously-synced item edited
  after its calendar became read-only) is then no longer discarded — the un-pushable local edit survives
  (the switch has no Dirty branch), silently violating the "read-only calendars never keep local changes"
  hard invariant. Hole: the only read-only-discard test (`TestSyncReadOnlyDiscardsStuckAndMirrors`) uses a
  never-synced resource (Href="", Dirty=true) that satisfies BOTH the original OR and the mutated AND, so
  it cannot distinguish them. Close with a synced-then-edited (Dirty, Href≠"") resource on a read-only
  calendar asserting `res.Discarded==1` / the resource is forgotten.
- **CAUGHT** — `internal/store/mutate.go` `remove()` tombstone guard: dropping the `r.Href != ""` clause
  (`if tombstone && r.Href != ""` → `if tombstone`) → `go test ./internal/store/` FAILED
  (`TestDeleteNeverSyncedLeavesNoTombstone`, `TestCommitPushHonorsDeleteOfNeverSyncedCreate`). A
  never-synced local delete would otherwise leave a tombstone triggering a pointless server DELETE. The
  net on this surface has teeth.

---

## Convergence & residual risk

- **Severity trend:** down in raw counts — total findings 3 (pass 21) → 1 (pass 22); HIGH 1 → 0; MED 2 →
  1; LOW 0 → 0. The one finding is a functional MED (a dropped occurrence), not a data-loss or brick.
- **Canary trend:** worse — 1 escape (pass 21, a false-positive-adjusted 1-of-4) → 3 genuine escapes
  (pass 22). All three are latent-regression coverage holes on correctness paths, not current bugs, but
  they mean three exact-path regressions would ship silently until closed.
- **Residual / uncovered this pass:**
  - The MED finding is CONFIRMED and UNFIXED (this is the evidence-report step, not a fix arc).
  - Three canary escapes are OPEN (caldav `normalizeETag` weak-ETag, ui `drillRange` bound, sync
    `reconcileReadOnly` dirty-discard) — each closable with a one-boundary test.
  - The standing **cross-package resource-is-gone residual** — `internal/model` and `internal/caldav`
    peer write paths — was *touched* on the caldav side (conditional-write semantics data-loss sweep, no
    CONFIRMED finding) but not exhaustively swept; `internal/model` is pure/headless so the concurrency
    class can't literally manifest there.
  - Parser surfaces (decode/ingest, quick-add, tz/DST, color), go-ical heal-set spec-diff (just fixed
    pass 21), and whole-app feature-promise spec-diff were **not** re-run this pass — a newly-introduced
    path since pass 21 would be missed.
  - Permanent blind spots unchanged: Raspberry Pi on real hardware; full `sync-collection` incremental
    (deferred, not built).

## Recommendation

**more_passes_recommended.** A MED was confirmed and left unfixed, and 3 of 4 mutation canaries escaped
(all OPEN). Next pass should: (1) fix the "Ends on date" timed-UNTIL MED repro-first; (2) close the three
canary escapes with boundary tests; (3) finish the cross-package resource-is-gone sweep on the remaining
`internal/caldav` write paths. There is no "clean" verdict — the code is not asserted bug-free.

---

## Resolution (2026-07-25)

The MED was fixed repro-first and all three canary escapes closed, same session as the audit. Gate green.

| Item | Fix commit | Resolution |
|------|-----------|------------|
| MED — "Ends on date" drops the end day for timed items | `c6f79f0` | `readCustomRecur` anchors UNTIL at the selected date + the anchor's own wall-clock time-of-day (in `a.loc`): timed series include their end-day occurrence; all-day (midnight) anchors stay midnight → `dateOnlyUntil` truncates unchanged. |
| Canary — `normalizeETag` W/ strip | `e7f3625` | `TestNormalizeETag` pins the `W/"…"` weak-validator rows. |
| Canary — `drillRange` upper bound | `e7f3625` | `TestDrillRangeAtTerminalIndexDoesNotPanic` drives `idx == len(items)` with a valid anchor. |
| Canary — `reconcileReadOnly` dirty-discard | `e7f3625` | `TestReadOnlyDiscardsSyncedThenEditedResource` uses a synced-then-edited (Dirty, Href≠"") resource on a read-only calendar. |

Each canary test was verified to kill its exact mutation (RED under mutation, GREEN reverted).

### Note on the fix layer for the MED

Fixed in the **UI** (`recurcustom.go`), not the model. `spec.Until` is also populated by RRULE
*decomposition* of an existing/foreign rule (`recurdecompose.go`), so bumping UNTIL in the model's
`applyRecurrence` would risk rewriting a foreign rule's exact bound (iron rule). The "Ends on date" field
is unambiguously a date picker, so the whole-day semantics belong at that layer. Using the anchor's own
time-of-day (rather than a fixed end-of-day) keeps it correct across DST/offset and needs no all-day flag.

### Strategic result

This pass was aimed at the carried **cross-package resource-is-gone / bare-write** residual. It largely
**retired** that blind spot: `internal/model` is pure/headless so the concurrency class cannot manifest
there by construction, and `internal/caldav` conditional-write paths held under data-loss fault injection
(touched, no confirmed finding — not exhaustively swept). The one confirmed finding (the timed-UNTIL MED)
was elsewhere and is a functional bug, not data-loss.

### Carried forward

- `internal/caldav` write paths were touched but not exhaustively swept for the resource-is-gone class.
- Recommendation stands: **`more_passes_recommended`** — findings converged (total 3→1, HIGH 1→0), but
  the canary escape rate regressed (3/4), so the test net on the audited surfaces needs the continued
  boundary-test discipline this arc applied.
