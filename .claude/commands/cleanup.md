---
description: End-of-session cleanup — sweep residual worktrees/branches, verify doc currency, record mid-arc state in notes.md, commit & push to ai-workspace
argument-hint: "(no arguments)"
---

Run the end-of-session cleanup. Work through the steps **in order** and finish with the report in step 6. Safety rails throughout: never commit, merge to, or delete `main` or `ai-init`; never delete `ai-workspace`; when something is ambiguous, keep it and report it rather than deleting it.

## Steps

1. **Survey.** Run `git branch --show-current`, `git status`, `git worktree list`, and `git branch --list`. Note anything unusual: a non-`ai-workspace` branch checked out, leftover worktrees, unmerged experiment branches, untracked files.

2. **Sweep residual work products:**
   - **Worktrees**: remove leftover disposable worktrees (audit canaries and similar scratch under `.claude/worktrees/`): `git worktree remove --force <path>` + `git branch -D <its-branch>`, then `git worktree prune`. Only clearly disposable ones (e.g. `canary-*`); anything you can't identify → leave and report.
   - **Branches**: delete local branches already fully merged into `ai-workspace` (`git branch --merged ai-workspace`). A branch with unmerged work is either finished (merge it back into `ai-workspace` now, per Git Branching Rules) or mid-arc (leave it, record it in `notes.md` in step 4).
   - **Stray files**: delete obvious throwaway artifacts you (or a prior session) generated (scratch scripts, temp outputs). Generated-but-gitignored output (`dist/`, binaries) needs no action. A file you can't identify → leave and report.

3. **Doc currency pass** — check this session's changes against each document's role in CLAUDE.md "The Documents", and fix any gap **now**:
   - `main.md`: every behavior/design change from this session is reflected, **updated in place** (no superseded decision left standing).
   - `README.md`: every user-visible behavior/usage/build change is reflected.
   - `log.md`: every distinct change group has its own `## YYYY-MM-DD — Title` entry; verify the heading-count rule.
   - `docs/audit/COVERAGE.md`: current, if hardening or guardrails were touched.

4. **`notes.md` (mid-arc state):**
   - If a task is **in progress** and will not finish this session: write a dated entry — what's in progress, the remaining steps, blockers, and any temporary context the next agent needs to pick it up.
   - If the tasks this session **completed**: make sure their notes (if any) are deleted — the healthy steady state is an empty file (nothing below the header).

5. **Gate, commit, push:**
   - If any code changed this session (when unsure, assume it did), run `make check` and fix or report failures before committing. Docs-only sessions may skip the test run.
   - Commit all remaining changes with descriptive messages — grouped logically if the working tree spans unrelated concerns — on `ai-workspace` (or the current branch off it), **never `main`**.
   - `git push origin ai-workspace` (and push the current feature branch too, if on one). Confirm the push landed (`git status -sb`).

6. **Report.** End with a short summary: what was swept (or "nothing residual"), which docs were updated, the `notes.md` state (empty / mid-arc entry written for X), the commits pushed, and anything left that needs the owner's attention.
