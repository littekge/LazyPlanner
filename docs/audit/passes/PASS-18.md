# Pass 18 — v1.1.0 multi-account surfaces: config-decode startup hang + `:config` account-list not live + a re-swept sync-core mid-push-delete resurrection

- **Date:** 2026-07-21
- **Prior pass:** Pass 17 (Import empty-href data-loss + IANA-TZID VALUE=PERIOD mis-zone; first audits of color.go/windowszones.go) — HIGH 0 · MED 2 · LOW 0 (all fixed)
- **This pass:** HIGH 2 · MED 1 · LOW 0 (all three confirmed; two verified by re-run/repro here, one by code+spec inspection)
- **Status (2026-07-21): findings CONFIRMED, all three UNFIXED.** The body below is the as-found
  evidence. Two of three repros were re-verified during synthesis (one runs red in the tree; one
  reproduced standalone); the third is confirmed by direct code + spec inspection because its repro
  was removed by the auditor.

This pass took the ledger's never-audited cells — the brand-new v1.1.0 multi-account surfaces added
*after* pass 17: the multi-account **config parse** (`[[account]]` schema + migration + ID
derivation), the new **`global.json`** state file, the **`runTUILoop`** switch-and-rebuild state
machine, the **`:account`** command/picker, and a whole-app **feature-promise spec-diff** of the
v1.1.0 promise set — plus the deliberately-deferred heavier surface: a **deep sync-core TOCTOU
re-sweep** (first since pass 11).

The news is bad on two axes. First, **HIGH reopened 0→2** after a HIGH-free pass 17: a config-parse
startup hang that defeats the read cap, and a sync-core mid-push-delete resurrection that silently
loses a user delete. Second, **all four mutation canaries escaped again** — the second consecutive
4/4-escape pass — every one a test-coverage hole on a v1.1.0 or sibling surface.

---

## Coverage exercised

| Surface | Package | Method | Result |
|---|---|---|---|
| Multi-account config parse (`[[account]]` / `[server]` migration / validateAccounts / ResolveActiveAccount / Account.ID) | internal/config | fuzz | **1 HIGH** (O(depth²) nested-inline-table decode hangs startup within the 4 MiB read cap); name-uniqueness / migration / ID logic held |
| Global state file (`global.json` LoadGlobal/SaveGlobal, corrupt→zero, atomic write) | internal/state | fault-injection | no finding (fail-safe + atomic write hold); 1 canary ESCAPE (0o600 mode untested) |
| Switch-and-rebuild loop (`runTUILoop` persist-before-open, fallback, fatal initial-open) | cmd/lazyplanner | fault-injection | no finding on the loop (persist-before-open crash-window residual accepted); 1 canary ESCAPE (`components()` untested) |
| `:account` command + picker | internal/ui | input-edge | surfaced the `:config`-reload account-list staleness (recorded as the feature-promise MED) |
| Feature-promise conformance vs main.md/CLAUDE.md (v1.1.0 promise set) | (whole app) | spec-diff | **1 MED** (`:config` reload doesn't re-parse the account list, contra main.md:340); rest of the promise set held |
| Sync-core deep concurrency / TOCTOU (reconcile vs concurrent edits/pulls) | internal/sync, internal/store | race | **1 HIGH** (`CommitPush` resurrects a resource deleted mid-push and wipes its tombstone) |

---

## Confirmed findings (each carries a runnable repro)

### HIGH

1. **Nested inline-table config decodes in O(depth²), hanging startup within the 4 MiB read cap** —
   `internal/config/config.go:195`. `toml.Decode` is quadratic in nesting depth for inline tables,
   so a config file well under `maxConfigBytes` (4 << 20, config.go:23) makes `Load()` — and thus
   lazyplanner startup — hang for minutes-to-hours with no UI and no error. The read cap is applied
   via `io.LimitReader` (config.go:190) but bounds only *bytes*, not decode CPU, and `Load` wraps
   `toml.Decode` with no timeout. Root cause is in the dependency (vendored BurntSushi/toml v1.6.0
   `parse.go` `valueInlineTable` appends to `p.context` at every nesting level; per-key
   addContext/setValue calls each walk the full chain → O(depth²)). This directly contradicts the
   config.go:21-23 comment ("a huge or endless file can't … hang startup") and the property
   `TestLoadCapsReadSize` claims — that test only exercises a long *comment*, not nested structure,
   giving false confidence. Threat model: a local, user-owned 0600 config, so reachability is a
   corrupted / mis-edited / crafted config rather than remote input; impact when triggered is a full
   silent startup hang.
   **Repro (re-verified during synthesis):** a `map`-target `toml.Decode` of `"x = " +
   strings.Repeat("{a=",n) + "1" + strings.Repeat("}",n)` measured depth=2000 (8 KB)→232 ms,
   depth=4000 (16 KB)→840 ms, depth=8000 (32 KB)→3.36 s — quadratic (≈4× per size doubling), all
   `err=nil` (pure CPU blowup, not an early parse error). A 32 KB file is already far under the 4 MiB
   cap; extrapolating, ~256 KB ≈ minutes and ~1 MB ≈ >1 h, all within the byte cap. The auditor's
   original standalone `TestNestedInlineTableDecodeScaling` was removed (would break the gate before
   a fix); re-add a **bounded-time** variant as the regression guard once the fix lands.
   Fix direction: bound the decode structurally — cap nesting depth or decode under a deadline —
   rather than trusting the byte cap.

2. **`CommitPush` resurrects a resource deleted mid-push and wipes its tombstone (delete silently
   lost)** — `internal/store/remote.go:83`. `CommitPush`'s build closure treats `cur==nil` (resource
   deleted concurrently during the network PUT) identically to `cur==pushed`, re-creating the
   deleted resource as clean; `stageResourceLocked` (mutate.go:161) then unconditionally
   `delete(cs.tombstones,name)`. So a user delete that lands while an edit-PUT is in flight is
   silently lost: item reappears locally clean, tombstone gone, server keeps the (edited) resource,
   next sync is a no-op. Sequence: synced resource N (ETag srv-1) edited → Dirty; sync goroutine's
   `pushUpdate` issues `PutObject` (store lock NOT held); user presses delete on the event-loop
   (`store.Delete` removes N, leaves tombstone{Href,ETag=srv-1}); PUT returns new-1; sync calls
   `CommitPush(N,pushed,new-1,href)`; under the lock `cur==nil`, hits the `cur==nil || cur==pushed`
   branch, returns clean `Resource{Object:pushed.Object,ETag:new-1,Dirty:false}`. The never-pushed
   `pushCreate` variant revives the item with the server identity the same way. The pointer-identity
   concurrency signal has no "the resource is gone" case — only "same pointer" vs "replaced by an
   edit".
   **Repro (ran red during synthesis):** `internal/store/commitpush_deletemidpush_test.go`
   (`TestCommitPushDoesNotResurrectDeletedResource`, left in the tree, untracked). Seeds synced N
   (srv-1), captures the `pushed` snapshot, `s.Delete` mid-push (leaves the tombstone), then
   `s.CommitPush(...,"srv-2",href)`. Both assertions fire: "wiped the tombstone … got 0, want 1
   (deletion silently lost)" and "resurrected the deleted resource synced_test.ics (Dirty=false,
   ETag=srv-2)". Deterministic (no goroutine scheduling needed — the `cur==nil` branch is reached
   whenever the resource was deleted before CommitPush ran, exactly the mid-flight window).
   Fix direction: in CommitPush, when `cur==nil` AND a tombstone exists for `name`, do not resurrect
   — keep the deletion and its tombstone, advancing the tombstone ETag to the new server etag so the
   pending DELETE stays conditional. The `pushCreate` variant needs the same guard.

### MED

3. **`:config` does not re-parse the account list live — new/renamed accounts invisible and
   unswitchable until restart** — `internal/ui/command.go:195`. main.md:340 promises a `:config`
   reload "re-parses the account list (picker/status bar update live)", but the reloaded list is
   discarded: `ConfigReload` (ui/app.go:324-332) carries only Sync/ColorMode/Warning; `editConfigFn`
   (cmd/lazyplanner/main.go) reads `cfg.Accounts` only to match the running account by ID and never
   returns the refreshed names; `applyConfigReload` (command.go:195-219) never updates `a.accounts`
   or `a.activeAccount`. Those two fields are set once in `Run()` (app.go:351-352) and are what the
   picker (command.go:157), the switch validator (command.go:112), and the status bar (render.go:665)
   read. So a user who `:config`-adds a third `[[account]]` (or renames one) sees the picker and
   status bar unchanged, and `:account <newname>` flashes "unknown account" — the new account is both
   invisible and unreachable until restart, contradicting the "update live" promise.
   **Repro (confirmed; file removed by auditor):** the auditor's Go test drove the real
   `applyConfigReload(ConfigReload{},nil)` path — `a.accounts` stayed `[personal b]` (added "work"
   absent) and `a.switchAccount("work")` left `switchTo` empty and flashed "unknown account: work".
   Separately, a `ConfigReload{Accounts: …}` literal failed to compile ("unknown field Accounts"),
   proving the struct carries no account list. **Verified during synthesis by direct inspection**:
   `ConfigReload` has exactly `{Sync, ColorMode, Warning}` (app.go:324-332), `applyConfigReload`
   touches only `a.syncFn`/`a.colorMode`/Warning, and main.md:340 carries the "update live" promise
   verbatim. Repro was removed (would fail the gate pre-fix); re-add as the regression guard with the
   fix.
   Fix direction: add Accounts (and ActiveAccount) fields to `ConfigReload`, have `applyConfigReload`
   refresh `a.accounts`/`a.activeAccount` and rebuild the picker.

---

## Surfaces swept with no finding

- **Global state file** (`internal/state/global.go`, fault-injection, first audit) — the
  corrupt/missing→zero fail-safe and the atomic temp+rename hold under injected corrupt / partial /
  oversized state and write faults; a bad `global.json` never blocks startup or strands the
  active-account memory. (A *test-net* hole in `Save`'s 0o600 mode surfaced as a canary escape.)
- **Switch-and-rebuild loop** (`cmd/lazyplanner` `runTUILoop`, fault-injection, first audit) —
  injecting `store.Open` failures exercised the previous-account fallback and the second-failure /
  initial-open fatal paths correctly; the documented persist-active-id-before-open crash window
  (sub-millisecond) is an accepted residual. No finding on the loop itself.
- **`:account` command + picker** (`internal/ui`, input-edge) — adversarial names (whitespace,
  active-name case variants, empty, case-only collisions) and switch-while-modal / pending-flush
  states surfaced no *new* command-handler defect; the input-edge is what exposed the `:config`
  account-list staleness (finding 3), which is a reload-path gap rather than a command-handler bug.
- **v1.1.0 feature-promise spec-diff** (whole app) — apart from finding 3, the promise set held:
  the teardown-and-rebuild GC's the old app (`store.Open` holds no OS handles/locks) so there is no
  cross-account pointer leak; cache carry-over rides the unchanged `Account.ID` derivation;
  last-active-by-id survives a rename via `ResolveActiveAccount`'s first-block fallback; and `:config`
  never yanks a live store (it keeps the in-memory connection and flashes a switch/restart advisory).

---

## Mutation-canary results — 4 of 4 escaped (all OPEN test-coverage holes)

Canaries probe the *test net*; an escape means the code is correct today but a plausible future
regression on that exact path would ship silently. None fixed this pass.

- **ESCAPE** — `internal/config/config.go` `permissionWarning()`: narrowing the loose-permission
  mask `mode.Perm()&0o077` → `&0o007` (dropping group bits) passed. `TestLoadWarnsOnLoosePermissions`
  uses 0o644, still tripping the narrowed mask via the other-readable bit, so a **group-readable-only**
  config (0o640) silently stops warning. Guard: table the warning across 0o600/0o640/0o604/0o644.
- **ESCAPE** — `internal/state/state.go` `Save()`: flipping the state-file mode 0o600 → 0o644
  (world-readable) passed. No test asserts the written `FileMode` (round-trip / temp-file / read-cap
  / JSON-error tests only); the 0o600 contract (and the 0o700 MkdirAll dir mode) is unguarded. Guard:
  `os.Stat`/`FileMode()` assertion after Save.
- **ESCAPE** — `cmd/lazyplanner/calendar.go` `components()`: flipping the `--tasks` branch
  `[]string{"VTODO"}` → `[]string{"VEVENT"}` passed. `components()`/`slugify()`/`joinWarnings()` have
  zero test coverage; the MKCALENDAR supported-component-set (events↔VEVENT / tasks↔VTODO) is
  unguarded — a swap or a broken `--both` ships silently. (The conn.go credential guard IS pinned by
  the pass-16 canary.) Guard: table `components(tasks,both)` over --tasks / --both / neither.
- **ESCAPE** — `internal/ui/mouse.go` `treeNodeAtY` (~line 101): off-by-one `if idx >= len(visible)`
  → `if idx > len(visible)` passed. A double-click one row past the last visible tree node would
  index `visible[len]` → panic the whole TUI; the failing boundary (`idx==len(visible)`) is never
  exercised (existing double-click tests hit valid in-range rows). Guard: double-click the row one
  past the last node, assert no panic / nil target.

---

## Convergence

| Severity | Pass 17 | Pass 18 | Trend |
|---|---|---|---|
| HIGH | 0 | 2 | ↑ |
| MED  | 2 | 1 | ↓ |
| LOW  | 0 | 0 | → |
| **Total** | **2** | **3** | ↑ |

Severity is **trending UP, not down.** After pass 17 recorded the first HIGH-free re-sweep, **HIGH
reopened 0→2** and total ticked 2→3. That is expected in direction — this pass deliberately opened
brand-new surface (the entire v1.1.0 multi-account feature stack, never before audited) plus the
long-deferred deep sync-core TOCTOU — but it means the readiness trend the recent streak was
building is **broken, not extended**: new feature code carries new HIGH defects, and one HIGH is a
*reopening* of the oldest heavy surface (sync-core concurrency, last deep-audited pass 11). The
canary signal is equally telling: **4/4 escaped for the second pass running**, all on v1.1.0 /
sibling code — the test net simply hasn't been extended over the new feature yet.

---

## Residual risk

- **2 HIGH + 1 MED confirmed, all THREE UNFIXED.** (1) A crafted/corrupted nested-inline-table
  config hangs startup for minutes-to-hours within the 4 MiB read cap, with no UI and no error — the
  cap it was supposed to be bounded by only limits bytes, not decode CPU. (2) A user delete landing
  during an in-flight edit-PUT is silently, permanently lost — `CommitPush` resurrects the resource
  clean and wipes its tombstone, and the next sync is a no-op; a real two-goroutine race in normal
  operation (background/debounced sync + a keypress). (3) A `:config`-added/renamed account is
  invisible and unreachable until restart, contradicting main.md:340.
- **All four canaries escaped — four live test-net holes**, none guarded: the config
  group-readable password warning, the state-file 0o600 permission contract, the CLI
  events-vs-tasks `components()` helper, and `treeNodeAtY`'s upper bound (a TUI-crashing panic). A
  boolean/comparison/off-by-one regression on any ships silently today. This is the second
  consecutive 4/4-escape pass.
- **The sync-core deep TOCTOU re-sweep bit on its first re-visit since pass 11 (the CommitPush HIGH)
  and is only partially cleared.** The reconcile-vs-concurrent-pull matrix beyond the CommitPush
  window (tombstone/keep-both races) remains shallowly covered — the surface is warm again, not
  proven.
- **The v1.1.0 feature is not live-verified end-to-end.** All account-switch coverage is headless;
  the real switch-then-sync path against a CalDAV server is unverifiable while the server is offline.
  Any switch/sync data-loss that manifests only against a real server is deferred.
- **Surfaces that produced no finding are warm, not proven correct**: the first audits of
  `global.json` (fault-injection) and the `runTUILoop` switch loop (fault-injection), the `:account`
  input-edge, and the whole-app spec-diff each found no defect through their lens — absence of a
  finding is not absence of a bug.
- **Repro hygiene:** `internal/store/commitpush_deletemidpush_test.go` is left in the tree,
  untracked, and currently RED — it will break `make check` until the CommitPush fix lands; keep it
  as the regression guard. The config-decode and `:config`-account repros were removed by the auditor
  (they would fail the gate pre-fix) — re-add bounded/asserting variants with each fix.
- **Not covered / still deferred:** full `sync-collection` token-delta sync (unbuilt); the Raspberry
  Pi hardware target (needs a physical Pi); live two-account CalDAV switch-and-sync (server offline).
  The model/caldav/store parser + write-primitive cells (passes 15–17) were deliberately left to cool.

**Recommendation: more passes recommended** — 2 HIGH + 1 MED confirmed (all unfixed) and 4/4
canaries escaped. Fix repro-first: bound the config decode (depth/deadline) so the read cap's
promise holds; teach `CommitPush` to honor a concurrent delete (`cur==nil` + tombstone → keep the
deletion, advance the tombstone ETag) for both the update and create variants; carry the account
list (+ active) through `ConfigReload` and refresh the picker/status bar in `applyConfigReload`; and
close the four canary holes with boundary tests. Re-run the deep sync-core TOCTOU sweep beyond the
CommitPush window before treating that surface as cleared.
