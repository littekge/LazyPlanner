# Document Health — Design

> **Scope**: this spec addresses **document growth** only. A second, orthogonal problem — context clutter from reading long documents wholesale — is deliberately out of scope and will get its own design. Growth decides what content exists; access is designed over whatever survives.

## The problem

Documentation grows monotonically. Every change appends; nothing retires or compacts.

Growth itself is expected and legitimate: hard-won rules and new design decisions have to be recorded somewhere. The problem is the two kinds of growth that are **not** legitimate:

1. **Duplication** — recording content that already exists in another document.
2. **Staleness** — content that is superseded, contradicted, or points at things that no longer exist.

## Evidence (2026-07-26 condensation)

A manual condensation of four documents removed 15,306 words (47,675 → 32,369) without losing a single rule, decision, coverage row or residual — verified by an independent pass that checked 130 rules for loss or weakening.

**Duplication** was present in all four documents and accounted for most of the removal: `COVERAGE.md` had absorbed the pass reports' narrative; `main.md` restated v1.1–v1.3 design inside its own build records; `CLAUDE.md` carried 17 audit-pass references; `README.md` prose re-narrated the keybindings table.

**Staleness** appeared five times, including one contradiction found independently by two agents (the agenda-board click-to-select fix recorded as both shipped and not-shipped), a superseded UI description sitting directly beside the redesign that replaced it, and a coverage row citing a pass file that never mentioned its subject.

**The existing ritual did not catch any of it.** `.claude/commands/cleanup.md` already mandates "verify every doc is current per The Documents" and has run repeatedly at session end. The gap is not a missing ritual — it is that "verify currency" is an unbounded judgment task, which silently no-ops.

**Line counts hid the growth.** `CLAUDE.md` went 233 → 238 lines (+2%) while its word count rose 61%, because growth happened inside existing bullets.

## Non-goals

- Capping total document length. Legitimate growth is accepted.
- Condensing `log.md`. It is the append-only detailed record; startup reads only the 10 newest entries, so its length costs nothing.
- Solving context clutter. Separate problem, separate design.

## Layer 1 — Ownership (prevention)

Every fact has exactly **one owning document**. This makes duplication a typed error rather than a judgment call.

| Fact type | Owner |
|---|---|
| Design decision, behavior spec | `main.md` |
| Rule, banned practice, required pattern | `CLAUDE.md` |
| What happened and when | `log.md` |
| Audit finding narrative, per-pass detail | `docs/audit/passes/PASS-N.md` |
| Surface coverage state, residuals, next targets | `docs/audit/COVERAGE.md` |
| User-facing behavior, keys, install | `README.md` |
| In-progress task state | `notes.md` |

**Rule**: a non-owning document may *reference* a fact but never *restate* it.

**Citation format** — one form, specified once, so the detector can parse it and so the format cannot drift:

```
<short label> — see `<path>`[#<anchor>]
```

This table and rule are added to `CLAUDE.md`'s "The Documents" section.

## Layer 2 — Detection

### Living vs historical documents

The distinction that keeps the hard-fail check from becoming noise:

- **Living** — `CLAUDE.md`, `main.md`, `README.md`, `docs/audit/COVERAGE.md`, `docs/audit/PROTOCOL.md`. Describe the present. Subject to detection.
- **Historical** — `log.md`, `docs/audit/passes/PASS-N.md`. Append-only snapshots, frozen by design. A test name cited in a July 13 log entry may legitimately no longer exist; that is history, not drift. **Excluded from detection.**

### 2a. Dangling references — hard-fails `make check`

A Go test that extracts every reference from the living documents and asserts its target exists:

- Backticked file paths resolve on disk.
- Cited test and function names exist in the source.
- `PASS-N.md` citations resolve **and contain their subject**.
- Intra-doc anchors match a real heading.

Near-zero false positives, which is what earns it a place in the gate. Precedent: `internal/model/encoderdrift_test.go` already parses vendored Go source with `go/ast` so the gate enforces a re-diff that repeatedly failed by hand.

Would have caught: the wrong `PASS-11` pointer, and the ~82 orphaned identifiers found during verification.

### 2b. Duplicate spans — warn-only, not in the gate

Shingled n-gram overlap across living documents, reporting spans appearing in two or more. Advisory: it will have false positives, so it feeds a pass where a human is already judging rather than blocking a commit.

### 2c. Growth ledger

A small ledger records each living document's word count over time, so growth is visible without anyone having to notice it — the failure here was that nobody spotted `CLAUDE.md` doubling, because its line count barely moved.

**The ledger is written by `/cleanup` and at release, never by a test.** A test that writes into the repo on every `go test` run would produce spurious diffs and break the clean-tree discipline the project relies on. The docs-refs test stays strictly read-only.

Session startup reads the ledger's last entry (a few lines) and flags any living document that has grown more than **25%** since the last recorded compaction.

## Layer 3 — Scheduled compaction

### Two triggers, different scopes

**`/cleanup`, every session — mechanical only.** Confirm docs-refs is green, run the duplicate-span report, fix what it concretely names, append word counts to the ledger. No open-ended currency judgment: that is precisely what fails today. Frequent ledger entries also make the growth tripwire meaningful.

**Version release — full compaction**, four steps:

1. Resolve every confirmed duplicate span: delete the non-owner copy, replace with a pointer.
2. **Supersession walk** — per living document, does any decision sit beside a newer one that nullifies it?
3. **Retirement** — any guardrail whose class is now enforced by a machine check collapses to one line plus a pointer to the test.
4. Record post-compaction word counts in the ledger.

### Why step 3 is load-bearing

`CLAUDE.md` gains roughly 186 words per discovered class and nothing ever leaves, so its guardrail list is monotonic by construction. Making *conversion into a gate* the thing that earns a rule's prose the right to retire bounds that growth — and creates a standing incentive to build the gate, which independently attacks the separate problem of guardrail classes reopening.

## Limits

- Legitimate growth continues, by design.
- **Semantic staleness stays human judgment.** A superseded decision worded differently, rather than contradicting outright, is caught only by step 2, and only if someone looks.
- **docs-refs proves a target exists, not that it is the right target.** A pointer to a plausible-but-wrong section passes.
- **The duplicate detector will have false positives.** If ignored, the duplication half degrades to the honor system; feeding it into a scheduled pass is a mitigation, not a cure.
- **The check itself needs maintaining.** A drifting citation format rots it quietly — hence one specified format.

## Settled decisions (2026-07-26)

- **Growth threshold: 25%**, expressed as a single named constant so it is trivially tunable.
- **The duplicate-span detector is NOT built initially.** The first release-time compaction is done **by hand**, to learn what a useful signal actually looks like before automating one. A badly-tuned detector is worse than none: it produces false positives, gets ignored, and takes the credibility of the working checks down with it.

Two consequences follow, and the implementation must reflect them:

- `/cleanup`'s mechanical scope is initially **confirm docs-refs is green, then append word counts to the ledger** — there is no report to run yet.
- Release compaction step 1 (resolve duplicate spans) is a **manual review** for now. Whether to automate it is revisited after the first by-hand pass, informed by what that pass actually found.
