# LazyPlanner — Working Notes

> **Purpose**: short-term working memory — the state of a task in progress, written only when a session ends **mid-arc**. Record what's in progress, the remaining steps, blockers, and any temporary context the next session needs to pick the work back up. **The healthy steady state of this file is empty** (nothing below this header). Date every entry. When the task completes, delete its notes in the same increment that writes the `log.md` completion entry — resolution belongs to `log.md`, design to `main.md`; nothing accumulates here. A note that survives more than a few sessions is a misplaced `main.md` fact — move it there.

---

## 2026-07-21 — Fix the Pass 18 findings + close the canary holes (owner-approved, NOT STARTED)

**Owner decision (2026-07-21):** fix **all three** Pass 18 findings **and** close **all four** canary holes. Nothing is started yet — this is a clean handoff before any fix.

**Approach** (per CLAUDE.md + this session's discussion): **repro-first / TDD** — write the failing test, watch it fail for the right reason, fix, watch it pass; **one commit per fix**, full gate (`make check`) every commit; on `ai-workspace`. Use `test-driven-development` + `verification-before-completion` (they mirror the project rules). For the CommitPush race, apply `systematic-debugging`'s scope check — **the `pushCreate` variant shares the same root cause**, fix both. Optional independent review on the CommitPush change (races are subtle).

### Current repo state
- Branch `ai-workspace`, HEAD `d3b838c` (the Pass 18 audit commit). Tree clean, `make check` green.
- **2 commits ahead of `origin/ai-workspace`, UNPUSHED**: `dd3c955` (picker highlight fix) + `d3b838c` (Pass 18 audit). Owner may want these pushed. (Never merge to `main` — owner's action.)
- Full report: `docs/audit/passes/PASS-18.md`; ledger: `docs/audit/COVERAGE.md` (already updated).

### Suggested order (highest user-impact first)
1. CommitPush (HIGH, silent data loss) — including the `pushCreate` sibling.
2. `:config` account-list refresh (MED, contradicts main.md:340).
3. TOML decode bound (HIGH by label; low real-world likelihood — local crafted/corrupt config only — but owner wants it fixed; small fix).
4. The 4 canary holes (coverage gaps; current code is correct, so these are boundary regression tests).

---

### Fix 1 — CommitPush resurrects a resource deleted mid-push (HIGH, silent data loss)
- **Where:** `internal/store/remote.go:83` (`CommitPush`), and `stageResourceLocked` at `internal/store/mutate.go:161` (unconditional `delete(cs.tombstones, name)`).
- **Root cause:** the write callback treats `cur == nil` (event loop deleted the resource while the sync goroutine's PUT was in flight → tombstone left) identically to `cur == pushed` (unchanged) — it rebuilds the resource clean and the tombstone is wiped. User delete silently, permanently lost; next sync a no-op.
- **Also fix the sibling:** the never-pushed **`pushCreate`** variant has the same conflation — verify and fix both.
- **Fix direction:** distinguish `cur == nil` **with a tombstone present** → honor the deletion (don't resurrect; leave/advance the tombstone, e.g. carry the server ETag onto the tombstone so the next sync issues the DELETE `If-Match`). Only `cur == pushed` should finalize a clean resource.
- **Repro (verified RED on current code; helpers `seedSyncedResource` in `tombstone_test.go` + `findResource` in `testhelpers_test.go` already exist in-tree).** Drop this in as `internal/store/commitpush_deletemidpush_test.go`:

```go
package store_test

import (
	"context"
	"testing"

	"github.com/littekge/LazyPlanner/internal/store"
)

// TestCommitPushDoesNotResurrectDeletedResource reproduces the mid-push delete
// race: a synced resource is pushed by the background sync goroutine, the user
// deletes it (leaving a tombstone) while the PUT is in flight, and the PUT's
// CommitPush must NOT resurrect the resource or wipe the tombstone — the
// deletion must survive and still be pushed to the server on the next sync.
func TestCommitPushDoesNotResurrectDeletedResource(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	name := seedSyncedResource(t, dir, "cal1", "synced@test", "Synced")

	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	// The sync goroutine captured this snapshot and PUT it to the server.
	loc, ok := s.Locate("synced@test")
	if !ok {
		t.Fatal("resource not located")
	}
	pushed := loc.Prev
	href := pushed.Href

	// Mid-push: the user presses delete on the event loop. A tombstone is left.
	if err := s.Delete(ctx, "cal1", name); err != nil {
		t.Fatal(err)
	}
	if len(s.Tombstones()) != 1 {
		t.Fatalf("expected a tombstone after delete, got %d", len(s.Tombstones()))
	}

	// The PUT returns; sync finalizes it with the server's new ETag.
	if _, err := s.CommitPush(ctx, "cal1", name, pushed, `"srv-2"`, href); err != nil {
		t.Fatal(err)
	}

	// The deletion must not be lost.
	if got := s.Tombstones(); len(got) != 1 {
		t.Errorf("CommitPush wiped the tombstone of a mid-push deletion: got %d tombstones, want 1 (deletion silently lost)", len(got))
	}
	cal, ok := s.Calendar("cal1")
	if !ok {
		t.Fatal("calendar cal1 missing")
	}
	if r := findResource(cal, name); r != nil {
		t.Errorf("CommitPush resurrected the deleted resource %s (Dirty=%v, ETag=%q)", name, r.Dirty, r.ETag)
	}
}
```
- Add a parallel repro for the `pushCreate` variant. Consider a `-race` goroutine stress test (background sync + delete keypress) since this is a concurrency fix and the suite lacks one on this seam.
- **Note:** this is **pre-existing sync-core code, not v1.1.0** — but it's a release blocker for tagging v1.1.0 (silent data loss reachable with background sync).

### Fix 2 — `:config` reload doesn't refresh the account list (MED, contra main.md:340)
- **Where:** `internal/ui/command.go` `applyConfigReload` (never updates `a.accounts`/`a.activeAccount`, set once in `Run`); `ConfigReload` struct at `internal/ui/app.go` (carries only Sync/ColorMode/Warning); `editConfigFn` in `cmd/lazyplanner/main.go` (reads `cfg.Accounts` only to match the running account, never returns refreshed names).
- **Symptom:** a `:config`-added/renamed `[[account]]` stays invisible in the picker + status bar and unreachable via `:account` (flashes "unknown account") **until a full process restart**. main.md:340 promises the reload re-parses the account list live.
- **Clarification for the fixer (owner was unsure):** *switching* accounts is `:account` (in-app teardown+rebuild, already works, no process restart). This fix is only about making a newly-added account **visible/selectable** — refresh the in-memory list; `:account` then does the actual switch. Do **not** try to hot-swap the active account's cache; the existing "active account's connection changed → flash use `:account` or restart" behavior (in `editConfigFn`) is correct and must be preserved.
- **Fix direction:** add `Accounts []string` + `ActiveAccount string` to `ConfigReload`; `editConfigFn` returns the refreshed names; `applyConfigReload` updates `a.accounts` (+ `a.activeAccount` if unchanged) so the picker/status/`:account` see the new list. Repro: a UI test that reloads config with an added account and asserts `switchAccount("newname")` now records the switch (not "unknown account").

### Fix 3 — O(depth²) TOML decode hangs startup within the read cap (HIGH label, low real-world likelihood)
- **Where:** `internal/config/config.go:195` (`toml.Decode`); read cap `maxConfigBytes` (config.go:23) bounds bytes, not decode CPU; `Load` has no timeout.
- **Symptom:** a deeply-nested-inline-table config under 4 MiB hangs `Load()`/startup for minutes-to-hours (measured 32 KB→3.36 s, quadratic). Threat model: local corrupted/crafted config only (not remote); the real schema doesn't use nested inline tables. Owner: fix anyway for consistency with the project's "never hang" invariant; it's a small fix.
- **Fix direction:** bound the decode — decode in a goroutine with a `context`/timeout deadline, or reject configs past a nesting-depth cap before/while decoding. Repro: a timing/deadline test with a synthetic deep-inline-table string that fails (times out) on the unbounded path and passes once bounded. Keep it fast/deterministic (don't actually run for seconds — assert the bound triggers).

### Fix 4 — Close the 4 escaped canary holes (coverage gaps; current code is CORRECT)
These are missing regression tests, not live bugs — add a boundary test for each so the mutation is caught:
1. **`internal/config` `permissionWarning`** — mask `0o077` (flags group+other). Add a test asserting a **group-readable** (0o640) config triggers the warning (mutation `0o077→0o007` would drop it).
2. **`internal/state` `Save`** — the `0o600` file-mode contract is unasserted. Add a test asserting the written file's mode is `0o600` (Unix-only; guard `runtime.GOOS`). Mirror for `SaveGlobal` (same `writeJSONFile` helper).
3. **`cmd/lazyplanner` `calendar.go` `components()`** — the `--tasks`→VTODO / events→VEVENT mapping (+ `slugify`, `joinWarnings`) have zero coverage. Add a small table test pinning `events↔VEVENT`, `tasks↔VTODO`.
4. **`internal/ui` `mouse.go` `treeNodeAtY`** — the `idx >= len` upper-bound guard is unexercised (mutation `idx > len` would panic the TUI on a double-click one row past the last node). Add a boundary test that clicks past the last node and asserts no panic / no selection.

### Relevant context
- **v1.1.0 account switching is feature-complete** on `ai-workspace` (5 steps, committed) and the **owner confirmed it works with two live accounts (2026-07-21)** — so the live-verification gap is closed.
- **The CalDAV server is BACK ONLINE (2026-07-21)** — the live suite (`go test -tags live -run TestLive ./internal/sync/ -v`, throwaway test account only) is available again if any fix wants a live round-trip check.
- **Do not tag/release v1.1.0** until at least Fix 1 (CommitPush) lands — it's a silent-data-loss blocker. Release tagging is the owner's action regardless.
- Convergence after Pass 18: trending up (HIGH 0→2), streak broken; a clean re-sweep is still owed after these fixes land. Deeper sync-core reconcile-vs-concurrent-pull matrix (beyond the CommitPush window) remains a warm-but-shallow blind spot for a future pass.
