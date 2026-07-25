# Pass 21 — read-side recurrence expansion + go-ical heal-set recursion + caldav request-construction

- **Date:** 2026-07-25
- **Prior pass:** Pass 20 (store/sync/UI concurrent-write clobber sweep + post-pass-18 whole-app spec-diff) — HIGH 1 · MED 2 · LOW 2 (all five FIXED 2026-07-25)
- **This pass:** HIGH 1 · MED 2 · LOW 0 (all three CONFIRMED, all UNFIXED; every repro ran RED)
- **Status (2026-07-25): findings CONFIRMED, all three UNFIXED.** The body below is the as-found evidence, not a verdict.

This pass took the ledger's least-audited high-value cells left after pass 20: the **read-side
recurrence expansion** never re-fuzzed since pass 8 against the pass-14/17 multivalue-date inputs, the
**go-ical heal-set spec-diff** the Hard-won guardrail requires re-running (a class that has already
reopened twice), the never-adversarially-tested **caldav request-construction** half of the CalDAV
boundary, and three no-finding confirmations on surfaces the pass-20 fixes touched (the reconcile
**read-only twin** for the step-(A) Forget class, the brand-new **`ForgetIfUnchanged`/`removeLocked`**
race primitives, and the merged **`reanchoredRecurrence` core** matrix).

Six method-diverse audits (fuzz, spec-diff, fault-injection, data-loss, race, input-edge) landed
**three confirmed findings**, each with a repro that ran RED. The headline: **the "heal set must mirror
go-ical's full validateComponent" class reopened a THIRD time** — this pass's sole HIGH — because the
required-prop heals run only on top-level components while go-ical validates recursively. Severity is
trending **down** in raw counts (total 5→3, LOW 2→0), but a HIGH data-brick plus a reopened systemic
class plus one genuine canary escape means the surface is not converged.

---

## Coverage exercised

| Surface | Package | Method | Result |
|---|---|---|---|
| Recurrence expansion READ path (`Occurrences`/`safeBetween`, `resolveDateTimeValues`) vs pass-14/17 multivalue-date inputs | internal/model | fuzz | **1 MED** (aggregate/render-path expansion has no store-wide budget cap) |
| go-ical heal-set (`singleValuedProps` + DTSTAMP/required-prop/mutual-exclusion heals) vs vendored `validateComponent` | internal/model | spec-diff | **1 HIGH** (required-prop heals are top-level-only; go-ical validates recursively — nested phantom bricks the resource) |
| CalDAV request-construction (MKCALENDAR/PROPPATCH/DELETE bodies, resolve/href, name/color validation) | internal/caldav | fault-injection | **1 MED** (PROPPATCH 207 treated as success without per-property status) |
| Sync reconcile READ-ONLY twin (`reconcileReadOnly` Forget branches + `handleWriteForbidden`) for the pass-20 Forget-clobber class | internal/sync | data-loss | **no finding** (read-only collections take no concurrent UI write, so the stale-snapshot lost-update shape can't manifest) |
| Store peer write paths — new pass-20 `ForgetIfUnchanged`/`removeLocked` compare-and-remove core racing a pull/edit | internal/store | race | **no finding** (lock-held core makes compare-and-remove atomic; concurrent swap wins or loses cleanly) |
| Recurrence write-side merged `reanchoredRecurrence(raw, oldAnchor, newAnchor)` core across 1st/2nd/3rd/4th/last/5th × weekly/monthly × event/todo | internal/model | input-edge | **no finding** (todo DUE-anchored twin behaves identically to event DTSTART-anchored; pass-20 merge did not regress pass-19 fixes) |

---

## Confirmed findings (each carries a runnable repro; all ran RED)

### HIGH

**1. Nested-component DTSTAMP/required-prop heal is top-level-only — a phantom nested component bricks
the whole resource** — `internal/model/decode.go:284` (and the `ensureDTStamp` Parse loop, ~89-105).
The Hard-won guardrail promises the heal set mirrors go-ical's **full** `validateComponent`, but
go-ical's `checkComponent` (encoder.go) runs **recursively on every child at any depth**, whereas the
required-prop (DTSTAMP/UID) and mutual-exclusion (DTEND+DURATION / DUE+DURATION /
DURATION-without-DTSTART) heals iterate **only** top-level `cal.Children`: `ensureDTStamp` and
`healComponentConstraints` never walk nested components. `dedupeSingleValued` and `stripForbiddenNesting`
DO recurse (so nested *duplicate* props are healed) — the gap is the required-prop/mutual-exclusion
heals. Compounding it, `stripForbiddenNesting` has no `allowedChildren` entry for VALARM/STANDARD/DAYLIGHT,
so a VEVENT/VTODO/VJOURNAL/VFREEBUSY illegally nested inside one of those is neither stripped nor healed.
- **Failure scenario:** a foreign/hand-edited `.ics` holds a valid, editable top-level VEVENT
  (UID+DTSTAMP+DTSTART) whose VALARM child contains a nested VEVENT with no DTSTAMP. `Decode` succeeds
  and surfaces the real event; `ensureDTStamp` heals only the top level, so the nested phantom survives.
  The moment the user edits the real event, `Parsed.Encode()` recurses into the phantom and returns
  `ical: failed to encode "VEVENT": want exactly one "DTSTAMP" property, got 0` — the save fails and the
  **entire resource, including valid editable siblings, is unwritable**. Same brick for a nested
  VJOURNAL/VFREEBUSY missing DTSTAMP/UID, a nested DTEND+DURATION, or the same components nested under a
  STANDARD/DAYLIGHT in an otherwise-usable VTIMEZONE.
- **Repro (ran RED):** `TestNestedComponentMissingDTStampBricksResource` (full source below). Both the
  VALARM-nested and VTIMEZONE/STANDARD-nested variants produced the DTSTAMP encode failure; the
  nested-*dup* variant is healed (dedupe recurses). The auditor removed the test after running to keep
  the package gate green — **re-add it as the regression guard when the fix lands.**
- **Class:** THIRD reopening of "the heal set must mirror the full `validateComponent`" — pass 10
  (top-level VEVENT/VTODO), pass 16 (VJOURNAL/VFREEBUSY + VTIMEZONE-required-props), pass 21 (**nesting
  recursion depth**). A decode-but-can't-re-encode gap is a HIGH: one bad nested component makes the
  whole resource (incl. valid siblings) unsavable.
- **Fix direction:** make `ensureDTStamp`/`healComponentConstraints` walk nested children the way
  `dedupeComponent`/`sanitizeComponent` already do (heal recursively across ALL components, not just
  top-level), and either add VALARM to the nesting-strip vocabulary or otherwise strip a component
  illegally nested under a VALARM. When it lands, extend the Hard-won guardrail to state the heal walk
  must match go-ical's **recursion depth**, not just its per-component cardinality table.

```go
// internal/model/nested_dtstamp_repro_test.go
package model

import (
	"testing"
	"time"
)

func TestNestedComponentMissingDTStampBricksResource(t *testing.T) {
	const ics = "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//Foreign//EN\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:real-event\r\n" +
		"DTSTAMP:20260101T000000Z\r\n" +
		"DTSTART:20260101T090000Z\r\n" +
		"SUMMARY:Real event\r\n" +
		"BEGIN:VALARM\r\n" +
		"ACTION:DISPLAY\r\n" +
		"TRIGGER:-PT15M\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:phantom-nested\r\n" +
		"DTSTART:20260101T080000Z\r\n" +
		"SUMMARY:Phantom nested event with no DTSTAMP\r\n" +
		"END:VEVENT\r\n" +
		"END:VALARM\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"

	p, err := Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("Decode should succeed and surface the real event, got: %v", err)
	}
	if len(p.Events) != 1 {
		t.Fatalf("expected 1 surfaced top-level event, got %d", len(p.Events))
	}
	if _, err := p.Encode(); err != nil {
		t.Fatalf("re-encode bricked the whole resource (real event now unwritable): %v", err)
	}
}
```

### MED

**2. Recurrence expansion has no aggregate/render-path budget cap** — `internal/model/recurrence.go:102`
(`safeBetween`), driven from `Event.Occurrences` (~226) and `store.EventOccurrencesVisible`.
`safeBetween` caps a **single** event's skip-forward at `maxOccurrenceSteps` (1<<20 ≈ 107 ms wall for a
far-anchored `FREQ=SECONDLY` rule), but nothing caps the **sum** across a resource's events or across
the store: `Event.Occurrences` loops every event in a resource, and `store.EventOccurrencesVisible`
loops that over every resource of every visible calendar — invoked synchronously from `ui/render.go`
`calItems` on each grid rebuild/navigation.
- **Failure scenario:** a `.ics` (or several synced resources) containing N VEVENTs each with `DTSTART`
  far before the view window (e.g. `19000101T000000Z`) and `RRULE:FREQ=SECONDLY`/`MINUTELY`. Each event
  forces `safeBetween` to run the full 1<<20 steps skipping toward the window before giving up
  (~107 ms), and the render path expands all of them synchronously with no aggregate ceiling. Measured:
  50 such events in one resource → `EventOccurrences` over a one-month window took **5.07 s and returned
  0 occurrences** (the events are invisible — the freeze buys nothing); ~600 across the cache would
  freeze each redraw ~60 s. LazyPlanner is offline-first and ingests foreign/hostile `.ics`, so the
  input is attacker-influenceable. The scale guardrail frames the invariant as "a pathological rule
  can't hang the UI" (singular) — the per-event bound holds, the aggregate does not.
- **Repro (ran RED, LEFT IN TREE):** `internal/model/aggcap_repro_test.go`
  (`TestAggregateRecurrenceCapRepro`) — `go test ./internal/model/ -run TestAggregateRecurrenceCapRepro -v`.
  Decodes one `.ics` with 50 far-anchored `FREQ=SECONDLY` VEVENTs, times a one-month `EventOccurrences`,
  asserts ≤500 ms; observed 5.07 s. **This test is currently RED and left in the tree, so it breaks
  `make check`/`go test ./...` until an aggregate cap lands — the owner may want to gate or delete it
  pending the fix.**
- **Fix direction:** an aggregate step/deadline budget shared across the render-path expansion of the
  whole store (a `context` deadline or a summed step ceiling), not just the per-event cap; extend the
  scale-invariant guardrail/benchmark to assert the *aggregate* is bounded.

**3. PROPPATCH treats any 207 Multi-Status as success without inspecting per-property status** —
`internal/caldav/proppatch.go:48`.
`SetCalendarProps` returns `nil` on a 207 Multi-Status **without parsing the body**, so a server that
accepts the PROPPATCH at the HTTP level (207) but rejects the `displayname`/`calendar-color` property
*inside* the 207 (a `<propstat>` carrying `HTTP/1.1 403 Forbidden` or `409`) is treated as a successful
push.
- **Failure scenario:** user renames or recolors a shared/limited-privilege calendar. The server
  returns 207 with a `<propstat>` reporting 403 for `calendar-color`. `proppatch.go:48` sees
  `StatusCode==207` and returns nil; `sync.go:225-233` then calls `MarkCalendarPropsSynced`, which
  clears `pendingName`/`pendingColor` because the pushed value equals the local value. The edit is now
  marked synced but was never applied server-side, and no later sync retries it — silent divergence.
  Contrast `discoverColors`, which DOES walk propstats.
- **Repro (ran RED):** `TestSetCalendarPropsRejectedPropertyIsAnError` (full source below) — an httptest
  server responds 207 with a `<propstat>` 403 for `calendar-color`; `SetCalendarProps` returned nil
  instead of an error. The auditor removed the test after running to keep the package gate green —
  re-add it as the regression guard when the fix lands.
- **Fix direction:** parse the 207 body and surface a non-2xx `<propstat>` status for a requested
  property as an error, mirroring `discoverColors`.

```go
// internal/caldav/proppatch_repro_test.go
package caldav

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSetCalendarPropsRejectedPropertyIsAnError(t *testing.T) {
	const body = `<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:" xmlns:x="http://apple.com/ns/ical/">
  <d:response>
    <d:href>/dav/cal/shared/</d:href>
    <d:propstat>
      <d:prop><x:calendar-color/></d:prop>
      <d:status>HTTP/1.1 403 Forbidden</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", `application/xml; charset="utf-8"`)
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c, err := NewClient(Config{Endpoint: srv.URL, Username: "u", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	err = c.SetCalendarProps(context.Background(), "/dav/cal/shared/", "", "#ff8800")
	if err == nil {
		t.Fatal("expected an error when the server rejects the color property with 403, got nil (edit silently dropped)")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("expected the 403 rejection surfaced, got %v", err)
	}
}
```

---

## Surfaces swept with no finding

- **Sync reconcile READ-ONLY twin** (`reconcileReadOnly`'s `!onServer` Forget, its dirty-discard
  Forget, and `handleWriteForbidden`'s Forget) — the textbook "mirror the pass-20 step-(A) guard onto
  the sibling" escape. No finding: the read-only twin's removals sit behind a *confirmed* server
  deletion on a read-only collection, and read-only calendars are never written by the UI, so no
  concurrent local edit can land during the network I/O — the stale-snapshot lost-update shape that bit
  the read-write step-(A) cannot manifest here.
- **Store `ForgetIfUnchanged`/`removeLocked` compare-and-remove core** (added pass 20, after that pass's
  store `-race` sweep, so never race-stressed) — no race or lost-update finding. The lock-held
  `removeLocked` makes the pointer-identity check and the map removal atomic, so a concurrent
  `stageResourceLocked` swap either wins the whole compare-and-remove or loses it cleanly (Forget
  skipped, edit survives) — no interleave leaves a half-removed entry or a torn tombstone.
- **Merged `reanchoredRecurrence(raw, oldAnchor, newAnchor)` core** across the
  1st/2nd/3rd/4th/last/5th × weekly/monthly × event/todo matrix — no finding. The DUE-anchored todo
  twin behaves identically to the DTSTART-anchored event; the pass-20 merge did not regress the
  pass-19-fixed "last-weekday"/"positive-nth" day-move behavior.

---

## Mutation canaries — 1 of 4 genuinely escaped (OPEN)

The workflow's own summary reported **2** escapes; this synthesis **re-verified both against the
current tree** (applied each mutation, ran the suite, reverted) and found only **1 genuine escape**.
Full detail (with close-out guidance) is in `COVERAGE.md` → "Escaped mutation canaries — pass 21".

- **ESCAPE (OPEN, verified)** — `internal/caldav/object.go` `PutObject` (line 87): dropping
  `http.StatusOK` from the accepted-success set. Re-verified this synthesis: mutation applied, `go test
  ./internal/caldav/` still `ok`. A PUT answered **200 OK** (RFC-legal, returned by some real CalDAV
  servers) would be treated as a write failure — spurious error, edit left dirty/conflicted though the
  write succeeded. The line's comment documents 200 as valid; the success-path tests cover only 201 and
  204. Close with a test issuing a PUT that responds 200 OK and asserting `PutObject` returns
  `(etag, nil)`.
- **FALSE ESCAPE → actually CAUGHT (verified this synthesis)** — `internal/model/quickadd.go`
  `parseTimeHalf` (line 551): widening the 24-hour ceiling `h > 23` → `h > 24`. The workflow reported an
  escape, but the pass-20 guard `TestParseTimeHalfHourCeiling` (`internal/model/parsetimehalf_test.go`,
  closed `d81d432`) pins `24:00` rejected. Applying the mutation on the current tree and running the
  test yields `parseTimeHalf("24:00") ok = true, want false` — **FAIL**. The workflow's ESCAPE was a
  stale-worktree artifact (the worktree predated `d81d432`). No action needed.
- **CAUGHT** — `internal/sync/sync.go` reconcileCalendar step-(A) both-sides-changed conflict guard
  (line 427): flipping `serverObj.ETag != r.ETag` → `== r.ETag` → `go test ./internal/sync/` FAILED
  across 7 tests (`TestSyncConflictKeepsBoth`, `TestSyncPushesLocalEdit`,
  `TestSyncPushDoesNotClobberConcurrentEdit`, `TestSyncUnparseableServerConflictNotTreatedAsDeletion`,
  `TestSyncRefetchesOn412`, `TestReproPullBatchClobbersConcurrentEditToOrphan`,
  `TestUndoOfSyncedEditSurvivesNextSync`).
- **CAUGHT** — `internal/store/mutate.go` `remove()` tombstone guard (`r.Href != ""` → `r.Href == ""`)
  → `go test ./internal/store/` FAILED across 7 tests (`TestDeleteSyncedResourceLeavesTombstone`,
  `TestDeleteNeverSyncedLeavesNoTombstone`, `TestRestoreClearsTombstone`,
  `TestHasPendingChanges/tombstone_is_pending`, plus three CommitPush mid-push-delete invariants).

---

## Convergence & recommendation

- **Severity trend (raw counts):** DOWN — HIGH 1→1, MED 2→2, LOW 2→0; total 5→3.
- **But not converged:** a HIGH data-brick was confirmed, the "heal set must mirror the full
  `validateComponent`" class reopened a THIRD time (this time at recursion depth), one canary genuinely
  escaped (`PutObject` 200-OK success path unguarded), and a render-path DoS (aggregate recurrence
  expansion) is UNFIXED with its repro currently RED in the tree.
- **Recommendation:** `more_passes_recommended`. Fix arc: all three findings repro-first (one commit
  each, full gate every commit) — the nested-heal HIGH recursively across all components + the VALARM
  nesting gap, the aggregate recurrence budget, and the PROPPATCH per-property status; then extend the
  heal guardrail to require matching go-ical's recursion depth (not just its cardinality table) and the
  scale guardrail to bound the *aggregate*, close the `PutObject` 200-OK canary escape with a boundary
  test verified RED, and **remove/gate the left-in-tree `aggcap_repro_test.go` so `make check` is green
  again**. Still-un-swept for the reopened classes: `internal/model` and `internal/caldav` peer write
  paths for the concurrent-delete-signal/bare-write class (this pass covered only the sync/store side),
  and the never-audited full `sync-collection` incremental once implemented.

---

## Residual risk (what remains unknown / uncovered)

- **Heal-set recursion gap is broad:** the confirmed HIGH is one instance (VALARM-nested VEVENT), but
  every required-prop/mutual-exclusion heal is top-level-only, so nested VJOURNAL/VFREEBUSY, nested
  DTEND+DURATION, and components nested under STANDARD/DAYLIGHT are all latent bricks until the heal walk
  is made recursive — the fix must be systemic, not a single-case patch.
- **Render-path recurrence DoS is live and unfixed** — a foreign `.ics` with many far-anchored
  high-frequency events freezes each redraw with no aggregate cap; the guarding repro is RED in the tree
  and will keep `make check` failing until the fix or a gate lands.
- **Cross-package resource-is-gone sweep still incomplete** (carried from pass 20): `internal/model` and
  `internal/caldav` peer write paths were not audited for the bare-write/concurrent-delete-signal class.
- **`PutObject` 200-OK success path unguarded** until the escaped canary is closed.
- **Not re-covered this pass** (recent, deliberately deferred): UI input handlers / keys / chords /
  mouse / draw paths (14/18); quick-add grammar, timezone/DST, Windows→IANA, color parsing, subtask-tree
  build (17/19); multi-account config, global/local state, CLI wiring, `:account`/`:config` flows,
  import ingest (16/17/18); a fresh full whole-app conformance spec-diff (only the narrow encoder-heal
  spec-diff ran this pass); store write-pipeline atomicity under disk fault and background-sync goroutine
  timing (15/16).
- **Permanently accepted / deferred blind spots** (unchanged): Raspberry Pi on real hardware; full
  `sync-collection` incremental (feature deferral); the pass-15 import UID-bearing/UID-less MED
  (owner-accepted residual); center agenda-board click-to-select (low-impact UI follow-up).
