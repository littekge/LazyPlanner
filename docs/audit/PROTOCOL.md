# Hardening-audit protocol

The rules the `hardening-audit` workflow enforces, and how to read its output.
The premise: **a verdict ("1.0-ready") is unfalsifiable; evidence is checkable.**
An audit's job is to produce evidence and name its own blind spots — not to bless
the code. A pass that finds nothing is only meaningful if it also shows *what it
looked at, how, and what it did not*.

## The rules

1. **Coverage-first.** Every pass starts from the ledger (`COVERAGE.md`) and
   targets the surfaces that are `never`/`stale`, not whatever's convenient. The
   bug that motivated this protocol lived in a surface no prior pass had examined.
2. **Method diversity.** Match the method to the surface — fuzz (parsers),
   fault-injection (I/O), race (concurrency), data-loss (write/sync paths),
   input-edge (UI handlers), spec-diff (promise vs code). One lens misses whole
   classes; convergence comes from *different* methods agreeing, not from re-running
   the same one.
3. **Adversarial verification.** Each finding faces N skeptics prompted to *refute*
   it (default: refuted when unsure). Only strict-majority survivors proceed.
4. **A repro or it didn't happen.** Every confirmed finding ships a reproduction
   the verifier *actually ran* and you can re-run — ideally a failing test. No
   executed repro → reported as **unverified**, never "confirmed".
5. **Regression test per fix.** Fixing is a separate step, but every fix must leave
   a permanent test so the class can't silently return. The suite is the durable,
   compounding artifact — coverage you re-run beats a verdict you re-trust.
6. **Test the net, not just the code.** Each pass runs mutation *canaries*: inject
   known bugs into audited surfaces and confirm the suite catches them. An escaped
   mutation is a measurable hole, independent of any agent's self-report.
7. **One commit per fix, logged.** Traceability is a trust mechanism. An unlogged
   fix can't be verified after the fact regardless of code quality.
8. **No "clean" endpoint.** A pass ends in `more_passes_recommended` or
   `residual_accepted_with_caveats` — the honest target is *bounded, named residual
   risk*, because absence of evidence is not evidence of absence.

## The stop rule

Stop when **diverse methods converge**: successive passes over *new* ground (the
ledger proves it was new) stop finding HIGH/MED, the canary escape rate is ~0, and
the remaining blind spots are explicitly accepted. Do **not** stop because one agent
returned "no issues" on the first try — that selects for the least-thorough run.

## Reading the output

- `enforcement.valid=false` or any `ENFORCEMENT:` warning → the pass is not
  trustworthy as-is (missing ledger, unverified "confirmed" findings, escaped
  canaries, or a non-residual recommendation). Re-run or investigate.
- `canarySummary.escaped > 0` → fix the test *gap*, not just any finding.
- `convergence` + `residualRisk` → the real signal for readiness.

## Running it

```
Workflow({ name: 'hardening-audit' })
Workflow({ name: 'hardening-audit', args: { targets: [{ surface: 'internal/sync', pkg: 'internal/sync', method: 'race' }], maxTargets: 4 } })
```

It fans out many agents (opt into multi-agent first). It is **read-only** on your
working tree: audits only read, canaries run in disposable git worktrees, and only
the final synthesis writes (the ledger + a `passes/PASS-N.md` report). Applying
fixes is a separate, deliberate step.
