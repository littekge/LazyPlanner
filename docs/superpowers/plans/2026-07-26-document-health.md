# Document Health Implementation Plan

> ## NOT IMPLEMENTED — deliberately deferred (owner decision, 2026-07-26)
>
> **Do not execute this plan.** It is complete and reviewed, and it is kept as the record of a considered
> decision, not as pending work.
>
> The mechanism it describes prevents documentation growth over many future sessions. The project is
> reaching its final state, so it would never amortize. Reopening it needs the owner's explicit say-so.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop illegitimate documentation growth — duplication and staleness — with a single-owner rule, a hard-failing dangling-reference check, a growth ledger, and two compaction triggers.

**Architecture:** Three layers from the spec. Ownership makes duplication a typed error. Detection is split by precision: dangling references hard-fail `make check` over the *living* documents only, while historical records (`log.md`, `docs/audit/passes/`) are excluded because their references are frozen snapshots. Compaction runs mechanically at every `/cleanup` and fully at each release.

**Tech Stack:** Go standard library only (`testing`, `os`, `regexp`, `path/filepath`, `strings`). No new dependencies. Make for the size target.

**Spec:** `docs/superpowers/specs/2026-07-26-document-health-design.md` — read it before starting.

## Global Constraints

- Go standard library only. No new third-party dependency, no vendoring change.
- `gofmt` is law; imports in goimports order (stdlib → third-party → project, blank-line separated).
- Doc comments on all exported identifiers, godoc style, starting with the identifier's name.
- Comments explain **why**, not what. A comment restating the code is a defect.
- Named constants for anything with meaning — no magic numbers.
- Every error checked. No package-level `var` holding mutable state (immutable lookup tables are fine).
- Standard `testing` package only; table-driven preferred.
- Full gate every commit: `go test ./... && go vet ./... && staticcheck ./... && go build ./...`
- One commit per task. Commit messages use a conventional prefix and end with:
  `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>`
- Work on `ai-workspace`. **Never** commit to or merge into `main`.
- **Living documents** (subject to checking): `CLAUDE.md`, `main.md`, `README.md`, `docs/audit/COVERAGE.md`, `docs/audit/PROTOCOL.md`.
- **Historical records** (never checked): `log.md`, `docs/audit/passes/PASS-N.md`.
- Growth threshold: **25%**, as a single named constant.

## Measured baseline (verified 2026-07-26 — do not re-derive)

Across the five living documents: **72** distinct path-like backticked references, **0** genuinely missing. **25** distinct `Test*`/`Fuzz*` names cited, **0** missing. The check therefore passes on day one, provided it implements these three resolution rules:

1. A literal path that exists resolves.
2. Otherwise, a **bare basename** resolves if one or more files with that name exist outside `vendor/` (e.g. `grab.go` → `internal/ui/grab.go`). Ambiguity is **not** an error — `color.go` and `recur_edit.go` each exist in two packages, and forcing qualification is churn with no correctness gain.
3. Allowlisted templates and elisions are skipped: `foo.go`, `foo_test.go`, `PASS-N.md`, `passes/PASS-N.md`, `config.toml`, and any token containing `...`.

## File Structure

| File | Responsibility |
|---|---|
| `internal/docscheck/doc.go` (create) | Package clause + doc comment. Exists so `go build ./...` does not fail on a test-only package. |
| `internal/docscheck/refs.go` (create) | Pure reference extraction and resolution helpers. No test logic. |
| `internal/docscheck/refs_test.go` (create) | Unit tests for the helpers, plus the integration check over the real living documents. |
| `CLAUDE.md` (modify) | Ownership table + citation format; architecture note for the new package; compaction procedure; startup ledger step. |
| `Makefile` (modify) | `doc-sizes` target. |
| `docs/doc-sizes.md` (create) | The growth ledger. |
| `.claude/commands/cleanup.md` (modify) | Mechanical document-health step. |

---

### Task 1: Ownership table and citation format

Prevention layer. Makes duplication a typed error rather than a judgment call, and fixes one citation format so Task 2 can parse it and the format cannot drift.

**Files:**
- Modify: `CLAUDE.md` — the "The Documents" section

**Interfaces:**
- Produces: the citation format `<short label> — see \`<path>\`[#<anchor>]`, which Task 2's anchor check parses.

- [ ] **Step 1: Read the target section**

Read `CLAUDE.md`'s "The Documents" section in full. It already states per-document maintenance rules; you are adding a table that makes ownership explicit across them, not replacing what is there. Match the file's existing voice and density.

- [ ] **Step 2: Add the ownership table and rule**

Insert directly after the "Style rules for **all** documents" bullet list, before the `### main.md` subsection:

```markdown
**Every fact has exactly one owning document.** A non-owning document may *reference* a fact but never *restate* it — restating is how duplication enters, and how two documents come to disagree.

| Fact type | Owner |
|---|---|
| Design decision, behavior spec | `main.md` |
| Rule, banned practice, required pattern | `CLAUDE.md` |
| What happened and when | `log.md` |
| Audit finding narrative, per-pass detail | `docs/audit/passes/PASS-N.md` |
| Surface coverage state, residuals, next targets | `docs/audit/COVERAGE.md` |
| User-facing behavior, keys, install | `README.md` |
| In-progress task state | `notes.md` |

**Citation format** — one form, so the gate can check it and it cannot drift:

`<short label> — see `path/to/file.md`` , optionally with `#anchor` for a heading.
```

> When transcribing that last line, write it as a normal sentence with the path in backticks; do not nest backticks inside backticks. The literal shape a reference takes is: a short label, an em dash, the word "see", then the backticked path, optionally followed by `#anchor`.

- [ ] **Step 3: Verify and commit**

```bash
go build ./... && git add CLAUDE.md
git commit -m "docs: every fact has one owning document, with a fixed citation format

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 2: The dangling-reference check

The gate. A read-only Go test that fails `make check` when a living document points at a file, test, function or heading that does not exist.

**Files:**
- Create: `internal/docscheck/doc.go`
- Create: `internal/docscheck/refs.go`
- Create: `internal/docscheck/refs_test.go`
- Modify: `CLAUDE.md` — the architecture package list

**Interfaces:**
- Consumes: the citation format from Task 1.
- Produces: `extractRefs(md string) []string`, `resolvePath(root, ref string) error`, `resolveIdent(root, name string) error`, `resolveAnchor(root, ref string) error`, `repoRoot(t *testing.T) string`.

**Why `internal/docscheck`:** it must run under `go test ./...` so `make check` and CI both catch it with no separate wiring. Precedent is `internal/model/encoderdrift_test.go`, which parses vendored source so the gate enforces a re-diff that kept failing by hand. The package imports nothing from the application. A test-only directory breaks `go build ./...` with "no non-test Go files", which is why `doc.go` exists.

- [ ] **Step 1: Write the failing unit tests**

Create `internal/docscheck/refs_test.go`:

```go
package docscheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractRefsSkipsFencedBlocks(t *testing.T) {
	md := "See `real.go` here.\n\n```\nthis `fenced.go` must be ignored\n```\n\nAnd `other.md`.\n"
	got := extractRefs(md)
	want := []string{"real.go", "other.md"}
	if len(got) != len(want) {
		t.Fatalf("extractRefs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("extractRefs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResolvePath(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "ui"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"main.md", "internal/ui/grab.go"} {
		if err := os.WriteFile(filepath.Join(root, p), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		ref     string
		wantErr bool
	}{
		{"main.md", false},              // literal path
		{"internal/ui/grab.go", false},  // literal path
		{"grab.go", false},              // bare basename, resolves by search
		{"nosuch.go", true},             // genuinely missing
		{"foo.go", false},               // allowlisted template
		{"vendor/.../encoder.go", false}, // elision
	} {
		err := resolvePath(root, tc.ref)
		if (err != nil) != tc.wantErr {
			t.Errorf("resolvePath(%q) err = %v, wantErr %v", tc.ref, err, tc.wantErr)
		}
	}
}

func TestResolveIdent(t *testing.T) {
	root := t.TempDir()
	src := "package x\n\nfunc TestRealThing(t *testing.T) {}\n"
	if err := os.WriteFile(filepath.Join(root, "x_test.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := resolveIdent(root, "TestRealThing"); err != nil {
		t.Errorf("resolveIdent(TestRealThing) = %v, want nil", err)
	}
	if err := resolveIdent(root, "TestGhost"); err == nil {
		t.Error("resolveIdent(TestGhost) = nil, want an error")
	}
}

func TestResolveAnchor(t *testing.T) {
	root := t.TempDir()
	md := "# Title\n\n## The Documents\n\ntext\n"
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := resolveAnchor(root, "a.md#the-documents"); err != nil {
		t.Errorf("resolveAnchor(existing) = %v, want nil", err)
	}
	if err := resolveAnchor(root, "a.md#no-such-heading"); err == nil {
		t.Error("resolveAnchor(missing) = nil, want an error")
	}
}

// TestLivingDocsReferencesResolve is the gate: every reference in a living
// document must point at something that exists. Historical records are excluded
// on purpose — a test name cited in an old log entry may since have been
// renamed, which is history, not drift.
func TestLivingDocsReferencesResolve(t *testing.T) {
	root := repoRoot(t)
	for _, doc := range livingDocs {
		body, err := os.ReadFile(filepath.Join(root, doc))
		if err != nil {
			t.Fatalf("read %s: %v", doc, err)
		}
		for _, ref := range extractRefs(string(body)) {
			var err error
			switch {
			case strings.Contains(ref, ".md#"):
				err = resolveAnchor(root, ref)
			case identPattern.MatchString(ref):
				err = resolveIdent(root, ref)
			case pathPattern.MatchString(ref):
				err = resolvePath(root, ref)
			default:
				continue // ordinary inline code, not a reference
			}
			if err != nil {
				t.Errorf("%s: %v", doc, err)
			}
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/docscheck/`
Expected: FAIL — the package does not exist yet.

- [ ] **Step 3: Write the implementation**

Create `internal/docscheck/doc.go`:

```go
// Package docscheck holds repo-hygiene checks over the project's living
// documentation. It contains no application code and nothing imports it; its
// only job is to fail the gate when a document points at something that no
// longer exists.
package docscheck
```

Create `internal/docscheck/refs.go`:

```go
package docscheck

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// livingDocs describe the present and are therefore checked. log.md and
// docs/audit/passes/ are excluded deliberately: they are append-only historical
// records whose references are snapshots of the day they were written, so a
// since-renamed identifier is history rather than drift. Checking them would
// produce pure noise, and a noisy gate gets disabled.
var livingDocs = []string{
	"CLAUDE.md",
	"main.md",
	"README.md",
	"docs/audit/COVERAGE.md",
	"docs/audit/PROTOCOL.md",
}

// refAllowlist are backticked tokens shaped like references that are
// deliberately not real files: illustrative examples and filename templates.
var refAllowlist = map[string]bool{
	"foo.go":           true,
	"foo_test.go":      true,
	"PASS-N.md":        true,
	"passes/PASS-N.md": true,
	"config.toml":      true,
}

var (
	backtickPattern = regexp.MustCompile("`([^`\n]+)`")
	pathPattern     = regexp.MustCompile(`^[A-Za-z0-9_./-]+\.(go|md|toml)$`)
	identPattern    = regexp.MustCompile(`^(Test|Fuzz)[A-Za-z0-9_]+$`)
	headingPattern  = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
)

// extractRefs returns the backticked spans of a markdown document, skipping
// fenced code blocks — a code sample may reference a path that intentionally
// does not exist.
func extractRefs(md string) []string {
	var out []string
	fenced := false
	for _, line := range strings.Split(md, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		for _, m := range backtickPattern.FindAllStringSubmatch(line, -1) {
			out = append(out, m[1])
		}
	}
	return out
}

// resolvePath reports whether ref names a file that exists. A bare basename is
// resolved by searching the tree, because the docs legitimately write `grab.go`
// rather than the full path. Multiple matches are accepted: the reference is
// imprecise, not broken, and demanding qualification would be churn.
func resolvePath(root, ref string) error {
	if refAllowlist[ref] || strings.Contains(ref, "...") {
		return nil
	}
	if _, err := os.Stat(filepath.Join(root, ref)); err == nil {
		return nil
	}
	if strings.Contains(ref, "/") {
		return fmt.Errorf("dangling path reference %q", ref)
	}
	found := false
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "vendor" || d.Name() == ".git") {
			return filepath.SkipDir
		}
		if !d.IsDir() && d.Name() == ref {
			found = true
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("searching for %q: %w", ref, err)
	}
	if !found {
		return fmt.Errorf("dangling path reference %q", ref)
	}
	return nil
}

// resolveIdent reports whether a cited Test*/Fuzz* function exists in the
// source. These are the pointers that rot most often: a guardrail names the
// regression test that enforces it, the test gets renamed, and the pointer
// silently stops leading anywhere.
func resolveIdent(root, name string) error {
	needle := "func " + name + "("
	found := false
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "vendor" || d.Name() == ".git") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || found {
			return nil
		}
		body, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.Contains(string(body), needle) {
			found = true
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("searching for %q: %w", name, err)
	}
	if !found {
		return fmt.Errorf("dangling identifier reference %q", name)
	}
	return nil
}

// resolveAnchor reports whether ref's "file.md#heading" target names a heading
// that exists, matching GitHub's slug rules closely enough for our own docs.
func resolveAnchor(root, ref string) error {
	parts := strings.SplitN(ref, "#", 2)
	if len(parts) != 2 {
		return fmt.Errorf("malformed anchor reference %q", ref)
	}
	body, err := os.ReadFile(filepath.Join(root, parts[0]))
	if err != nil {
		return fmt.Errorf("anchor reference %q: %w", ref, err)
	}
	for _, m := range headingPattern.FindAllStringSubmatch(string(body), -1) {
		if slugify(m[1]) == parts[1] {
			return nil
		}
	}
	return fmt.Errorf("dangling anchor %q", ref)
}

// slugify converts a heading to its anchor form: lowercase, spaces to hyphens,
// punctuation dropped.
func slugify(h string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(h)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	return b.String()
}

// repoRoot walks up from the test's working directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up from the test directory")
		}
		dir = parent
	}
}
```

> `refs.go` imports `testing` for `repoRoot`. That is acceptable in a package whose only purpose is checks, but if `staticcheck` objects, move `repoRoot` into `refs_test.go` and leave the rest in `refs.go`.

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/docscheck/ -v`
Expected: all PASS, including `TestLivingDocsReferencesResolve` against the real documents. The measured baseline says 0 genuine failures. **If it reports failures, do not add them to the allowlist reflexively** — check whether each is a real dangling reference that should be fixed in the document instead. Report any you allowlist and why.

- [ ] **Step 5: Prove the gate bites**

Append a deliberately broken reference to a living document — for example add ``See `internal/ui/no_such_file.go`.`` to the end of `README.md` — and run `go test ./internal/docscheck/`. Confirm it FAILS naming that reference. Remove the line, confirm GREEN, and confirm `git status` is clean. Report both outputs. A check that cannot fail is not a check.

- [ ] **Step 6: Record the package in the architecture list**

In `CLAUDE.md`'s Architecture section, add `docscheck/` to the `internal/` package list with a one-line description: repo-hygiene checks over the living documentation; no application code.

- [ ] **Step 7: Full gate and commit**

```bash
go test ./... && go vet ./... && staticcheck ./... && go build ./...
git add internal/docscheck CLAUDE.md
git commit -m "test: fail the gate when a living doc points at something that does not exist

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 3: Growth ledger and the `/cleanup` step

Makes growth visible automatically. The failure this addresses is that nobody noticed `CLAUDE.md` doubling, because its line count barely moved while its word count rose 61%.

**Files:**
- Modify: `Makefile`
- Create: `docs/doc-sizes.md`
- Modify: `.claude/commands/cleanup.md`

**Interfaces:**
- Produces: `make doc-sizes` (word count per living document), and `docs/doc-sizes.md` whose last `compaction` row is the baseline Task 4's startup check compares against.

- [ ] **Step 1: Add the Make target**

The `Makefile` already has `check: test vet staticcheck` at line 36 and a `## fmt:` comment convention. Add, following that convention:

```make
## doc-sizes: word counts for the living documents (feeds docs/doc-sizes.md)
doc-sizes:
	@for f in CLAUDE.md main.md README.md docs/audit/COVERAGE.md docs/audit/PROTOCOL.md; do \
		printf "%s\t%s\n" "$$(wc -w < $$f)" "$$f"; \
	done
```

Do **not** add `doc-sizes` to `check` — it prints, it does not assert.

- [ ] **Step 2: Verify the target**

Run: `make doc-sizes`
Expected: five tab-separated lines, word count then path. Keep the output; the next step seeds the ledger with it.

- [ ] **Step 3: Create the ledger**

Create `docs/doc-sizes.md`, filling the seed row with the **real numbers from step 2** and today's date:

```markdown
# Document size ledger

> Word counts for the living documents over time. Appended by `/cleanup` and at each release compaction — never by a test, since a test that writes into the repo would produce spurious diffs on every run.
>
> The `compaction` rows are baselines. A document more than **25%** above its most recent `compaction` baseline is flagged at session startup.

| Date | Event | CLAUDE.md | main.md | README.md | COVERAGE.md | PROTOCOL.md |
|---|---|---|---|---|---|---|
| 2026-07-26 | compaction | … | … | … | … | … |
```

- [ ] **Step 4: Add the document-health step to `/cleanup`**

Read `.claude/commands/cleanup.md` first and match its structure. Add a "Document health" step containing exactly this scope:

1. Confirm `make check` is green — the dangling-reference check runs inside it.
2. Run `make doc-sizes` and append a row to `docs/doc-sizes.md` with today's date and event `session`.
3. Compare each count against the most recent `compaction` row. Report any document more than 25% above its baseline; do not act on it — that is the release compaction's job.

If the existing file has a step asking an agent to "verify every doc is current per The Documents", **replace it** with the above. That instruction is why this mechanism is needed: it is an unbounded judgment task, it has run repeatedly, and it caught none of the 15,306 words of duplication and five contradictions found on 2026-07-26. Bounded, tool-driven steps are the point.

- [ ] **Step 5: Verify and commit**

```bash
make doc-sizes && go build ./...
git add Makefile docs/doc-sizes.md .claude/commands/cleanup.md
git commit -m "docs: growth ledger, make doc-sizes, and a bounded /cleanup doc-health step

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 4: Release compaction procedure and the startup tripwire

Closes the loop: the ledger is only useful if something reads it, and compaction is only useful if its scope is small enough that skipping it is obvious.

**Files:**
- Modify: `CLAUDE.md` — Session Startup, and the Versioning section

**Interfaces:**
- Consumes: `docs/doc-sizes.md` from Task 3.

- [ ] **Step 1: Add the startup tripwire**

`CLAUDE.md`'s "Session Startup" currently has five numbered steps. Add a sixth, before the summary step:

```markdown
6. **Check document growth**: read the last row of `docs/doc-sizes.md` and compare it with the most recent `compaction` row. Flag to the owner any living document more than **25%** above its baseline — it is due for compaction.
```

- [ ] **Step 2: Add the release compaction procedure**

In `CLAUDE.md`'s Versioning section, add:

```markdown
### Document compaction at release

Every release compacts the living documents. Growth from new rules and new design decisions is expected; duplication and staleness are not.

1. **Duplication** — review the living documents against the ownership table. Where a non-owning document restates a fact, delete the restatement and leave a pointer. *Done by hand for now: automating a duplicate-span detector was deliberately deferred until a by-hand pass shows what a useful signal looks like.*
2. **Supersession** — per document, does any decision sit beside a newer one that nullifies it? Rewrite in place.
3. **Retirement** — any guardrail now enforced by a machine check collapses to one line plus a pointer to that check.
4. **Record** — append a `compaction` row to `docs/doc-sizes.md`. This becomes the new baseline.

Step 3 is what bounds growth. `CLAUDE.md` gains roughly 186 words per discovered class and nothing ever leaves, so its guardrail list is monotonic by construction. Making conversion into a gate the thing that earns a rule's prose the right to retire is what breaks that — and it creates a standing incentive to build the gate.
```

- [ ] **Step 3: Verify and commit**

Confirm the new `CLAUDE.md` text introduces no dangling references:

```bash
go test ./internal/docscheck/ && go build ./...
git add CLAUDE.md
git commit -m "docs: release compaction procedure and the startup growth tripwire

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage.** Layer 1 ownership → Task 1. Layer 2a dangling references → Task 2. Layer 2b duplicate spans → deliberately not built, per the spec's settled decision; Task 4 step 1 records it as a by-hand review. Layer 2c growth ledger → Task 3, with the startup read in Task 4. Layer 3 `/cleanup` trigger → Task 3 step 4; release trigger → Task 4 step 2. Living/historical split → Task 2's `livingDocs` and its doc comment. 25% constant → Tasks 3 and 4.

**Placeholders.** None. The one intentional blank is the ledger seed row, which must be filled from the real `make doc-sizes` output rather than guessed — flagged at its step.

**Type consistency.** `extractRefs`, `resolvePath`, `resolveIdent`, `resolveAnchor`, `repoRoot`, `livingDocs`, `refAllowlist`, `pathPattern`, `identPattern` are declared in `refs.go` and used with matching signatures in `refs_test.go`.

**Known risk.** `resolvePath` and `resolveIdent` each walk the tree per reference — roughly 100 walks per run. If that proves slow, cache one file-name set and one identifier set per run; do not add caching pre-emptively.
