---
description: Launch the coverage-first hardening-audit workflow (plan → audit → verify → canary → residual-risk report)
argument-hint: "[surface] [method] | key=value… | (empty = auto-pick least-audited)"
---

Launch the `hardening-audit` multi-agent workflow (`.claude/workflows/hardening-audit.js`).

Invoking this command **is** the explicit request to run a multi-agent workflow — treat it as the opt-in and call the `Workflow` tool directly; do not ask for confirmation again.

## Steps

1. Parse `$ARGUMENTS` into the workflow's `args` object (every field optional):
   - **No arguments** → call with **no `args`**. The workflow's Plan phase auto-selects the least-audited surfaces from `docs/audit/COVERAGE.md`. This is the normal full run.
   - **`key=value` tokens** map straight through: `maxTargets=N`, `maxCanaries=N`, `skeptics=N`, `ledgerPath=…`.
   - **A surface path, optionally followed by a method** → one explicit target. Methods are exactly one of: `fuzz`, `fault-injection`, `race`, `data-loss`, `input-edge`, `spec-diff`.
     - `internal/sync race` → `args: { targets: [{ surface: "internal/sync", pkg: "internal/sync", method: "race" }] }`
     - If a surface is given **without** a method, pick the best-fit method for it (fuzz for parsers/decoders, fault-injection for I/O, race for concurrent code, data-loss for write/sync paths, input-edge for UI handlers, spec-diff for promise-vs-code) and state which you chose.
   - Multiple surfaces → multiple target objects. `key=value` and targets can combine.
   - **Ambiguous** → ask one short clarifying question instead of guessing.

2. Call `Workflow({ name: "hardening-audit", args })` — omit `args` entirely when empty. It runs in the background; tell the user they can watch progress with `/workflows`.

3. When it completes, relay from the returned object — **do not** summarize it as "clean":
   - the **recommendation** (`more_passes_recommended` or `residual_accepted_with_caveats`) and the **residualRisk**,
   - each **confirmed finding** with its file:line, severity, and whether its repro was actually run (`reproRan`),
   - any **canary escapes** (mutations the suite failed to catch — test-coverage holes),
   - the **coverage-ledger update** and remaining **blind spots**,
   - and prominently, any `ENFORCEMENT:` warnings or `enforcement.valid === false` — those mean the run is **not trustworthy as-is** (missing ledger, unverified "confirmed" findings, or escaped canaries); say so plainly.

It is read-only on the working tree (audits read; canaries run in disposable git worktrees; only the final synthesis writes the ledger + `docs/audit/passes/PASS-N.md`). Applying fixes is a separate, deliberate step. See `docs/audit/PROTOCOL.md`.
