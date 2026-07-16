# Pass 13 — spec-diff reopens Locate→Put + reconcile degraded-download data loss + CalDAV request-side idempotency

- **Date:** 2026-07-16
- **Prior pass:** Pass 12 (Locate→Put systemic follow-up + undo replay + color/name PROPFIND) — HIGH 2 · MED 5 · LOW 0
- **This pass:** HIGH 1 · MED 4 · LOW 0 (all 5 confirmed with runnable failing-test repros executed and observed red)
- **Status (2026-07-16): ALL RESOLVED.** All 5 confirmed findings and all 4 escaped
  mutation-canary holes are now fixed — each with an adversarially-verified regression
  test and its own commit (see `log.md` 2026-07-16 entries). HIGH #1 (degraded-download
  false deletion) → `downloadResilient` returns the listed-but-unfetched set, both
  reconcile paths skip it. MED #2/#3 (Locate→Put reopened) → `applyMutation` and
  `reparentSelected` route through `store.PutIfUnchanged`. MED #4/#5 → MKCALENDAR/DELETE
  idempotency. The store-level clobber repros were promoted to guarded regression tests
  (`internal/ui/editclobber_test.go`, `internal/store/reparent_noclobber_test.go`).
- **At audit time:** NONE fixed. Every repro was written, run, and observed to fail
  against the then-current code (the record below is the point-in-time audit evidence).

This pass took the pass-11/12-named convergence target head-on — a whole-app **spec-diff**
against `main.md`/`CLAUDE.md`, focused on the invariants passes 11/12 introduced — and
paired it with two never/stale surfaces the ledger flagged: the **sync reconcile state
machine** case matrix (data-loss) and the **CalDAV request-construction side**
(MKCALENDAR/PROPPATCH/DELETE bodies + idempotency), plus fuzz on `internal/config` and an
input-edge sweep of the `:` command dispatch beyond `:calendar`, and a `-race` look at the
store write primitives.

The headline: the spec-diff **disproved** pass 12's "the Locate→Put class is structurally
closed / no known clobber sites remain" claim — two unconverted sites survived the earlier
spot check, one of them the app's longest-lived clobber window (the edit form).

---

## Coverage exercised

| Surface | Package | Method | Result |
|---|---|---|---|
| Feature-promise conformance (spec-diff on pass-11/12 invariants) | (whole app) | spec-diff | 2 MED (Locate→Put reopened) |
| Sync reconcile state machine (case matrix, keep-both, Forget branches) | internal/sync | data-loss | 1 HIGH |
| CalDAV request-construction (MKCALENDAR/PROPPATCH/DELETE, idempotency) | internal/caldav | fault-injection | 2 MED |
| internal/config TOML parse / first-run round-trip / password_command stdout | internal/config | fuzz | no finding (canary hole below) |
| `:` command dispatch beyond `:calendar` (`:goto`/`:view`/`:search`/`:config`/`:conflicts`/`:q`) | internal/ui | input-edge | no finding |
| Store write primitives (PutIfUnchanged/RestoreDirty) under concurrent access | internal/store | race | no finding |

---

## Confirmed findings (each carries a failing-test repro that was run and observed red)

### HIGH

1. **Degraded (resilient-fallback) download makes reconcile treat an unfetchable-but-existing
   server resource as a remote deletion** — `internal/sync/sync.go:394` (read-only twin at 499-504).
   `reconcileCalendar`/`reconcileReadOnly` infer "deleted on the server" purely from absence
   in `serverByHref`. But when the bulk `DownloadAll` fails and `downloadResilient` falls back
   to per-resource `GetObject` (sync.go:308-312), any resource whose individual fetch fails is
   simply omitted from `serverObjs` — it still exists on the server. reconcile then treats it
   as remotely deleted: a **clean** local resource is `Forget`ted and counted as
   `PulledDeletes` (no user-facing signal it is actually still on the server); a **dirty** one
   is flagged with a false `ServerDeleted=true` conflict whose keep-server discards the local
   edit. The full server href set is available (`ListObjectHrefs` succeeded to drive the
   fallback), but reconcile is never told which hrefs merely failed to fetch vs are truly gone.
   Repro: `internal/sync/degraded_deletion_repro_test.go`
   (`TestDegradedDownloadNotTreatedAsDeletion`), ran red — `PulledDeletes = 1, want 0`;
   `bad.ics` Forgotten from cache while still present on the server. The existing
   `TestSyncDownloadFallbackSkipsBadResource` misses this because it starts from an empty
   store, so the unfetchable resource is never in the cache to be Forgotten.

### MED

2. **Edit-form and recurrence-scoped saves still use plain `Put` — an unconverted Locate→Put
   clobber site the spec claims is closed** — `internal/ui/edit.go:908` (`applyMutation`).
   `applyMutation` is the shared tail of every form Save and the `commitMutation`/
   `commitMutationKeepingDrill` callers, committing a Locate→edit→`store.Put` with no version
   check. A background sync pull (`triggerSync` does not check `modalOpen`) landing while the
   form is open replaces `cs.resources[name]` with the server copy; on Save, `Put` adopts the
   pulled ETag/Href but persists the stale form content marked Dirty → the next push's CAS
   matches and the remote edit is lost with no conflict. Directly contradicts `main.md:270`
   and `COVERAGE.md`'s "No known Locate→Put clobber sites remain." Same path backs
   edit-this-occurrence (`recur_edit.go:116`), delete-this-occurrence event
   (`recur_edit.go:232`), delete-this-&-future (`recur_edit.go:215`).
   Repro: `internal/store/editclobber_repro_test.go`
   (`TestEditFormDoesNotClobberConcurrentPull`), ran red — after Save: `ETag="etag-v2"
   Dirty=true Description="original note"`; the remote edit was silently overwritten.

3. **`reparentSelected` (`H`/`L`) commits a Locate→edit→Put with plain `Put`, no version
   check** — `internal/ui/edit.go:525`. `reparentSelected` does Locate → `model.SetTodoParent`
   → `store.Put`, passing `loc.Prev` only to `pushUndo`, never as a `PutIfUnchanged` baseline.
   Same TOCTOU clobber as #2: a background pull for the selected task landing before `H`/`L`
   makes the reparent adopt the pulled ETag onto stale content and the next push overwrites
   the concurrent remote edit.
   Repro: `internal/store/reparent_noclobber_repro_test.go`
   (`TestReparentDoesNotClobberConcurrentPull_REPRO`), ran red — after reparent:
   `ETag="etag-v2" Dirty=true Summary="Original summary"`; the pulled `SERVER EDITED THE
   TITLE` was lost.

4. **DELETE is non-idempotent: a 404/410 wedges the calendar as permanently pending-delete**
   — `internal/caldav/mkcalendar.go:94`. `DeleteCalendar` treats every status other than
   204/200 as failure, so a DELETE that already succeeded server-side but whose response was
   lost (connection reset / proxy timeout) — or a calendar deleted from the NextCloud web UI —
   returns an error. `pushCalendarDeletes` (`internal/sync/sync.go:206`) then records a skip
   and does NOT call `RemoveCalendarLocal`, so the calendar stays present locally, still
   pending-delete; every later sync re-issues DELETE, gets 404, errors again — the phantom
   calendar never disappears and each sync emits a spurious failure. RFC-wise a 404/410 on
   DELETE means the desired end-state is already reached.
   Repro: `internal/caldav/mkcalendar_test.go`
   (`TestDeleteCalendarAlreadyGoneIsIdempotent`), ran red — `DeleteCalendar` returns the 404
   and 410 as errors; want nil.

5. **MKCALENDAR is non-idempotent: a lost 201 leaves the calendar stuck in pending-create, and
   the retry's 405 is a permanent error** — `internal/caldav/mkcalendar.go:69`.
   `CreateCalendar` accepts only 201; when a create succeeds server-side but the 201 is lost,
   `pending_create` is never cleared (only `MarkCalendarSynced` clears it, and only on a 201).
   Discovery adopts the collection onto the same local id and sets `Href`, but leaves
   `pending_create=true`; every later sync retries MKCALENDAR against the existing collection,
   gets 405, and `recordSkip`s — a permanent recurring sync error.
   Repro: `internal/sync/mkcalendar_lost201_repro_test.go`
   (`TestMKCalendarLost201LeavesPermanentPendingCreate`), ran red — after discovery adopts the
   Href, `PendingCreate=true`; the second sync retries MKCALENDAR (`createCalls=2`) and records
   a 405 skip. **Repro caveat:** the finding's sub-claim that `pending_create` keeps
   `HasLocalChanges` true / defeats the CTag short-circuit was checked and is **inaccurate** —
   `Store.HasLocalChanges` does not inspect `pendingCreate` (only `HasPendingChanges` does).
   The primary defect (permanent recurring MKCALENDAR/405) is real and reproduced.

---

## Mutation-canary results — 4 of 4 escaped (all test-coverage holes)

Canaries probe the *test net*; an escape means the code is correct today but a future
regression on that exact path would ship silently. All four have suggested boundary tests in
`COVERAGE.md`.

- **ESCAPE** — `internal/model/timegrid.go` `LayoutDay` (cluster-flush ~line 59): flipping
  `!start.Before(clusterEnd)` → `start.After(clusterEnd)` (`<=` vs `<`) — `go test
  ./internal/model/` PASSED. Touching/back-to-back occurrences fold into the prior overlap
  cluster and inflate a standalone block's `Lanes` (drawn too narrow). The property check
  asserts overlaps never share a lane but never that lane counts stay *minimal* at a touch.
- **ESCAPE** — `internal/sync/sync.go` `Sync` (CTag-cache guard ~line 186): `len(res.Skipped)
  == skipsBefore` → `>=` — `go test ./internal/sync/` PASSED. `>=` is always true (Skipped
  only grows), collapsing the guard to always-cache; a calendar with a per-resource failure
  this run caches its CTag anyway and next sync's short-circuit never retries the failed
  resource. No two-sync test asserts the CTag is NOT cached after a failed reconcile.
- **ESCAPE** — `internal/caldav/object.go` `DeleteObject`: dropping the `else` that sets
  `If-Match: *` when no stored ETag is provided — `go test ./internal/caldav/` PASSED. Turns a
  safe conditional delete into a blind unconditional DELETE. `TestDeleteObjectNotFoundIsSuccess`
  exercises the empty-ETag path but never inspects the outgoing `If-Match` header.
- **ESCAPE** — `internal/config/config.go` `Load` (~line 136): `io.ReadAll(io.LimitReader(f,
  maxConfigBytes))` → `io.ReadAll(f)` — `go test ./internal/config/` PASSED. Drops the 4 MiB
  read cap (memory-exhaustion / endless-file guard). No test feeds an oversized config; the
  removed guard leaves `maxConfigBytes` unused (staticcheck would flag it, but the test gate
  doesn't).

---

## Convergence

| Severity | Pass 12 | Pass 13 | Trend |
|---|---|---|---|
| HIGH | 2 | 1 | ↓ |
| MED  | 5 | 4 | ↓ |
| LOW  | 0 | 0 | — |
| **Total** | **7** | **5** | ↓ |

Severity is trending **down** on every axis (HIGH 2→1, MED 5→4, total 7→5). But the drop is
modest and the shape is not reassuring: the two MED findings are the *reopening* of a class
(Locate→Put) pass 12 declared closed, and the HIGH is a genuinely new data-loss cell in a
surface (reconcile) that keeps yielding data-loss across passes 3/11/12/13. New surfaces
(CalDAV request-construction) still turned up two MED on first contact. **Not converged.**

---

## Residual risk

- **5 confirmed findings remain UNFIXED**, all with red repros. The HIGH (#1) is silent data
  loss on a realistic path: a transient per-resource fetch failure during a degraded download
  deletes a clean local item that still exists on the server (or drops a local edit via a
  false ServerDeleted conflict).
- **The Locate→Put class is NOT closed** (findings #2, #3) — pass 12's "structurally closed"
  claim was based on a spot check, not a sweep. Two sites remain on plain `store.Put`,
  including `applyMutation` (edit form + all recurrence-scoped saves), the app's longest
  clobber window. `store.PutIfUnchanged` exists; routing both through it is the direct fix.
  The `main.md`/`COVERAGE.md` invariant text is currently false and must be corrected.
- **CalDAV request-side idempotency** (#4, #5): DELETE and MKCALENDAR treat already-gone /
  already-exists as hard errors, wedging a calendar in pending-delete/pending-create forever
  with a recurring spurious sync failure. Fix: treat 404/410 on DELETE and 405/collision after
  a create as the desired end-state reached.
- **4 escaped canaries** are live test-coverage holes (LayoutDay lane-minimality, CTag-cache-
  after-skip, DeleteObject `If-Match: *`, config read cap) — all correct today, all unguarded.
- **No finding but only partial depth** this pass: `internal/config` fuzz, the `:` command
  dispatch input-edge, and the store-primitive `-race` look each produced no finding, but the
  config canary escape shows the config *test net* has a hole even where the code is sound.
- **Still never/deferred:** full `sync-collection` token-delta sync (deliberately unbuilt);
  the Raspberry Pi hardware target (on-device timing, kiosk/autologin, bare-TTY color) — needs
  a physical Pi. Timezone/DST (pass 8), UI draw/display widgets (pass 6), and the bulk of
  non-command key/chord input-edge coverage remain stale-but-untouched.

**Recommendation: more passes recommended** — one HIGH and four MED confirmed, all four
mutation canaries escaped, a reopened systemic Locate→Put class, and a data-loss cell in the
reconcile matrix all remain unfixed. Fix the reconcile degraded-download deletion inference
(distinguish a failed fetch from a genuine server absence), route `applyMutation` and
`reparentSelected` through `PutIfUnchanged`, make DELETE/MKCALENDAR idempotent, and close the
four canary holes — then correct the "Locate→Put closed" invariant text and re-run the sync
reconcile matrix for the remaining cells.
