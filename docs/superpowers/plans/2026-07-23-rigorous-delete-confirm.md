# Rigorous type-to-confirm for collection deletes — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Require the user to type a calendar/task-list's name before its (non-undoable) deletion can fire, replacing the ordinary one-button confirm for collection deletes only.

**Architecture:** `deleteCollection` (`internal/ui/calendar.go`) keeps its early guards but, instead of calling `a.confirm(...)`, opens a small `caretForm` (the same primitive as the calendar edit form) with one input field and Delete/Cancel buttons. The Delete button no-ops (flashes + stays open) unless the typed text matches the collection name via a pure, trim-and-case-sensitive helper. Item deletes are untouched — they stay undoable and keep the light confirm.

**Tech Stack:** Go, `rivo/tview` (`caretForm`, `tview.InputField`, buttons), `github.com/gdamore/tcell/v2`. Standard `testing` package, table-driven where natural.

## Global Constraints

- Only `internal/ui` may import tview/tcell. This work is entirely inside `internal/ui`; no `store`/`model` changes.
- `gofmt` is law; `goimports` ordering (stdlib → third-party → project). No new dependencies.
- Full gate after every code change, green before every commit: `go test ./...`, `go vet ./...`, `staticcheck ./...`, `go build ./...`.
- Every change is repro-first (TDD): failing test → minimal implementation → green, and the test stays as a regression guard.
- Comment rules: names explain *what*, code explains *how*, comments explain *why*.
- Docs ripple in the same increment: `main.md` (design), `README.md` (user-visible), `:help`, and a dated `log.md` entry.
- Existing test helpers to reuse (do not redefine): `newRootedTestApp(t, now)` (keys_test.go), `newTestApp` (app_test.go), `runeKey(r rune)` (keys_test.go:17), `keyEv(k tcell.Key)` (formnav_test.go:12).
- Relevant existing signatures (verbatim): `func (s *Store) Calendar(id string) (Calendar, bool)`; `store.Calendar` fields `DisplayName string`, `Resources []*Resource`, `ReadOnly bool`; `func (s *Store) MarkCalendarDeleted(ctx context.Context, id string) error`.

---

## Task 1: Pure name-match helper

**Files:**
- Modify: `internal/ui/calendar.go` (add the helper; `strings` is already imported)
- Test: `internal/ui/calendar_test.go` (append the unit test)

**Interfaces:**
- Consumes: nothing.
- Produces: `func collectionDeleteNameMatches(typed, name string) bool` — reports whether `typed` confirms deletion of a collection named `name`. Trim both sides, then case-sensitive equality.

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/calendar_test.go`:

```go
func TestCollectionDeleteNameMatches(t *testing.T) {
	cases := []struct {
		name, typed, target string
		want                bool
	}{
		{"exact", "School", "School", true},
		{"trailing space trimmed", "School ", "School", true},
		{"leading space trimmed", "  School", "School", true},
		{"both sides trimmed", "  School  ", "  School  ", true},
		{"wrong case rejected", "school", "School", false},
		{"substring rejected", "Scho", "School", false},
		{"empty rejected", "", "School", false},
		{"internal spaces significant", "My List", "My List", true},
		{"internal spaces mismatch", "MyList", "My List", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := collectionDeleteNameMatches(c.typed, c.target); got != c.want {
				t.Errorf("collectionDeleteNameMatches(%q, %q) = %v, want %v", c.typed, c.target, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestCollectionDeleteNameMatches`
Expected: FAIL — build error `undefined: collectionDeleteNameMatches`.

- [ ] **Step 3: Write minimal implementation**

Add to `internal/ui/calendar.go` (near `deleteCollection`):

```go
// collectionDeleteNameMatches reports whether typed confirms deletion of a
// collection named name. Trim + case-sensitive: both sides are trimmed so a
// stored stray space can't make a name impossible to type, but the match is
// otherwise exact — deleting a collection is not undoable, so the confirmation
// is deliberately strict.
func collectionDeleteNameMatches(typed, name string) bool {
	return strings.TrimSpace(typed) == strings.TrimSpace(name)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ui/ -run TestCollectionDeleteNameMatches`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/calendar.go internal/ui/calendar_test.go
git commit -m "feat: add trim+case-sensitive name-match for collection delete confirm

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 2: Type-to-confirm dialog + rewire deleteCollection

**Files:**
- Modify: `internal/ui/calendar.go` (add `promptDeleteCollection`; replace the `a.confirm(...)` block in `deleteCollection`)
- Test: `internal/ui/calendar_test.go` (append the UI test)

**Interfaces:**
- Consumes: `collectionDeleteNameMatches` (Task 1); `newCaretForm()`, `(*caretForm).addInput`, `(*caretForm).stylePopup`, `(*caretForm).AddButton`, `(*caretForm).SetCancelFunc`, `(*app).openModal(name string, prim tview.Primitive, width, height int)`, `(*app).closeModal`, `(*app).refresh`, `(*app).scheduleSyncDebounced`, `(*app).flash`, `(*app).flashErr`, `store.MarkCalendarDeleted`.
- Produces: `func (a *app) promptDeleteCollection(id string, cal store.Calendar) *caretForm` — opens the confirm dialog on `pageForm` and returns the form (so tests can drive it; the production caller ignores the return).

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/calendar_test.go`. This drives the real form via its NORMAL-mode navigation (`g` → first element, `j` → next, Enter → activate), exactly as the form-nav tests do:

```go
// TestCollectionDeleteRequiresTypedName: the collection-delete dialog deletes
// only when the typed text matches the name — a wrong name keeps the form open
// and deletes nothing; the correct name (whitespace-padded, to exercise the trim)
// deletes and closes the modal.
func TestCollectionDeleteRequiresTypedName(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	a.setMode(modeCalendar)
	id := a.selectedCalendarID()
	if id == "" {
		t.Skip("no calendar in the fixture")
	}
	cal, ok := a.store.Calendar(id)
	if !ok {
		t.Fatalf("selected calendar %q not found", id)
	}

	f := a.promptDeleteCollection(id, cal)
	in, ok := a.tv.GetFocus().(*tview.InputField)
	if !ok {
		t.Fatalf("focus after opening the dialog is %T, want the confirm *tview.InputField", a.tv.GetFocus())
	}

	// NORMAL-mode nav: g → first element (the input), j → the Delete button, Enter → activate.
	var focus func(tview.Primitive)
	focus = func(p tview.Primitive) { p.Focus(focus) }
	activateDelete := func() {
		focus(f)
		f.InputHandler()(runeKey('g'), focus)
		f.InputHandler()(runeKey('j'), focus)
		f.InputHandler()(keyEv(tcell.KeyEnter), focus)
	}

	// Wrong name: nothing deleted, form still open.
	in.SetText("definitely not the name")
	activateDelete()
	if _, ok := a.store.Calendar(id); !ok {
		t.Fatal("a wrong name deleted the calendar")
	}
	if name, _ := a.root.GetFrontPage(); name != pageForm {
		t.Errorf("front page after a wrong name = %q, want the dialog still open (%q)", name, pageForm)
	}

	// Correct name, whitespace-padded: deleted, modal closed.
	in.SetText("  " + cal.DisplayName + "  ")
	activateDelete()
	if _, ok := a.store.Calendar(id); ok {
		t.Error("typing the correct name did not delete the calendar")
	}
	if name, _ := a.root.GetFrontPage(); name != pageMain {
		t.Errorf("front page after delete = %q, want the modal closed (%q)", name, pageMain)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestCollectionDeleteRequiresTypedName`
Expected: FAIL — build error `a.promptDeleteCollection undefined`.

- [ ] **Step 3: Write the dialog builder**

Add to `internal/ui/calendar.go` (below `deleteCollection`):

```go
// promptDeleteCollection opens the rigorous type-to-confirm dialog for deleting a
// calendar/list. Unlike an item delete, a collection delete is not undoable (it
// pushes no undo op), so the user must type the collection's exact name before
// Delete fires — a stray keystroke can't wipe a whole collection. Returns the form
// so tests can drive it; the production caller ignores the return.
func (a *app) promptDeleteCollection(id string, cal store.Calendar) *caretForm {
	noun := "calendar"
	if a.mode == modeTasks {
		noun = "list"
	}
	title := fmt.Sprintf(" ⚠ Delete %s %q — cannot be undone ", noun, cal.DisplayName)
	if n := len(cal.Resources); n > 0 {
		title = fmt.Sprintf(" ⚠ Delete %s %q (%d item(s)) — cannot be undone ", noun, cal.DisplayName, n)
	}

	f := newCaretForm()
	nameField := f.addInput("Type name to confirm", "", 0)
	f.stylePopup()
	f.AddButton("Delete", func() {
		if !collectionDeleteNameMatches(nameField.GetText(), cal.DisplayName) {
			a.flash("Name doesn't match — type it exactly to delete")
			return // keep the dialog open for another attempt
		}
		if err := a.store.MarkCalendarDeleted(context.Background(), id); err != nil {
			a.flashErr("Delete", err)
			return
		}
		a.refresh("")
		a.closeModal(pageForm)
		a.scheduleSyncDebounced()
		a.flash(fmt.Sprintf("Deleted %q", cal.DisplayName))
	})
	f.AddButton("Cancel", func() { a.closeModal(pageForm) })
	f.SetCancelFunc(func() { a.closeModal(pageForm) })
	f.SetBorder(true).SetTitle(title)
	a.openModal(pageForm, f, 62, 7)
	return f
}
```

- [ ] **Step 4: Rewire `deleteCollection` to call it**

In `internal/ui/calendar.go`, replace the tail of `deleteCollection` — the block that builds `prompt`/`title` and calls `a.confirm(...)` (currently lines ~293–311, starting `prompt := fmt.Sprintf("Delete calendar %q", ...)` through the closing `})` of the `a.confirm` call) — with a single call:

```go
	a.promptDeleteCollection(id, cal)
}
```

Leave everything above it (the pane-mode switch, the empty-id/`not found`/`ReadOnly` guards) unchanged.

- [ ] **Step 5: Run the new test to verify it passes**

Run: `go test ./internal/ui/ -run TestCollectionDeleteRequiresTypedName`
Expected: PASS.

- [ ] **Step 6: Run the delete-guard regression test still passes**

The old `TestDeleteCollectionNeedsCollectionPane` (calendar_test.go) must stay green — it exercises the guard path that is unchanged.

Run: `go test ./internal/ui/ -run 'TestDeleteCollection|TestCollectionDelete'`
Expected: PASS (all).

- [ ] **Step 7: Full gate**

Run:
```bash
go test ./... && go vet ./... && staticcheck ./... && go build ./...
```
Expected: all pass, no output from vet/staticcheck, build succeeds.

- [ ] **Step 8: Commit**

```bash
git add internal/ui/calendar.go internal/ui/calendar_test.go
git commit -m "feat: type-to-confirm dialog for calendar/list deletes

A collection delete is not undoable; require typing the collection name
before Delete fires. Item deletes (undoable) keep the light confirm.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 3: Docs ripple

**Files:**
- Modify: `main.md` (move the item to shipped; update collection-delete prose)
- Modify: `README.md:120` (delete keybinding row) and `README.md:72` (`d` prose)
- Modify: `internal/ui/help.go:58` (delete row)
- Modify: `log.md` (new dated entry at the top)

**Interfaces:** none (documentation only).

- [ ] **Step 1: Update `main.md` — mark the item shipped**

In the v1.3.0 "Post-Build Incremental Changes" section, **remove** the "Rigorous confirm for irreversible deletes" bullet from the **"Planned before release"** group and add it to the **shipped** list (the group that begins "Behavior refinements after the six-step build … **Shipped** items each carry tests"), phrased as delivered. Suggested bullet:

```markdown
- **Rigorous confirm for collection deletes.** Deleting a calendar or task list
  (`d` on the focused Calendars/Tasks pane) is not undoable, so it no longer uses
  the one-button confirm: a type-to-confirm dialog requires typing the
  collection's exact name (trim + case-sensitive) before **Delete** fires — a
  mismatch flashes and keeps the dialog open. Item deletes stay on the ordinary
  undoable confirm.
```

If the "Planned before release" group is now empty except for the custom-recurrence-form item, leave that one item in place.

- [ ] **Step 2: Update the collection-delete prose in `main.md`**

In the Creation section paragraph that describes `d` deleting the focused pane's collection (search for `deletes the focused pane's collection` / the calendar create/edit form paragraph), append a sentence noting the type-to-confirm gate for collection deletes. Keep it one sentence; do not re-narrate the keybinding table.

- [ ] **Step 3: Update `README.md`**

Edit line 120's table row to name the stronger confirm:

```markdown
| `d` | Delete selected item — or the calendar/list when its panel is focused (typing its name to confirm, since a collection delete can't be undone) |
```

And extend the line 72 prose sentence so the `d` clause reads (collection deletes call out the type-to-confirm; item deletes keep the plain confirm):

```markdown
`d` deletes (an item after a confirm — a folder removes its whole subtree; a calendar/list, when its panel is focused, requires typing its name to confirm because it can't be undone).
```

Also, in the "Managing Calendars" paragraph (around line 94, the `d` to delete the focused pane's collection clause), add a short clause: "— confirmed by typing the collection's name, since it can't be undone."

- [ ] **Step 4: Update `:help` (`internal/ui/help.go:58`)**

Change the delete row text to note the collection-delete confirm:

```go
		{"d", "delete (item; calendar/list when its panel is focused — type its name to confirm)"},
```

- [ ] **Step 5: Verify docs build cleanly (help string compiles)**

Run: `go build ./... && go test ./internal/ui/ -run TestHelp`
Expected: build succeeds; any help-rendering test stays green. (If no `TestHelp` exists, `go build ./...` succeeding is sufficient.)

- [ ] **Step 6: Add the `log.md` entry**

Insert at the top of `log.md`, directly below the intro blockquote and above the newest existing entry (do not modify that entry):

```markdown
## 2026-07-23 — v1.3.0: rigorous type-to-confirm for collection deletes

- Collection deletes (calendar/list, `d` on the focused Calendars/Tasks pane) are not undoable, so they no longer use the one-button confirm. A new type-to-confirm dialog (`promptDeleteCollection`, `internal/ui/calendar.go`) requires typing the collection's exact name — trim + case-sensitive (`collectionDeleteNameMatches`) — before **Delete** fires; a mismatch flashes "Name doesn't match" and keeps the dialog open. Item deletes (undoable) keep the ordinary confirm.
- Built on the shared `caretForm` (inherits the popup chrome, focus-stack, and NORMAL/DRILL nav); the warning lives in the title (`⚠ Delete <noun> "<name>" (N item(s)) — cannot be undone`) since `openModal` type-asserts a `*caretForm` and can't wrap a separate text line.
- **Repro-first (TDD)**: `TestCollectionDeleteNameMatches` (match table) and `TestCollectionDeleteRequiresTypedName` (wrong name → nothing deleted, dialog stays open; correct whitespace-padded name → deleted, modal closed) — both RED before, green after.
- Docs rippled: `main.md` (item moved to shipped + collection-delete prose), `README.md` (delete row + prose), `:help`.
- Full gate green (`go test ./...`, `go vet ./...`, `staticcheck ./...`, `go build ./...`).
- Files: `internal/ui/calendar.go`, `internal/ui/calendar_test.go`, `internal/ui/help.go`, `main.md`, `README.md`, `log.md`.
```

- [ ] **Step 7: Verify `log.md` heading count**

Confirm the number of `##` headings equals the number of entries (one new entry added, all prior entries intact).

Run: `grep -c '^## ' log.md`
Expected: the previous count + 1.

- [ ] **Step 8: Commit**

```bash
git add main.md README.md internal/ui/help.go log.md
git commit -m "docs: record rigorous collection-delete confirm (v1.3.0)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

**1. Spec coverage** (against `docs/superpowers/specs/2026-07-23-rigorous-delete-confirm-design.md`):
- Scope (collection deletes only; empty + non-empty; item deletes untouched) → Task 2 rewires only `deleteCollection`; `confirm`/`confirmOK`/`styleModal` left in place. ✓
- Dialog: `caretForm`, title carries warning + name + item count, Delete/Cancel → Task 2 Step 3. ✓
- Warning in title/label, not a wrapped text line (openModal type-assert constraint) → Task 2 Step 3 + log note. ✓
- Interaction: NORMAL open, Enter/type flow, Esc/Cancel close → `SetCancelFunc` + buttons; test drives NORMAL nav. ✓
- Match helper: trim + case-sensitive, both sides trimmed → Task 1. ✓
- Mismatch: flash + stay open; match: existing delete body → Task 2 Step 3. ✓
- Files: only `internal/ui/calendar.go`; no store/model changes → Tasks 1–2. ✓
- Testing: unit table + UI wrong/right test; displaystress already covers caretForm → Tasks 1–2. ✓
- Docs ripple: main.md, README, :help, log.md → Task 3. ✓

**2. Placeholder scan:** No TBD/TODO; every code step shows complete code; every command has expected output. ✓

**3. Type consistency:** `collectionDeleteNameMatches(typed, name string) bool` used identically in Task 1 (def), Task 2 (call), and the log entry. `promptDeleteCollection(id string, cal store.Calendar) *caretForm` returns the form the Task 2 test consumes. `pageForm`/`pageMain` page names match the modal helpers. `store.Calendar` fields (`DisplayName`, `Resources`) match the verified signatures. ✓
