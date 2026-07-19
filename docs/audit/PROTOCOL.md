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
9. **Recurring class → codify the rule.** When a pass's findings share a root cause
   that is a *coding practice* rather than a one-off bug — especially one surfacing
   across multiple passes (e.g. the bare `Locate→Put` write) — the fix is not
   complete until the banned practice / required pattern is recorded as a Hard-won
   guardrail in `CLAUDE.md`, in the same increment as the fix. Regression tests
   protect existing code from regressing; only the guardrail protects *future* code
   from repeating the practice — the failure mode audits are worst at catching,
   since they examine what exists and the ledger immediately marks it "recent".

## Convergence — the stop rule

"Zero findings" is the **wrong target**: real software keeps a long MED/LOW tail,
this workflow is built never to return "clean" (rule 8), and one surface — the
Raspberry Pi on real hardware — cannot be audited headlessly at all. So convergence
is a **severity-weighted trend across diverse passes, not a raw finding count**. A
flat count (e.g. 5→6) while HIGH falls (5→1→0 across passes 10→13→14) is
*converging*, not stalling — judging by count alone misreads it as stuck.

The phase is **converged for release readiness** when *all* of these hold:

1. **Matrix covered once.** Every *headless* surface×method cell in `COVERAGE.md`
   has been audited at least once. The Pi-hardware surface is exempt and stays a
   permanently-accepted gap. Until the matrix is complete, "still finding bugs"
   means coverage is incomplete — not that the code is uniquely fragile.
2. **Severity floor at zero.** Two *consecutive* passes over new-or-re-swept ground
   yield **no HIGH**.
3. **No new root-cause class.** Those same two passes surface no MED of a *new*
   class — only variations of classes already bound by a guardrail + tests (rule 9).
4. **Rising trigger cost.** New findings require increasingly exotic conditions to
   reach (e.g. foreign import + degraded network + invalid typed input), not
   ordinary use.
5. **Canary escape rate ~0.** The suite catches injected mutations on audited
   surfaces.

Converged does **not** mean "stop auditing" — it means drop from back-to-back
passes to occasional spot re-sweeps and accept the named residual risk. Do **not**
treat a single "no issues" pass as convergence; that selects for the least-thorough
run — criterion 2/3 require *two* consecutive passes precisely to defeat that.

Make the trend auditable: every `PASS-N.md` records its HIGH·MED·LOW counts **and
whether any MED was a new root-cause class**, so the two-consecutive-clean-pass test
can be checked against the record rather than re-litigated each pass.

## Reading the output

- `enforcement.valid=false` or any `ENFORCEMENT:` warning → the pass is not
  trustworthy as-is (missing ledger, unverified "confirmed" findings, escaped
  canaries, or a non-residual recommendation). Re-run or investigate.
- `canarySummary.escaped > 0` → fix the test *gap*, not just any finding.
- `convergence` + `residualRisk` → the real signal for readiness.

**Test-net guardrail — boundaries and sibling-guard parity** (codified after passes 14 + 17 escaped
twin canaries — the pass-17 `DayAgenda` upper-bound escape mirrored the pass-14 lower-bound escape on
the *same* function, and `reconcileReadOnly`'s degraded-download escape mirrored a guard already
covered on the read-*write* path). Escaped canaries recur when the regression net is extended
point-by-point rather than by class. When closing a canary — or writing any boundary/guard test —
close the whole class, not the one point:

1. **Both sides of every half-open window.** A `[start, end)` check has two boundaries; a test that
   pins one (e.g. a todo due exactly at `dayStart` is *included*) must be paired with one that pins
   the other (due exactly at `dayEnd` is *excluded*). One-sided boundary tests leave the opposite
   comparison free to flip silently.
2. **Mirror a guard onto every sibling path.** When a guard exists on one path (e.g. the read-write
   reconcile's degraded-download / empty-href skip), grep for its siblings (`reconcileReadOnly`, the
   Import loop) and give each the *same* canary. A guard covered on one twin and untested on another
   is a standing escape waiting to happen — the pass-17 MED findings were themselves this shape (a
   sibling path missing a guard its twin already had).

## Running it

The `/audit` slash command is a thin wrapper that just launches this workflow with
your arguments:

```
/audit                       # full run; Plan auto-picks the least-audited surfaces
/audit internal/sync race    # one explicit target (surface + method)
/audit maxTargets=3          # cheaper run
```

Or drive the workflow directly (ask in natural language; the assistant calls the tool).
Launch by `scriptPath` — this environment resolves only built-in workflow *names*, so
`name: 'hardening-audit'` fails; the file path always works:

```
Workflow({ scriptPath: '.claude/workflows/hardening-audit.js' })
Workflow({ scriptPath: '.claude/workflows/hardening-audit.js', args: { targets: [{ surface: 'internal/sync', pkg: 'internal/sync', method: 'race' }], maxTargets: 4 } })
```

It fans out many agents (opt into multi-agent first). It is **read-only** on your
working tree: audits only read, canaries run in disposable git worktrees, and only
the final synthesis writes (the ledger + a `passes/PASS-N.md` report). Applying
fixes is a separate, deliberate step.
