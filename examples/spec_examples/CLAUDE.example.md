# Claude Code — Project Instructions

> These rules apply to every task in this project.
> Claude Code reads this file automatically from the project root.

---

## Project Context

- **Spec**: `main.md` — the single source of truth for what to build (includes 12-step build plan).
- **Rules Reference**: `rules.md` — detailed hockey game rules (chain battles, goalie fatigue, etc.).
- **Original Specs**: `ASSETS/DATA/hockey_main_original.md`, `ASSETS/DATA/hockey_rules_original.md`, `ASSETS/DATA/baseball_main_original.md`.
- **Change Log**: `log.md` — append an entry every time you make a change.
- **Card Data**: `ASSETS/DATA/*.json` — skater, goalie, coach, and arena definitions.
- **Card Images**: `ASSETS/IMAGES/CARDS/*.jpg` — real vintage 1989-90 O-Pee-Chee hockey card scans.
- **Platform**: Windows, PowerShell, Python 3.11+, Pygame-CE.
- **Virtual Environment**: Stored **outside** the project at `C:\Users\%USERNAME%\.venvs\hockey_card` to avoid Google Drive sync issues (venvs contain hardcoded absolute paths that break across machines). Activate with:
  ```
  source "/c/Users/$USER/.venvs/hockey_card/Scripts/activate"
  ```

---

## Iterative Build Workflow

1. **Before starting work**, read `main.md` to understand the spec and `log.md` to see what's already been done.
2. **Work in small increments** — one module or feature at a time.
3. **After every change**, append a dated entry to `log.md` describing what was added, changed, or fixed.
4. **Run tests** after every code change: `python -m pytest TESTS/ -v`
5. **Run the game** to verify it still launches: `python SRC/main.py`
6. **Commit often** with descriptive messages: `git add . ; git commit -m "feat: ..."`

---

## Coding Standards

### No Global Variables

Do **not** use global variables. Pass state through:
- Constructor parameters (`__init__`)
- A shared context/config object
- Method arguments

Every class should receive its dependencies explicitly. If a module needs config values, pass the config dict or object in — don't import a global.

### Comment Rules

Follow three rules for comments:

- **Rule 1 — Names explain *what***: Choose clear, descriptive names for classes, methods, and variables. If the name is good enough, no comment is needed to explain what it does.
- **Rule 2 — Code explains *how***: The code itself should be readable enough to show how things work. Don't write comments that restate the code.
- **Rule 3 — Comments explain *why***: Only add comments when the reason behind a decision isn't obvious from the code. Explain *why* this approach was chosen, *why* a workaround exists, or *why* a non-obvious value is used.

```python
# BAD — restates what the code does
self.score = 0  # set score to zero

# GOOD — explains why
self.score = 0  # Reset between innings; accumulated score is in game_state.total_runs
```

### Other Conventions

- **Type hints** on all function signatures.
- **Docstrings** on public classes and public methods (one-liner is fine if the name is clear).
- **Imports**: Group as stdlib → third-party → project (`SRC.*`), separated by blank lines.
- **No wildcard imports** (`from module import *`).
- **f-strings** for string formatting (not `.format()` or `%`).
- **pathlib.Path** for file paths, not string concatenation.
- **Constants** come from `CONFIG/game_config.json` via `SRC/UTILS/constants.py` — never hardcode magic numbers.

---

## Architecture Rules

- `SRC/GAME/` — **zero pygame imports**. Pure Python game logic only.
- `SRC/UI/` — all pygame/rendering code.
- `SRC/UTILS/` — config loading, asset management, shared helpers.
- Card data is loaded from `ASSETS/DATA/*.json` by `CardDatabase` (in `asset_loader.py`). Card classes (`card.py`, `goalie.py`) are pure data — no pygame.
- Each screen is its own class with `handle_event()`, `update(dt)`, `draw(screen)`.

---

## Log Format

When appending to `log.md`, use this format:

```markdown
## YYYY-MM-DD — Short Title

- What was done (bullet points)
- Files created or modified
- Tests added or updated
- Any issues encountered
```

---

## Build Order (Hockey Migration)

Building from the existing baseball codebase. See `hockey_main.md` Step 3 for detailed file mapping.

1. **Rebrand docs & config** — Update CLAUDE.md, README, game_config.json to hockey identity
2. **Data files** — Create `ASSETS/DATA/skaters.json`, `goalies.json`, `coaches.json`, `arenas.json`, `nhl_teams.json`
3. **Core models** — `card.py` (SkaterCard, CoachCard), `goalie.py` (GoalieCard + fatigue)
4. **LBR engine** — `lbr.py` (keep as-is — same five-point star)
5. **Chain engine** — `chain.py` (chain resolution through 5 locations)
6. **Pattern engine** — `pattern.py` (preset + random pattern generation)
7. **Resolution engine** — `resolution.py` (period resolution with chain battles + goalie saves)
8. **Tests for core** — `test_card.py`, `test_lbr.py`, `test_chain.py`, `test_pattern.py`, `test_resolution.py`
9. **Deck & game state** — `deck.py`, `game_state.py` (3 periods, 5 placements per period)
10. **Utils** — `constants.py`, `asset_loader.py` (adapt for hockey card data)
11. **UI shell** — `game_window.py`, `menu.py` → game launches to menu with hockey branding
12. **Game screen** — `game_screen.py` → 5-hole goalie layout, chain animation, placement
13. **Polish** — verify full flow menu → team select → play → score → menu

At each step, run tests and verify the game still launches.
