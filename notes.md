# LazyPlanner — Working Notes

> **Purpose**: short-term working memory — the state of a task in progress, written only when a session ends **mid-arc**. Record what's in progress, the remaining steps, blockers, and any temporary context the next session needs to pick the work back up. **The healthy steady state of this file is empty** (nothing below this header). Date every entry. When the task completes, delete its notes in the same increment that writes the `log.md` completion entry — resolution belongs to `log.md`, design to `main.md`; nothing accumulates here. A note that survives more than a few sessions is a misplaced `main.md` fact — move it there.

---

## 2026-07-24 — v1.5.0 mid-arc: phases 2–3 remain

**Where the version stands** (detail: main.md's v1.5.0 Build Plan status line): step 0, both
gap-closers, and all of phase 1 (spec-diff sweep + triaged fixes) are shipped and pushed. The
remaining v1.5.0 work:

1. **Phase 2 — key×context consistency matrix**: build the matrix (every key/chord ×
   NORMAL/DRILL/GRAB/SELECT/RESIZE × view/form/modal, actual behavior vs help bar / `:help` /
   README) via an agent fan-out, then triage divergences with the owner, exactly like phase 1.
   Reuse phase 1's workflow shape (`docs/superpowers/specs/2026-07-24-v1.5.0-polish-audit-design.md`
   §Phase 2). **Workflow-resume caution learned in phase 1**: a resumed workflow cache-misses any
   agent whose prompt embeds upstream agent output (re-serialization changes the bytes) — if a run
   dies partway, salvage by extracting the missing work into a fresh self-contained mini-workflow
   fed via `args`, instead of resuming the big run.
2. **Phase-2 cleanup batch candidates** (small, owner-approved direction, not yet done): the new
   `j`/`k` wiring in the Conflicts list / account picker duplicates `motionArrow`'s mapping —
   fold onto the existing idiom; re-check the `q`-inert-in-forms doc scoping once the matrix runs.
3. **Phase 3 — deep audit**: `/audit`, minimum one pass, targets already listed in main.md's
   convergence paragraph (SELECT/bulk-ops surface, v1.3.0 recurrence primitives, v1.2.0 grammar,
   sync-core reconcile matrix beyond `CommitPush` incl. the rollback-`Restore` clause noted in
   COVERAGE.md).
4. **Release gate** (spec §Release criteria): claim inventory dispositioned ✓ · matrix reconciled
   ✗ · gap-closers ✓ · ≥1 audit pass ✗ · docs current ✓ (as of this entry).
