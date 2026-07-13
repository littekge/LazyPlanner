// hardening-audit — a coverage-first, evidence-producing audit workflow.
//
// It exists to defeat the failure mode that motivated it: a prior pass declared
// "1.0-ready" yet real HIGH bugs sat in surfaces that pass never examined. So this
// workflow refuses to trade in verdicts. It trades in *evidence*:
//
//   1. Plan    — read the coverage ledger + survey the repo; pick the surfaces
//                that are LEAST audited (never/stale), not whatever's convenient.
//   2. Audit   — fan out one method per target (fuzz, fault-injection, race,
//                data-loss/TOCTOU, input-edge, spec-diff). Diversity of method is
//                the point; a single lens misses whole bug classes.
//   3. Verify  — every finding faces N skeptics prompted to REFUTE it (default to
//                refuted when unsure). Survivors must carry a runnable repro that
//                the verifier actually executed. No repro → not "confirmed".
//   4. Canary  — inject known bugs into the audited surfaces and check the test
//                suite catches them. This tests the AUDITOR/net, not the code:
//                an escaped mutation is a hole you can measure, agent-independent.
//   5. Report  — emit a coverage-ledger update (incl. explicit blind spots), the
//                verified findings with repros, the canary escapes, and a
//                convergence signal vs the last pass. The recommendation is a
//                bounded *residual risk*, never "clean, ship it".
//
// Invoke:  Workflow({ name: 'hardening-audit' })
//   or:    Workflow({ name: 'hardening-audit', args: { targets: [{surface:'internal/sync', method:'race'}], maxTargets: 4 } })
// Read the docs/audit/PROTOCOL.md for the rules this enforces and how to read the output.

export const meta = {
  name: 'hardening-audit',
  description: 'Coverage-first hardening audit: plan (least-audited surfaces) -> method-diverse audit -> adversarial verify with repros -> mutation canary -> residual-risk report. Never returns "clean".',
  phases: [
    { title: 'Plan', detail: 'read the ledger + repo; choose least-audited surfaces' },
    { title: 'Audit', detail: 'one method per target, fanned out' },
    { title: 'Verify', detail: 'N skeptics refute each finding; survivors need a run repro' },
    { title: 'Canary', detail: 'inject known bugs; the suite must catch them' },
    { title: 'Report', detail: 'coverage ledger + findings + convergence + residual risk' },
  ],
}

// ---- configuration (override via args) ----------------------------------------
const LEDGER = (args && args.ledgerPath) || 'docs/audit/COVERAGE.md'
const PROTOCOL = 'docs/audit/PROTOCOL.md'
const MAX_TARGETS = (args && args.maxTargets) || 6        // surface x method audits this pass
const MAX_CANARIES = (args && args.maxCanaries) || 4      // mutation probes this pass
const SKEPTICS = (args && args.skeptics) || 3             // refuters per finding (majority rules)

// ---- structured-output schemas ------------------------------------------------
const PLAN_SCHEMA = {
  type: 'object',
  properties: {
    inventory: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          surface: { type: 'string' },
          pkg: { type: 'string' },
          methodsUsed: { type: 'array', items: { type: 'string' } },
          lastPass: { type: 'string' },
          status: { type: 'string', enum: ['never', 'stale', 'recent'] },
        },
        required: ['surface', 'status'],
      },
    },
    targets: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          surface: { type: 'string' },
          pkg: { type: 'string' },
          method: { type: 'string', enum: ['fuzz', 'fault-injection', 'race', 'data-loss', 'input-edge', 'spec-diff'] },
          rationale: { type: 'string' },
        },
        required: ['surface', 'method', 'rationale'],
      },
    },
    priorPass: {
      type: 'object',
      properties: { name: { type: 'string' }, high: { type: 'integer' }, med: { type: 'integer' }, low: { type: 'integer' } },
    },
    blindSpots: { type: 'array', items: { type: 'string' } },
  },
  required: ['inventory', 'targets', 'blindSpots'],
}

const FINDINGS_SCHEMA = {
  type: 'object',
  properties: {
    surface: { type: 'string' },
    method: { type: 'string' },
    findings: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          title: { type: 'string' },
          file: { type: 'string' },
          line: { type: 'integer' },
          severity: { type: 'string', enum: ['HIGH', 'MED', 'LOW'] },
          summary: { type: 'string' },
          failureScenario: { type: 'string' },
          reproSketch: { type: 'string' },
        },
        required: ['title', 'file', 'severity', 'failureScenario'],
      },
    },
  },
  required: ['surface', 'method', 'findings'],
}

const VERDICT_SCHEMA = {
  type: 'object',
  properties: {
    refuted: { type: 'boolean' },
    reason: { type: 'string' },
  },
  required: ['refuted', 'reason'],
}

const REPRO_SCHEMA = {
  type: 'object',
  properties: {
    hasRepro: { type: 'boolean' },
    kind: { type: 'string', enum: ['failing_test', 'command', 'manual_steps', 'none'] },
    repro: { type: 'string' },
    ran: { type: 'boolean' },          // did the verifier actually execute it?
    observed: { type: 'string' },      // what it observed (the crash/wrong output)
  },
  required: ['hasRepro', 'kind', 'repro', 'ran'],
}

const CANARY_SCHEMA = {
  type: 'object',
  properties: {
    surface: { type: 'string' },
    mutation: { type: 'string' },
    file: { type: 'string' },
    caught: { type: 'boolean' },       // did the suite fail (good) or pass (ESCAPE)?
    detail: { type: 'string' },
  },
  required: ['surface', 'mutation', 'caught'],
}

const SYNTH_SCHEMA = {
  type: 'object',
  properties: {
    passName: { type: 'string' },
    coverageUpdate: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          surface: { type: 'string' },
          method: { type: 'string' },
          result: { type: 'string' },
        },
        required: ['surface', 'method'],
      },
    },
    blindSpots: { type: 'array', items: { type: 'string' } },
    confirmedFindings: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          title: { type: 'string' },
          file: { type: 'string' },
          severity: { type: 'string', enum: ['HIGH', 'MED', 'LOW'] },
          reproKind: { type: 'string' },
          reproRan: { type: 'boolean' },
        },
        required: ['title', 'severity', 'reproRan'],
      },
    },
    canarySummary: {
      type: 'object',
      properties: {
        total: { type: 'integer' },
        escaped: { type: 'integer' },
        escapes: { type: 'array', items: { type: 'string' } },
      },
      required: ['total', 'escaped'],
    },
    convergence: { type: 'string' },
    residualRisk: { type: 'string' },
    // Deliberately NO "clean" / "ship" value — a pass ends in one of these two states.
    recommendation: { type: 'string', enum: ['more_passes_recommended', 'residual_accepted_with_caveats'] },
    wroteLedger: { type: 'boolean' },
    wrotePassReport: { type: 'boolean' },
  },
  required: ['coverageUpdate', 'blindSpots', 'confirmedFindings', 'canarySummary', 'residualRisk', 'recommendation'],
}

// ---- prompts ------------------------------------------------------------------
function planPrompt() {
  return `You are the planning step of a coverage-first hardening audit of the Go project at the repo root.

Read, in order:
- ${PROTOCOL} (the rules this audit enforces) and ${LEDGER} (the living coverage ledger), if they exist.
- The recent entries of log.md and the "Hardening & audit phase" section of main.md, to learn which surfaces were audited in which pass and by what method.
- The package layout (internal/*, cmd/*) to enumerate the real trust boundaries and input surfaces.

Produce an inventory of every meaningful surface (a package, trust boundary, or input path) with the method(s) already applied, the last pass that touched it, and a status: "never" (no audit ever), "stale" (audited but several passes ago, or only by a weak/indirect method), or "recent".

Then choose up to ${MAX_TARGETS} (surface, method) TARGETS for THIS pass. Rules for choosing:
- Strongly prefer "never" and "stale" surfaces — the whole point is to look where prior passes did NOT. Do not re-run a method on a surface it was recently run on unless you have a concrete reason.
- Match method to surface: fuzz for parsers/decoders; fault-injection for network/disk I/O; race for concurrent code; data-loss for sync/mutation/write paths; input-edge for UI key/command handlers; spec-diff for promised-vs-implemented gaps against main.md.
- Give a one-line rationale per target naming why this surface/method is under-covered.

Also record the prior pass's confirmed-finding counts by severity (from log.md) as priorPass, and list blindSpots: surfaces you are NOT covering this pass (be honest and explicit — an audit that hides its gaps is the failure we are preventing).`
}

function auditPrompt(t) {
  const method = {
    'fuzz': 'Treat the surface as a parser/decoder. Look for inputs that panic, hang, infinite-loop, or decode-but-cannot-re-encode (a data-loss round-trip break). Prefer to extend an existing FuzzXxx target and actually run `go test -run FuzzXxx` on the seed corpus; propose new seeds for anything suspicious.',
    'fault-injection': 'Attack the I/O trust boundary. What happens on a truncated/oversized/malformed/slow/wrong-status response or a failed disk write (ENOSPC/EACCES/rename fail)? Look for unbounded reads, swallowed errors, hangs, and state that diverges from disk.',
    'race': 'Hunt concurrency bugs. Find shared mutable state reached from >1 goroutine (sync loop vs UI edits, timers, background workers). Reason about interleavings that lose/duplicate data or tear a resource. Propose a `go test -race` stress test if one is missing.',
    'data-loss': 'Trace every write/mutation/delete path for the iron rule (never drop/mangle properties the app does not model) and for lost edits: TOCTOU between read and write, revert paths that swallow errors, tombstones/dirty flags dropped on failure, hand-built objects that skip a round-trip.',
    'input-edge': 'Drive the key/command/mode handlers in edge states: empty lists, nil/stale selection, out-of-range index after a shrink, interrupted chords, absurd counts, bad :command args, acting on an item a concurrent sync removed. Look for panics (nil deref, index OOB) and wrong-item mutations.',
    'spec-diff': 'Read the relevant part of main.md as the contract and diff it against the implementation. Find behavior that is promised but missing, silently different, or a settled decision the code contradicts.',
  }[t.method] || 'Audit this surface for correctness and robustness bugs.'

  return `You are auditing the surface "${t.surface}"${t.pkg ? ' (' + t.pkg + ')' : ''} of the Go project at the repo root, using ONE method: ${t.method}.

Method guidance: ${method}

Read the real code (do not guess). Use Bash/Grep/Read and, where the method calls for it, actually run the relevant tests. Report only findings you can point to with a file:line and a concrete failure scenario (inputs/state -> wrong outcome). Rank by severity: HIGH = crash/hang/data-loss/security; MED = degrades badly/interop violation; LOW = polish. Include a reproSketch (how one would trigger it) for each. Return an empty findings array if the surface is clean under this method — that is a valid, valuable result. Do NOT fix anything.`
}

function refutePrompt(f, i) {
  return `You are skeptic #${i + 1} of ${SKEPTICS}. Your job is to REFUTE this claimed defect, not to confirm it. Default to refuted=true unless you can independently reproduce the reasoning from the actual code.

Claim: "${f.title}" in ${f.file}${f.line ? ':' + f.line : ''} (severity ${f.severity}).
Failure scenario: ${f.failureScenario}
${f.reproSketch ? 'Repro sketch: ' + f.reproSketch : ''}

Read the cited code and its guards/callers. Consider: is the trigger actually reachable? Is there an existing guard, clamp, recover, or validation that already prevents it? Does a test already cover it? Is the severity inflated? Set refuted=true if the finding is wrong, unreachable, already-guarded, or overstated, with the specific reason. Set refuted=false ONLY if it genuinely holds.`
}

function reproPrompt(f) {
  return `A defect survived adversarial review: "${f.title}" in ${f.file} (severity ${f.severity}). Failure scenario: ${f.failureScenario}

Produce a REPRODUCTION the owner can run themselves, and ACTUALLY RUN IT to confirm the failure before the fix exists. Prefer, in order: (1) a small failing Go test (write it, run it, observe the failure — then remove it or leave it as a proposed regression test and say which); (2) an exact shell command that shows the bug; (3) precise manual steps if neither is possible.

Set ran=true only if you executed it and observed the failure; put what you observed in "observed". If you could NOT reproduce it, set hasRepro=false and kind="none" — an unreproducible finding must not be reported as confirmed.`
}

function canaryPrompt(surface, i) {
  return `You are a mutation canary testing whether the AUDIT NET (the test suite) has teeth for the surface "${surface}". You are in an isolated throwaway git worktree — edits here are discarded and never committed.

Introduce exactly ONE plausible, realistic bug into the production (non-test) code of this surface — e.g. flip a comparison (< to <=), delete a guard/clamp/recover, drop an error check, or change an off-by-one boundary. Pick a mutation that a real regression could plausibly be. Then run that package's tests (e.g. \`go test ./<pkg>/\`).

Report: the mutation you made and the file; caught=true if the suite FAILED (the net works), caught=false if the suite still PASSED (an ESCAPE — a measurable hole where a real bug could slip through). Put the test output summary in detail. Do not commit; do not try to fix anything. Vary your mutation from what other canaries might pick (you are probe #${i + 1}).`
}

function reportPrompt(plan, confirmed, canaries) {
  return `You are the synthesis step of a hardening audit. Produce the evidence report — NOT a verdict.

Inputs:
- Coverage plan (inventory + this pass's targets + blind spots): ${JSON.stringify(plan)}
- Confirmed findings (survived adversarial review; each should carry a repro): ${JSON.stringify(confirmed)}
- Mutation-canary results: ${JSON.stringify(canaries)}

Do all of the following:
1. Update the ledger at ${LEDGER}: for each target audited this pass, set its method/last-pass/status; add any newly discovered blind spots. Write the file. (Read it first; preserve existing rows.)
2. Determine the pass name/number from log.md (the next in sequence) and get today's date with \`date +%F\`. Write a pass report to docs/audit/passes/PASS-<n>.md summarizing coverage, findings (with repros), and canary escapes.
3. Return the structured summary. For "convergence", compare this pass's confirmed HIGH/MED/LOW counts to the prior pass (from the plan) and state whether severity is trending down. For "residualRisk", state plainly what remains unknown/uncovered. For "recommendation", choose "more_passes_recommended" if any HIGH/MED was confirmed, any canary escaped, or a high-value surface is still "never"/"stale"; otherwise "residual_accepted_with_caveats" and name the caveats. There is deliberately no "clean" option — do not imply the code is bug-free.
4. Set wroteLedger/wrotePassReport to reflect what you actually wrote.

Remember: a confirmed finding without a runnable repro is not confirmed — flag it as unverified in the report rather than counting it.`
}

// ---- helpers ------------------------------------------------------------------
// verifyFinding: adversarial refutation (majority) then a required, executed repro.
async function verifyFinding(f) {
  const votes = await parallel(
    Array.from({ length: SKEPTICS }, (_unused, i) => () =>
      agent(refutePrompt(f, i), { label: `refute:${f.file}#${i}`, phase: 'Verify', schema: VERDICT_SCHEMA })
    )
  )
  const heard = votes.filter(Boolean)
  const stands = heard.filter((v) => !v.refuted).length
  // Survives only on a strict majority of skeptics failing to refute it.
  if (stands * 2 <= SKEPTICS) return null
  const repro = await agent(reproPrompt(f), { label: `repro:${f.file}`, phase: 'Verify', schema: REPRO_SCHEMA })
  return { ...f, verdict: 'CONFIRMED', repro: repro || { hasRepro: false, kind: 'none', repro: '', ran: false } }
}

// ---- orchestration ------------------------------------------------------------
phase('Plan')
const plan = await agent(planPrompt(), { label: 'plan', phase: 'Plan', schema: PLAN_SCHEMA, effort: 'high' })
if (!plan) throw new Error('hardening-audit: planning step failed')

let targets = plan.targets.slice(0, MAX_TARGETS)
if (args && Array.isArray(args.targets) && args.targets.length) {
  targets = args.targets.slice(0, MAX_TARGETS) // explicit override wins
}
if (!targets.length) throw new Error('hardening-audit: no target surfaces selected')
log(`Plan: ${plan.inventory.length} surfaces mapped, ${plan.blindSpots.length} blind spots declared; auditing ${targets.length} target(s) this pass.`)

// Audit -> Verify pipelined: each target's findings verify as soon as its audit
// returns, so a slow audit never blocks a fast one's verification.
const perTarget = await pipeline(
  targets,
  (t) => agent(auditPrompt(t), { label: `audit:${t.surface}/${t.method}`, phase: 'Audit', schema: FINDINGS_SCHEMA }),
  async (res) => {
    if (!res || !res.findings || !res.findings.length) return []
    const verified = await parallel(res.findings.map((f) => () => verifyFinding(f)))
    return verified.filter(Boolean)
  }
)
const confirmed = perTarget.flat().filter(Boolean)
log(`Verified findings: ${confirmed.length} survived adversarial review.`)

// Canary: test the net, not the code. Isolated worktrees so parallel mutations
// never clobber the working tree or each other.
phase('Canary')
const canarySurfaces = targets.map((t) => t.pkg || t.surface).filter((v, i, a) => a.indexOf(v) === i).slice(0, MAX_CANARIES)
const canaries = (await parallel(
  canarySurfaces.map((surface, i) => () =>
    agent(canaryPrompt(surface, i), { label: `canary:${surface}`, phase: 'Canary', schema: CANARY_SCHEMA, isolation: 'worktree' })
  )
)).filter(Boolean)
const escapes = canaries.filter((c) => !c.caught)
log(`Canary: ${canaries.length} mutations, ${escapes.length} escaped the suite (holes).`)

// Report: evidence, not a verdict.
phase('Report')
const report = await agent(reportPrompt(plan, confirmed, canaries), { label: 'synthesize', phase: 'Report', schema: SYNTH_SCHEMA, effort: 'high' })

// Enforcement gate: refuse to let a thin/evidence-free result read as reassuring.
const enforcement = { valid: true, warnings: [] }
if (!report) {
  return { enforcement: { valid: false, warnings: ['synthesis failed to return a report'] }, confirmed, canaries }
}
if (!report.coverageUpdate || !report.coverageUpdate.length) {
  enforcement.valid = false
  enforcement.warnings.push('no coverage ledger update — result is INVALID (a pass must record what it examined)')
}
const noRepro = (report.confirmedFindings || []).filter((f) => !f.reproRan)
if (noRepro.length) {
  enforcement.warnings.push(`${noRepro.length} "confirmed" finding(s) lack an executed repro — treat as UNVERIFIED, not confirmed`)
}
if (report.canarySummary && report.canarySummary.escaped > 0) {
  enforcement.warnings.push(`${report.canarySummary.escaped} mutation(s) escaped the suite — the regression net has measurable holes`)
}
if (report.recommendation !== 'more_passes_recommended' && report.recommendation !== 'residual_accepted_with_caveats') {
  enforcement.valid = false
  enforcement.warnings.push('recommendation must be residual-based, never "clean"')
}
for (const w of enforcement.warnings) log(`ENFORCEMENT: ${w}`)

return { report, enforcement, confirmedCount: confirmed.length, canaryEscapes: escapes.length }
