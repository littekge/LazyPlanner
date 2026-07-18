# Pass 15 — CalDAV write-redirect silent success + load-time stale-temp sweep eats real resources + import UID-less-sibling drop

- **Date:** 2026-07-18
- **Prior pass:** Pass 14 (reconcile 412 tombstone drop + recurrence write-side split defects + multi-valued RDATE/EXDATE collapse + quick-add day-range) — HIGH 0 · MED 5 · LOW 1
- **This pass:** HIGH 2 · MED 1 · LOW 0 (all 3 confirmed with runnable failing-test repros executed and observed red)
- **Status (2026-07-18): both HIGH fixed + the escaped canary closed; the MED accepted as a
  documented residual (owner decision).** The two HIGH were fixed repro-first (redirect →
  method-aware `CheckRedirect` + 3xx-as-error; stale-temp → require the `.tmp-<digits>` shape),
  the canary is guarded (`TestListObjectHrefsExcludesNestedCollection`), and the MED (import
  drops a valid sibling of a UID-less component) was **not** fixed — every fix crosses a hard
  invariant (fabricate-UID reverses a settled decision; per-component encode weakens the iron
  rule; the transport hands us an already-decoded calendar, so no raw bytes survive to
  preserve). It is reachable only from a malformed foreign/hand-edited `.ics` and is surfaced,
  not silent. See `log.md` (2026-07-18) and `COVERAGE.md`. The findings below are the
  point-in-time as-found evidence.

This pass took the ledger's top stale/never CalDAV + store + sync cells head-on, the matrix
cells `main.md`'s convergence paragraph names as the remaining work: the **CalDAV
response-parse** path (fault-injection, stale since pass 7; passes 12/13 only touched
color/name decode and request-construction), a **fuzz** of the hand-rolled `ListObjectHrefs`
XML decode boundary, a first-ever direct **-race** stress of the **store write primitives**
(previously exercised only via the sync engine), a **data-loss** re-sweep of the reconcile
**keep-both / Forget / read-only-twin** branches, a **fuzz** of the never-audited **import
ingest path**, and a **fault-injection** re-sweep of the **store disk-fault atomicity**
sidecar/delete-half.

The headline: two HIGH silent-data-loss defects surfaced outside `internal/model` for the
first time in several passes — a CalDAV **write** that follows an HTTP redirect and reports
success while the write never lands, and a load-time **stale-temp sweep** that deletes
legitimately-named `.ics` resources on Open.

---

## Coverage exercised

| Surface | Package | Method | Result |
|---|---|---|---|
| CalDAV response-parse (multiget/PROPFIND/REPORT decode, truncated/oversized bodies, unexpected status, **write redirects**) | internal/caldav | fault-injection | 1 HIGH (write-redirect silent success) |
| CalDAV XML decode boundary (`ListObjectHrefs` hand-written parse, ETag/href/resourcetype extraction) | internal/caldav | fuzz | no finding (canary hole below) |
| Store write primitives racing PullRemoteBatch (mutate / PutIfUnchanged / RestoreDirty / tombstone) | internal/store | race | no race/deadlock; 1 HIGH found separately at load (stale-temp sweep) |
| Sync reconcile keep-both / Forget / read-only-twin branches | internal/sync, internal/store | data-loss | no new finding (canaries caught) |
| Import ingest path (DownloadAll batch + per-resource GetObject fallback, ImportError) | internal/sync, cmd/lazyplanner | fuzz | 1 MED (UID-less sibling drops the valid component) |
| Store write-pipeline disk-fault atomicity (.ics + sidecar temp/rename, delete-half) | internal/store | fault-injection | no finding (accepted delete-half residual still degrades safely) |

---

## Confirmed findings (each carries a failing-test repro that was run and observed red)

### HIGH

1. **`PutObject`/`DeleteObject` silently report success when the server or a proxy answers a
   write with a 301/302/303 redirect** — `internal/caldav/object.go:67` (delete at
   `object.go:108`; the redirect-following `http.Client` with no `CheckRedirect` is built at
   `client.go:146`). The shared client uses Go's default redirect policy, which follows 3xx on
   any method and downgrades 301/302/303 to a **bodyless GET**, dropping the request body and
   the `If-Match`/`If-None-Match` conditional headers. A GET to the redirect target returning
   200/204 lands in the success set, so the call returns `(etag, nil)` / `nil` even though the
   resource was never written or deleted. Sync records success and clears the pending/dirty
   flag, so the local edit never reaches the server and never retries — disk diverges from
   server, violating the "sync never silently overwrites/loses" invariant. Reads
   (PROPFIND/REPORT) fail loudly on the same redirect (the resulting non-207 errors), so the
   asymmetry hides the write failure. Triggered by any `http://` endpoint or any reverse proxy
   doing http→https / trailing-slash normalization.
   Repro: `internal/caldav/redirect_repro_test.go`
   (`TestPutObjectRedirectMustNotReportSuccess`, `TestDeleteObjectRedirectMustNotReportSuccess`),
   ran red — the httptest server answers the PUT/DELETE with 301→`/final.ics`, Go re-issues it
   as a GET returning 200+ETag, and `PutObject` returns `("etag-from-get", nil)` /
   `DeleteObject` returns `nil` with no write having occurred (server log: `[PUT href, GET
   /final]`). Fix: a per-request `CheckRedirect` returning `http.ErrUseLastResponse` so a 3xx is
   returned as-is, then treat any 3xx status on a write as an error (a write must land on the
   exact href, never a proxy-chosen `Location`).

2. **Load-time stale-temp sweep deletes legitimate `.ics` resources whose name starts with `.`
   and contains `.tmp-`** — `internal/store/store.go:188` (heuristic at `store.go:215`).
   `loadCalendar`'s stale-temp sweep uses the over-loose `isStaleTempName`
   (`HasPrefix(".") && Contains(".tmp-")`) and runs **before** the `.ics`-extension filter at
   `store.go:195`, so a real resource whose sanitized name begins with a dot and contains
   `.tmp-` is `os.Remove`'d on Open instead of loaded. A VTODO/VEVENT with a UID like
   `.tmp-important@host` sanitizes to `.tmp-important_host.ics` (via `ResourceName`→`SafeName`,
   which preserves leading dots and `.tmp-`), is written by `Put`/`writeFileAtomic` as a real
   cache file, and is then swept on the next `Store.Open`. If it was created offline and not yet
   pushed (`Href==""`) it is **permanently lost** — never on the server, never re-pullable; if
   already synced it churns (deleted every launch, re-pulled every sync). The same name is
   reachable from a foreign/hostile server resource whose href basename starts with `.` and
   contains `.tmp-` (`resourceFileName` in `internal/sync/import.go:132` runs it through
   `SafeName`), so an imported calendar can carry a disappearing item.
   Repro: `internal/store/staletemp_repro_test.go`
   (`TestStaleTempSweepSpareLegitimateResource`), ran red — after writing
   `.tmp-important_host.ics` and reopening, `findResource` returns nil ("resource was deleted on
   Open"). Fix: exclude names ending in `icsExt`/the sidecar name from the stale-temp match,
   and/or reorder so the extension filter wins; the real temp pattern
   `os.CreateTemp(dir, "."+base+".tmp-*")` yields names ending in `.tmp-<digits>` with no real
   extension, so `isStaleTempName` should require that.

### MED

3. **A single imported resource mixing a UID-bearing component with a UID-less one fails to
   encode as a whole, dropping the valid sibling** — `internal/sync/import.go:99` (encode at
   `import.go:112`). Import parses each server resource into one `model.Parsed` and re-encodes
   it as one all-or-nothing `.ics` (`PullRemoteBatch`→`stageResourceLocked`→`Encode`). The
   ingest healers add a missing DTSTAMP/VERSION/PRODID but deliberately never fabricate a UID
   (pass-3 #7), so any component lacking UID makes go-ical's encoder reject the **entire**
   resource (`want exactly one "UID" property, got 0`). Import records the whole resource in
   `res.Skipped`, so a perfectly valid UID-bearing VEVENT sharing the resource with a UID-less
   VTODO is never imported — item-level data loss for a well-formed component due to a malformed
   sibling. Surfaced (not silent) in `res.Skipped`, but the valid item is lost. `FuzzDecode`
   bails its round-trip assertion whenever `allHaveUID` is false, so this
   decode-but-cannot-re-encode case is outside existing fuzz coverage.
   Repro: `internal/sync/mixeduidless_repro_test.go`
   (`TestImportMixedUIDlessSiblingDropsValidComponent`), ran red — `Objects imported = 0`,
   `Skipped = [personal (…/mixed.ics): … failed to encode "VTODO": want exactly one "UID"
   property, got 0]`; the valid VEVENT (UID `good@test`) never lands in the cache. Fix: heal a
   missing UID at ingest, or encode per-component so a bad sibling can't poison the resource.

---

## Mutation-canary results — 1 of 3 escaped (a test-coverage hole)

Canaries probe the *test net*; an escape means the code is correct today but a future
regression on that exact path would ship silently.

- **ESCAPE** — `internal/caldav/listobjects.go` `ListObjectHrefs`: removing the
  `|| r.isCollection()` clause from the member-filter — `go test ./internal/caldav/` PASSED
  (incl. `TestListObjectHrefs`). The fixture (`objectListMultistatusXML`) has exactly one
  collection response whose href equals the queried calendar path, still excluded by the
  surviving path-equality clause, so the count is unchanged. Nothing exercises a **nested**
  sub-collection (a distinct href that is a collection, e.g. `/dav/cal/personal/inbox/`) —
  precisely what `isCollection()` exists to filter. A real regression dropping the check would
  leak nested-collection hrefs into the returned refs; the per-resource download fallback would
  then GET a collection URL as an event object. Suggested guard: a fixture case with a
  nested-collection href ≠ the query path, asserting it is excluded.
- **CAUGHT** — `internal/store/mutate.go` `Store.remove` (line ~292): dropping the
  `r.Href != ""` guard on the tombstone write (`if tombstone && r.Href != ""` → `if
  tombstone`) — `go test ./internal/store/` FAILED, `TestDeleteNeverSyncedLeavesNoTombstone`.
  A never-synced local delete would otherwise leave a tombstone a later sync tries to DELETE
  server-side for a resource that never existed.
- **CAUGHT** — `internal/sync/sync.go` `reconcileCalendar` step B (line ~447): dropping the
  `|| tombstonedHref[o.Path]` guard (`if localByHref[o.Path] || tombstonedHref[o.Path]` →
  `if localByHref[o.Path]`) re-pulls a locally-deleted-but-unpushed resource, resurrecting it —
  `go test ./internal/sync/` FAILED across 5 tests (`TestSyncPushesTombstoneDelete`,
  `TestSyncTombstoneVsServerEditIsConflict`, `TestSyncDeleteTransient403KeepsTombstone`,
  `TestSyncDeleteConfirmedReadOnlyDiscards`, `TestUndoOfSyncedDeleteSurvivesNextSync`).

---

## Convergence

| Severity | Pass 14 | Pass 15 | Trend |
|---|---|---|---|
| HIGH | 0 | 2 | ↑ |
| MED  | 5 | 1 | ↓ |
| LOW  | 1 | 0 | ↓ |
| **Total** | **6** | **3** | ↓ |

**New root-cause class this pass:** no single shared class — the three findings are
independent (HTTP redirect policy on writes; a load-time filename-heuristic ordering bug; an
all-or-nothing per-resource encode with no UID healer). Recorded per `PROTOCOL.md`.

Total count fell (6→3), but by the severity-weighted reading that is **not** the signal:
**HIGH resurged 0→2** after pass 14's first no-HIGH pass, so the two-consecutive-no-HIGH
convergence test resets. Both HIGH are silent data-loss defects that had gone unaudited because
they live at the CalDAV/store boundaries the prior passes deprioritized — evidence that the
remaining stale/never matrix cells still hide real bugs, not just tail MED/LOW. Against the
criteria: the no-HIGH streak is broken (back to zero consecutive), and while no *new* recurring
class appeared, a canary escaped. **Not converged.**

---

## Residual risk

- **3 confirmed findings remain UNFIXED**, all with red repros. Two are HIGH silent data loss:
  (1) any write against an endpoint behind a redirecting proxy is reported as success while
  the edit/delete vanishes and the dirty flag clears — no retry, disk diverges from server;
  (2) a legitimately-named `.ics` resource (dot-prefixed, `.tmp-`-containing UID) is deleted on
  every Open, permanently for an offline-created not-yet-pushed item and reachable from a
  hostile import href. The MED is item-level import loss: a valid component dropped because a
  UID-less sibling poisons the whole-resource encode.
- **The CalDAV write path and the store load path are the pass's weak spots** — both HIGHs are
  at boundaries (HTTP transport policy; on-disk load sweep) that the model-heavy prior passes
  never reached. The fixes are localized (per-request `CheckRedirect` + treat 3xx as a write
  error; tighten `isStaleTempName` / reorder the extension filter; heal-or-per-component encode
  on import) but each needs its own repro-first commit.
- **One escaped canary** (`ListObjectHrefs` nested-collection filter) is a live test-coverage
  hole — correct today, unguarded; a nested sub-collection href would leak as a member resource
  and be GET as an event object if the filter regressed.
- **Repro hygiene / build note:** two proposed regression test files are left in the tree and
  currently RED — `internal/caldav/redirect_repro_test.go` and
  `internal/store/staletemp_repro_test.go` (both verified present). The import repro
  (`internal/sync/mixeduidless_repro_test.go`) was written, run red, and observed, then
  **removed** by the auditor to keep the gate green — it is NOT in the tree; recreate it from
  the source embedded in the finding (uses the existing `fakeSource`/`mustCal`/`findResource`
  helpers in `internal/sync/import_test.go`). Reconcile all three (keep as guards once fixed, or
  delete) before the next gate.
  `go` is not on the default PATH on the audit host (it lives at `/tmp/go/bin/go`); run the
  `go test` lines with `export PATH=$PATH:/tmp/go/bin` first.
- **Not covered this pass / still stale-or-never:** mouse handling and `:config`/$EDITOR reload
  (internal/ui, stale since pass 10); full `sync-collection` token-delta sync (deliberately
  unbuilt); the Raspberry Pi hardware target (needs a physical Pi). The internal/model parsers
  (decode/ingest, recurrence, quick-add, tz, encoder round-trip) are recent from pass 14 and
  were not re-fuzzed.

**Recommendation: more passes recommended** — two HIGH and one MED confirmed (all unfixed),
one escaped canary, and the no-HIGH streak broken. Fix repro-first: give the CalDAV client a
per-request `CheckRedirect` that stops on 3xx and treat any 3xx on a write as an error; tighten
`isStaleTempName` (exclude `.ics`/sidecar names, require the no-extension temp shape) and/or run
the extension filter before the sweep; heal a missing UID at ingest or encode per-component in
import; and add the `ListObjectHrefs` nested-collection canary guard.
