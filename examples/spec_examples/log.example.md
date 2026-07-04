# Drop of the .. — Change Log

> Append a new entry every time a change is made. Newest entries at the top.

---

## 2026-04-24 — Hints off by default

- `SRC/UI/hint_manager.py`: changed `HintManager.__init__` default from `enabled=True` to `enabled=False`; existing saved prefs still respected, so users who have already toggled hints on stay on
- Web builds regenerated: `hockey_game_web.zip` and `hockey_game_web_mobile.zip`

---

## 2026-04-24 — Newspaper apostrophe fix, top-of-screen line fix, web rebuilds

- `CONFIG/newspapers.json`: removed apostrophes from "The Devils Advocate" and "The Cats Meow Gazette"
- `SRC/GAME/pchc_cup.py`: removed all apostrophes from rendered newspaper text (HOCKEY_JOKES, COACH_QUOTES, PLAYER_QUOTES, HNIC_REFS, and all article builder methods); contractions expanded, possessives simplified, single-quote quotation marks removed
- `WEB_BUILD/index_desktop.html`, `index_mobile.html`: removed leftover diagnostic logDiv (position:fixed; top:0; padding:4px) that caused a ~8px black strip at top of browser; collapsed logViewport to a no-op stub
- `.gitignore`: updated WEB_BUILD entry to track HTML templates and web_build_notes.md
- Web builds regenerated: `hockey_game_web.zip` and `hockey_game_web_mobile.zip`

---

## 2026-04-24 — Web builds, doc updates, chain-1 Weak Shot rules and tutorial

- `WEB_BUILD/hockey_game_web.zip`: rebuilt desktop (128.3 MB, BUILD_VERSION 202604241557)
- `WEB_BUILD/hockey_game_web_mobile.zip`: rebuilt mobile (portrait 600x900)
- `README.md`: Chain Scoring table updated — "0-1 Turnover" split into Chain 0 (Turnover) and Chain 1 (Weak Shot +10%); step 8 mentions chain 1+ shots; Chain 5 corrected to -75% (was "Auto-goal")
- `main.md`: "chains reach length 2" updated to "chains reach length 1"; Weak Shot note added

## 2026-04-24 — Chain-1 Weak Shot: rules.md and tutorial updates

- `rules.md`: Overview updated — chains of 1 now produce a Weak Shot (+10% goalie)
- `rules.md`: Section 7 Chain Results table: split "0-1 Turnover" into "0: Turnover" and "1: Weak Shot (goalie +10%)"
- `rules.md`: Section 16 Quick Reference chain scoring now lists Chain 1 = Weak Shot
- `rules.md`: LBR Battle Resolution fixed — "Tie: Offense wins" replaced with "50/50 live roll"
- `rules.md`: PCHC Arena and Coach tables updated to match new redesigned abilities
- `SRC/UI/tutorial_v2.py`: "Building a Chain" scene text updated to mention chain-1 Weak Shot
- `SRC/UI/tutorial_v2.py`: "The Chain Pyramid" scene text: "Chain 0-1: Turnover" split into two rows
- `SRC/UI/tutorial_v2.py`: `_draw_chain_pyramid` visual renderer: added Chain 1 "Weak Shot / +10" row

---

## 2026-04-24 — Tutorial pre-warm and collage frame cache

- `SRC/UI/game_window.py`: tutorial screen now built during startup loading (while
  "Loading in ~20 seconds" is visible) so first "How to Play" click is instant
- `SRC/UTILS/asset_loader.py`: `load_collage_frame` now caches by (index, w, h)
  so repeated calls across window resizes or tutorial re-opens pay zero cost

---

## 2026-04-24 — Arena and coach ability redesign: no location-specific effects, timing tools

- `ASSETS/DATA/arenas.json` — full rewrite of all 9 arena boon/penalty pairs:
  - Ball-O-Devils: chain +1 step boon / goalie -15 pct penalty
  - Felines: free first chain step boon / chain cap 3 penalty
  - Mile-High: fatigue immunity boon / double fatigue penalty
  - RedTurtles: 2 save rerolls boon / opponent gets 4th pattern penalty
  - Poodles: tie advantage all battles boon / opponent chain-4 auto-goal penalty
  - Fighting Scots: extra pattern boon / chain-4 no quality bonus penalty
  - Red Cows: cascade debuff boon / all skaters -1 LBR penalty
  - Habitants: chain-3-plus -15 pct goalie boon / first battle -1 LBR penalty
  - Mutton Busters: cap opponent chains at 4 boon / goalie -10 pct penalty
- `ASSETS/DATA/coaches.json` — full rewrite of all 9 coach abilities:
  - Grove Phantom: Stampede — chains gain 2 extra steps
  - Knox Midnight: Pounce Protocol — extra pattern + free first chain step
  - Diesel Cinder: Summit Defense — fatigue immunity + goalie +15 pct
  - Garret Basalt: Shell Shock — tie advantage all + 1 save reroll
  - Slade Sable: Ghost Walk — all skaters +1 LBR + 2 save rerolls
  - Duke Jarvis: Highland Fury — +1 LBR always, trailing P3 adds +1 LBR and +1 chain step
  - Magnus Gorge: Bull Rush — cap opponent chains at 3 + own chains +1 step
  - Buck Loyalist: La Forteresse — fatigue immunity + chain-3-plus -20 pct goalie
  - Puck Farrow: Wild Stampede — extra pattern + cascade debuff -1
- `SRC/GAME/ability_effects.py` — engine handler additions:
  - `_apply_lbr_boost`: added `target=="all"` path → `lbr_global`
  - `build_arena_effects`: added `cascade_debuff` type; `tie_advantage` now checks `target="all"` → `tie_advantage_all`
  - `_apply_fatigue_modifier`: sets `fatigue_immunity=True` when `multiplier==0.0`
  - `_apply_coach_effect` for `chain_boost`: added `target="all_chains"` → `chain_length_bonus`
  - `_apply_coach_effect` for `tie_advantage`: added `target="all"` alongside existing `"defense"`
- `SRC/GAME/game_state.py`: `get_winner()` returns `"tie"` when `in_overtime=True` (tied after regulation)
- All 76 tests passing

---

## 2026-04-24 — Tutorial polish: pattern lines, arena examples, mobile fixes, test fixes

- `SRC/UI/tutorial_v2.py`:
  - Page 5 (Per-Period Resolution): replaced simple colored lines with game-style thick AA pattern lines; shows P2 highlighted (filled green button), P1/P3 outlined; gradient alpha and square start marker matching game renderer
  - Page 9 (Comeback Roll): green ring box widened to `int(420*s)` so label text fits
  - Page 10 (Arena Boons and Penalties): replaced static 3-row effects table with 3 real arena examples (stadium thumbnail + arena name + BOON/PENALTY descriptions); removed P1/P2/P3 period buttons from left column
  - Added `_draw_aa_thick_line` static method (mirrors `hockey_game_screen._draw_aa_thick_line`)
  - Added `_render_wrapped` helper for word-wrapping arena description text
- `SRC/UI/tutorial_mobile.py`:
  - Override (1,2) visual_type changed to `"pattern_view"` so mobile shows the pattern diagram
  - Override (3,0) removed entirely; mobile now falls through to LESSONS and shows `boon_penalty` visual with real arena examples
  - Pages loop updated to consult `MOBILE_SCENE_OVERRIDES` for effective vtype — eliminates blank image page
  - Image page scale increased: base 0.75, caps 1.05/1.5, divisors 600/380 (was 0.6, 0.9/1.1, 720/440)
- `TESTS/test_lbr.py`: `test_lbr_draw_favors_offense` → `test_lbr_tie_is_50_50` (verifies ~50% win rate over 1000 trials)
- `TESTS/test_chain.py`: `test_ties_favor_offense` → `test_ties_are_live_roll` (verifies chain-5 only happens a small fraction of the time with equal LBR)

---

## 2026-04-22 — Tutorial rewrite: desktop + mobile bitmap-font safety, visual fixes

- `SRC/UI/tutorial_v2.py` — complete rewrite of LESSONS and all visual renderers:
  - Restructured 18 scenes → 15 scenes across 5 lessons (deleted Pages 3/5, merged Pages 6+7, moved Page 12 to new Page 5)
  - All text made bitmap-font-safe: removed apostrophes, %, (, ), +, =, /
  - Header height increased (100*s) and lesson title centered at 48*s to stop clipping
  - Forward card border changed Red (220,60,60), Defense card border Blue (60,110,220)
  - LBR ratings visual: added A vs A 50. row; removed "Tie: Offense always wins" text
  - Net layout: colored filled boxes instead of numbered boxes
  - Shot visual: removed percent signs, changed result line to "52 is greater than 45 GOAL!"
  - Comeback roll: bigger ring (48*s), updated labels
  - New `_draw_period_buttons()` helper for boon/penalty page
  - Coach tradeoff: uses return value of `_draw_card_thumb` to avoid overlap
  - Fatigue bars: taller (38*s) so text fits
- `SRC/UI/tutorial_mobile.py` — bitmap-font safety fixes only (visual rendering delegates to v2):
  - Page counter: `Page x/y` → `Page x of y` (removed `/`)
  - Override (1,2): `Pass / Turnover / SAVE / GOAL` → dashes; `+ goalie` → `and goalie`
  - Override (3,0): semicolon → comma in `seeing yours; you see theirs after`

---

## 2026-04-23 — Docs update: web scaling fix documented in README and main.md

- `README.md` — updated Web Build feature bullet and Web Build section to describe template-based `index.html` generation and mobile `MutationObserver` canvas enforcer
- `main.md` — updated "Web builds" Current State bullet with same detail

---

## 2026-04-23 — AI improvements, newspaper content, mobile UI polish

### SRC/GAME/hockey_ai.py
- Added three helpers: `_effective_win_rate()` (Comeback Roll-aware win probability), `_avg_lbr()` (hand quality), `_pattern_visit_index()` (location priority in attack chains)
- Rookie face-up weights changed to `[20, 60, 20]` (explicit 20% boon / 60% standard / 20% penalty)
- Veteran face-up now plays 3 only when hand is weak (avg_lbr > 2.0); adds pattern-position bonus to card scoring
- Veteran face-down now accepts opponent goalie saves and weights free-fill placements toward goalie weak spots
- All-Star face-up thresholds relaxed (strong < 1.5, medium 1.5–2.2); uses `_pattern_visit_index()` helper
- All-Star face-down rewritten: builds counter-target list sorted by Comeback Roll-adjusted win rate instead of raw LBR; exploits the gap-1 spike (≈64% win vs naive 40%)

### SRC/GAME/pchc_cup.py
- Expanded content pools: +6 pro teams, +4 defunct teams, +2 college refs, +4 hockey jokes, +6 coach quotes, +5 player quotes, +4 Hockey Night in Canada references
- Two new side-article generators: `_press_box_notes_article()` (scouting notes / stat fun facts) and `_fan_reaction_article()` (fan atmosphere / social media)
- Both new generators added to the article dispatch pool

### SRC/UI/game_window.py
- Mobile newspaper caching redesigned: now generates 7 articles (main + 6 side types) and picks one randomly per `(round, games_played, phase)` cache key instead of generating once
- Headline pool expanded to 2 headlines per generation
- Card image height on mobile newspaper reduced from 38% to 26% of page height
- Grayscale conversion switched from custom helper to `pygame.transform.grayscale()`
- Body font minimum size raised from 16 to 24 px for readability
- Word-wrap rewritten: handles `\n\n` paragraph breaks, inserts blank lines between paragraphs, ellipsizes only the last non-blank visible line

### SRC/UTILS/asset_loader.py
- Collage frame scaling replaced `smoothscale()` with cover-scale logic (fill + center-crop) to prevent squishing on portrait/mobile screens

---

## 2026-04-23 — Web build: template-based index.html generation

- `TOOLS/build_web.py` — replaced ~200-line string-patch block with a single template substitution step; reads `WEB_BUILD/index_desktop.html`, replaces the baked-in bundle name with the current build's staging dir name, updates `BUILD_VERSION` to a fresh timestamp, and writes the result as `build/web/index.html`
- `TOOLS/build_web_mobile.py` — replaced `_patch_index_html()` with `_install_index_html()`; same approach using `WEB_BUILD/index_mobile.html`
- Both scripts now produce the hand-crafted HTML exactly (MutationObserver canvas enforcer, `settle_and_resize`, `visualViewport` listeners, `hockey_desktop_build_version` / `hockey_mobile_build_version` cache keys) without fragile string-replace patches against pygbag's generated output
- Added `import datetime` and `import re` to top-level imports in `build_web.py`

---

## 2026-04-22 — Web build: black background + loading screen

- `TOOLS/build_web.py` + `TOOLS/build_web_mobile.py` — added three extra HTML patches applied after pygbag generates `index.html`:
  1. `background-color:powderblue` → `#000000` (pygbag's body CSS rule was overriding our `<head>` injection via cascade)
  2. `platform.document.body.style.background = "#7f7f7f"` → `"#000000"` (Python inline style beats all CSS; must be patched at source)
  3. `#infobox` CSS: `background: green; color: blue` → `background: #111111; color: #f0d98a` + centered padding + border-radius
  4. Initial loading div text: `"Loading, please wait ..."` → `"Drop of the .."` + `"Loading…"` subtitle

## 2026-04-22 — Mobile sudden-death overtime fix

- `SRC/UI/hockey_mobile_screen.py` — added `_resolve_overtime` method; `_advance_resolution_or_period` now checks `self._gs.in_overtime` after period 3 and calls `_resolve_overtime` instead of going straight to `PHASE_PERIOD_SUMMARY`; OT plays back as a timed resolution log with the same Pass/SAVE/GOAL animation and puck-glide system as regular periods

## 2026-04-22 — Mobile Auto-Fill button; rules.md made-up team arenas & coaches

- `SRC/UI/hockey_mobile_screen.py` — added Auto-Fill button to the bottom action bar; always allocated in layout (right of main action button), only drawn/clickable when no cards are placed yet; taps `_do_autofill()` which uses same bot AI + weighted face-up count (1:20%, 2:30%, 3:50%) as desktop
- `rules.md` — added full arena table for all 9 PCHC made-up teams (Pitchfork Pavilion, Scratching Post, Thin Air Coliseum, Shell Shocker Dome, Grooming Gallery, Kilt & Claymore Coliseum, Udder Dome, La Maison du Puck, Sheep Pen) with preset patterns, boon names/effects, and penalty names/effects
- `rules.md` — added full coach table for all 9 PCHC coaches (Grove Phantom through Puck Farrow) with ability names and effects; real-team coach list retained as "Example" section

## 2026-04-22 — Tie battles now roll 50/50 instead of auto-win offense

- `SRC/GAME/lbr.py` — removed early-return shortcut for gap-0 ties; `WIN_PCT_BY_GAP[0]` changed from `1.0` → `0.5`; every battle now generates a live dice roll including ties
- `rules.md` — updated LBR probability table (Gap 0: 50% live roll), removed "ties always favor offense" bullet, updated Comeback Roll note, tutorial summary, and Chicago Stadium boon description

## 2026-04-22 — Desktop placement-preview pucks, mobile matchup display fix

- `SRC/UI/hockey_game_screen.py` — removed `_draw_pattern_lbr_pucks` (was incorrectly overlaying LBR values on pattern dots at all times); replaced with `_draw_card_placement_preview` called after `_draw_pattern_lines` so ghost pucks render on top of the pattern overlay
- `_draw_card_placement_preview` draws a semi-transparent LBR puck at every empty YOUR slot when a card is selected (alpha 130 base, 180 on hover); green filled ring at diff==+1 (you are underdog → comeback roll for you), red at diff==-1 (opponent underdog)
- `SRC/UI/hockey_mobile_screen.py` — fixed `_draw_net_slots`: opponent slot at visual row `loc_id` now correctly displays `t2_placements.get(matchup[loc_id])` (the card that actually defends against your attack), matching the desktop; previously showed opponent's same-numbered slot card, causing green/red highlights to appear inconsistent with the visually paired opponent card
- `_draw_pucks_over_patterns` in mobile: same matchup fix for LBR puck display on opponent slots

## 2026-04-22 — RollDisplay widget, Comeback Roll integration, tutorial scene, mobile placement highlights

- `SRC/GAME/lbr.py` — added `return_roll=True` kwarg; returns `(result, roll_1_to_100)` tuple; ties return `(0, 0)`
- `SRC/GAME/chain.py` — `BattleStep` gains `roll_int`, `comeback_roll_int`, `first_roll_won` fields; `resolve_chain` saves pre-comeback `attacker_wins` as `first_attacker_wins` and populates all three fields
- `SRC/UI/roll_display.py` (new) — self-contained puck animation widget; states SPINNING→LANDING→RESULT→(PAUSING→SPINNING for second roll)→DONE; two-roll Comeback variant shows side-by-side pucks with combined green/red outcome ring
- `SRC/UI/hockey_game_screen.py` — imports RollDisplay; log tuples extended to 9-tuple (index 8 = RollDisplay|None); `_make_battle_roll_display` / `_make_shot_roll_display` helpers; roll zone (80px) carved out of log panel; `_current_roll_display` ticked in `update()`; skip() called at all jump-to-end sites
- `SRC/UI/hockey_mobile_screen.py` — same RollDisplay integration as desktop (8-tuple log entries, 68px roll zone, helpers, skip); `_draw_selected_preview` now shows pucks at ALL 5 locations (alpha 180 for empty/placeable, 80 for filled), with green ring where gap=1 vs opponent's placed card (Comeback Roll indicator)
- `SRC/UI/tutorial_v2.py` — added "The Comeback Roll" scene to CHAINS & SCORING lesson; `_draw_comeback_roll` visual renderer shows two puck circles with roll numbers, arrow, combined ring label

## 2026-04-22 — Add mobile logic tests; confirm web sound files present

- `TESTS/test_mobile_logic.py` (new) — 16 tests covering:
  - `TestDeckStatsComputation` (7): face-up excludes from all locs, face-down excludes only at placement loc, letter bucket decrements, game_over reveals all
  - `TestMatchupAndColors` (5): matchup symmetry, all-5 coverage, loc-3 self-pair, opp colors differ from player colors
  - `TestWebBuildSounds` (4): `FOR_WEB` dir exists, all required OGGs present, no empty files, manager filenames match directory
- `ASSETS/SOUNDS/FOR_WEB` — folder rename (was `FOR_WEB (1)`) fixes mobile and desktop web zip audio; no code changes needed

---

## 2026-04-22 — Mobile: opponent slot colors use matchup location colors

- `SRC/UI/hockey_mobile_screen.py` — `_draw_net_and_pairs` now computes `opp_loc_color = LOCATION_COLORS[matchup[loc_id]]` separately from `my_loc_color`, matching the desktop where the opponent border reflects their defensive location (e.g., slot next to your loc 1 gets loc 5's gold border).

---

## 2026-04-22 — Deck-stats tooltip: face-down cards reduce stats at their location

- `SRC/UI/hockey_game_screen.py` + `hockey_mobile_screen.py` — `_compute_opp_deck_stats` now builds a `facedown_here` set alongside `seen_ids`. Face-up cards continue to be excluded from all 5 location counts; face-down cards placed at the queried location are excluded only there (we know their letter at that slot even though the card is hidden).

---

## 2026-04-22 — Deck-stats tooltip: remove title row, shrink box, larger desktop text

- `SRC/UI/hockey_game_screen.py` — removed team name / "X left" title from deck-stats tooltip; bumped row font from 13→18px; box now sized to rows only
- `SRC/UI/hockey_mobile_screen.py` — same title removal; box now sized to rows only

---

## 2026-04-22 — Grinder rule, face-down fog, desktop per-period, deck-stats tooltip

- `SRC/GAME/chain.py` — implemented the **Comeback Roll** (rules.md
  section 6.1, "The Grinder"). `BattleStep` gets `comeback_used` +
  `comeback_won` flags. In `resolve_chain`, every skater-vs-skater
  battle where the worse-rated card lost by exactly 1 LBR gap now
  triggers a second `lbr_lookup` roll; if either roll is a win for the
  underdog, the underdog wins the location. Coach battles + ties +
  gap ≥ 2 battles are unaffected. Gap-1 underdog win rate jumps from
  ~40% to ~64% (verified via 1000-trial smoke).
- `rules.md` — added sections 6.1 Comeback Roll, 6.2 Face-Down
  Resolution, and 6.3 Opponent Deck Stats with the full rule text. §5
  "Deferred Resolution" renamed to "Per-Period Resolution" to match the
  new flow.
- `SRC/UI/hockey_game_screen.py` — **desktop now resolves per-period**
  (mobile already did). Face-down completion fires `_resolve_period()`
  instead of queueing an "Awaiting next period" button or
  `_resolve_all_periods()`. New `_advance_after_resolution()` is called
  on the post-animation click: advances to next period placement, runs
  OT if tied after P3, or shows the final summary.
- **Face-down fog-of-war** — `_draw_opponent_slots` on desktop and
  `_draw_pair_slot` / `_draw_pucks_over_patterns` on mobile keep
  opponent face-down cards hidden until `game_over`. A new
  `_revealed_fd` set (per period × team) tracks which battle locations
  have played during animation; each revealed (period, team, loc) gets
  its LBR puck drawn on the card back, but identity + other-loc values
  stay hidden until game_over. Skip-to-end and the new
  `_reveal_all_resolved_fd` helper keep the fog in sync on fast-forward.
- **Opponent deck-stats tooltip** — hovering (desktop) or dragging over
  (mobile) any location shows how many A/B/C/D/Es the opponent has
  unseen at that location. `_compute_opp_deck_stats` subtracts every
  face-up card the player has seen (plus full reveals at game_over)
  from the opponent's 15-skater roster.
- Tutorials updated:
  - `SRC/UI/tutorial_v2.py` — L2 scene 2 renamed **"Deferred
    Resolution" → "Per-Period Resolution"** with new copy; L4 "Face-Up
    vs Face-Down" now explains that face-down identity stays hidden
    until END OF GAME; new L5 scene **"The Grinder & Deck Stats"**
    covers the Comeback Roll, the fog-of-war, and the deck-stats
    tooltip.
  - `SRC/UI/tutorial_mobile.py` — mobile override for L4 updated to the
    same fog-of-war wording. The new L5 scene flows through
    automatically via the shared LESSONS list.
- `main.md` + `README.md` — updated the "Current State" bullets and the
  "How to Play" steps to describe per-period resolution, Comeback Roll,
  the face-down fog, and the deck-stats tooltip.
- All 60 tests still pass.

## 2026-04-21 — Tutorial polish + mobile puck animation

- `SRC/UI/tutorial_v2.py` — L1 "Meet Your Roster" corrected: border
  colors are **Red = Forward, Blue = Defenseman, Green = Coach, Yellow
  = Selected** (previously said Yellow = Forward, which is actually the
  selected-card state).
- `SRC/UI/tutorial_mobile.py` — introduced `MOBILE_SCENE_OVERRIDES` so
  scenes whose desktop copy references UI that mobile doesn't have get
  mobile-accurate text. Replaced L2 "Deferred Resolution" (mentioned a
  RESOLVE button + speed slider that don't exist on mobile) with
  "Period Playback" and rewrote L4 "Face-Up vs Face-Down" to drop the
  "both teams reveal simultaneously" step (mobile flows straight from
  face-up to face-down with no pause).
- Tutorial visuals bumped to fill the portrait width: per-visual scale
  cap raised from 0.67 to 0.9/1.1 (depending on whether the visual is a
  3-column layout or a single subject), `body_font` from 18→22 px base,
  `small_font` from 15→20 px base. Text-page body font bumped from
  `max(14, 22*s)` to `max(16, 28*s)` (~19 px at mobile scale).
- `SRC/UI/hockey_mobile_screen.py` — ported the PCHC logo puck from
  desktop resolution to mobile. Log entries extended to a 7-tuple that
  carries the attacker team + battle location. New `_tick_puck_anim`
  runs every frame so glide + fade + jiggle stay smooth between log
  ticks; `_apply_entry_side_effects` drives `_puck_step_to` on each
  Pass/Turnover/SAVE/GOAL; `_draw_resolution_pucks` renders the logo
  puck on the "your" slot at the active location with a subtle offset
  + lower alpha for the visiting team. Jiggle on goal, fade-out on
  chain-end.
- All 60 tests still passing.

## 2026-04-21 — Mobile layout config + UI tweaker tool

- `SRC/UI/hockey_mobile_screen.py` — introduced `MOBILE_LAYOUT_DEFAULTS`
  + `_load_mobile_lc()` that merges the `mobile` block of
  `CONFIG/layout_config.json` over the defaults. `self._lc` + new
  `reload_lc()` method push layout changes live. Zone heights, hand
  card width / gaps, action-button pads, pattern-toggle sizes, and the
  5 per-location (x%, y%) offsets are now driven by the config. The
  old pad-based pair-center math was dropped in favor of
  `net_rect.center + (loc{N}_x_pct * net_w, loc{N}_y_pct * net_h)`,
  which lets the tweaker move one slot at a time.
- New `show_*` flags (title_bar, pattern_toggles, period_btn,
  action_button, status_tint, selected_preview, coach_detail,
  opp_cards, fatigue_panels, resolution_log). Each draw method and the
  event handler short-circuit on its flag so the tweaker's "delete"
  toggles can hide elements without tagging individual blits.
- `SRC/UI/capture_surface.py` — `dump_ui_capture` now accepts a
  `mobile_mode` kwarg and writes it into the `capture_info` block, with
  `design_resolution` switching between `[600, 900]` and `[1600, 1000]`
  accordingly. `game_window._dump_ui_elements` passes the flag so
  pressing **U** during a mobile game dumps a properly-labeled JSON.
- New `TOOLS/ui_tweaker_mobile.py` — live portrait preview (left) +
  slider / toggle panel (right). Sliders for zone heights, card sizes,
  action-button / pattern-button dims, plus 10 (x%, y%) sliders for the
  5 locations. 10 visibility toggles drive the `show_*` flags. Ctrl+S
  writes the values into `layout_config.json["mobile"]`, R reloads from
  disk, arrow keys nudge the selected slider (Shift for big steps),
  1/2/3 flip the preview between Face-Up / Face-Down / Resolve phases
  so every UI element can be seen in context.
- All 60 tests still passing; tool smoke-tested (toggle-off + slot
  nudge both update the live preview).

## 2026-04-21 — Mobile tutorial + cup-run simplifications

- `SRC/UI/tutorial_mobile.py` — new `TutorialMobileScreen` that reuses
  `TutorialV2Screen`'s visual renderers via composition and splits each
  scene into two paged portrait screens: the visual first, then the
  writing. 31 pages total across the 5 lessons (14 visual scenes × 2 + 1
  text-only "Advanced Tips" scene). Prev/Back/Next nav bar, page counter,
  progress bar. Scenes with no visual collapse to a single text page.
- `SRC/UI/game_window.py` — when `_mobile_mode`:
  - Tutorial launcher picks `TutorialMobileScreen` instead of v2.
  - `_render_cup_bracket_to` delegates to new
    `_render_cup_bracket_mobile_to` which shows only the current round
    stacked vertically with player series gold-bordered, plus a two-line
    stats blurb (W-L and top scorer). Desktop's "Playoff Stats" button
    and its modal overlay are suppressed on mobile (too dense for 600px
    wide), and the event handler skips the stats click-path.
  - `_draw_cup_newspaper` delegates to `_render_cup_newspaper_mobile_to`
    — a simple white page with a small grayscale player card, a bold
    headline, and a mini article. Template-based newspaper is kept for
    desktop. Grayscale conversion uses `pygame.surfarray` ITU-R 601 luma.
- All 60 tests still passing; smoke-tested bracket + newspaper for
  running / champion / eliminated phases.

## 2026-04-21 — Mobile Phase 4: resolution log + goalie fatigue panel

- `SRC/UI/hockey_mobile_screen.py` — during `PHASE_RESOLVING` the empty
  hand zone now shows two goalie mini-panels (YOU/OPP, 5 rows of save-%
  color-coded by location, ticking up per revealed SAVE/GOAL) plus a
  scrolling resolution log. Timed playback at 0.45 s/entry with a 0.6 s
  goal hold for the horn; first tap fast-forwards, second tap advances
  the period.
- New log format: 6-tuple `(text, color, kind, defender, location,
  goal_team)` so `_apply_entry_side_effects` can tick `_display_fatigue`
  and `_display_t1/t2` as each Pass / Turnover / SAVE / GOAL line is
  revealed — matches desktop's per-shot fatigue animation.
- Title-bar score reads from `_display_t1/t2` during playback so the
  scoreboard increments when the player sees the GOAL line, not when the
  period is resolved under the hood.
- All 60 tests still passing.

Massive day of performance and polish work aimed at making the pygbag web
build viable. Target problem: audio glitches + frame stutter + slow screens
on WASM. Approach: measure, cache, simplify.

### Web build infrastructure
- `TOOLS/build_web.py` — now installs and drives pygbag 0.9.3; steps:
  stages a fresh copy at `~/hockey_web_<timestamp>/`, bundles cards from
  `cards_pchc_named_smaller/`, OGGs from `ASSETS/SOUNDS/FOR_WEB/` (see
  below), runs `pygbag --build`, patches the generated `index.html` with
  seven substitutions (fb dims 1600×1000, aspect 1.6, `gui_divider:1`,
  portrait-mode rotate hint, `force_canvas_size()` resize handler, cache-
  busting meta + SW cleanup, and replacement of pygbag's default
  `canvas.emscripten` CSS with a `100vw/100vh + object-fit: contain`
  version), overwrites pygbag's default favicon with the Jamieson card
  portrait, zips to `WEB_BUILD/hockey_game_web.zip`.
- `pygbag.ini` — fixed for pygbag 0.9.3: renamed `ignore_dirs` →
  `ignoredirs` (no underscore) + added `ignorefiles`, both in Python-list
  syntax `[...]`.
- `WEB_BUILD/favicon.png` — new 256×256 PNG of the Jamieson Felines
  card; `build_web.py` copies it over pygbag's default after build.

### Audio overhaul
- **Asset split**: `ASSETS/SOUNDS/FOR_LOCAL/` (full-quality 224 kbps
  originals) vs. `ASSETS/SOUNDS/FOR_WEB/` (re-encoded at 112 kbps stereo
  via `TOOLS/reencode_sounds_for_web.py` using `soundfile` with stream-
  block reads to avoid libsndfile stack overflow). Runtime
  `SOUNDS_PATH` in `constants.py` resolves to the right dir by
  `sys.platform == "emscripten"`. Also added `SOUNDS_SEARCH_DIRS`
  fallback list so legacy parent lookups still work.
- **Crowd loop promoted from `pygame.mixer.music` to `Sound`** on a
  dedicated channel — kills the streaming OGG decode that was a major
  cause of audio glitching on WASM. Tradeoff: ~85 MB of PCM in RAM.
- **Two-channel layout on web**: `CHANNEL_CROWD = 0` (always looping),
  `CHANNEL_EVENT = 1` (everything else). Desktop stays at 8 channels.
  Priority SFX (goal horn, period organ) set `_priority_busy=True` so
  incidental SFX (whistle, skate) are dropped rather than cutting the
  priority sound. Crowd ducks from `0.25` → `0.08` during priority.
- **Mixer init**: 48 kHz (matches big crowd/snippet files, eliminates
  real-time resample tax), 4096-sample buffer (smaller was glitchy).
- **Removed sounds**: cheer, miss snippets, random crowd snippets, team
  victory .wav songs — all dead weight on the web channel budget.
- **Horn reliability fixes**:
  - `fade_and_play_goal_horn`: `channel.fadeout(300) + channel.play(horn)`
    anti-pattern left a fade state on the channel that attenuated the
    new horn. Replaced with `channel.stop() + channel.play(horn)` at
    volume 1.0.
  - `_play_effect` called `channel.fadeout(length_ms)` with the sound's
    full length — made every priority sound (horn, organ, whistle)
    linearly fade from full to silent over its entire duration. Removed.
  - `play_goal_horn` default volume 0.6 → 1.0.
  - Resolution-time per-goal hold: after firing the horn, push
    `_resolve_timer` negative (scaled with speed slider, 0.4–1.6 s) so
    the next log entry waits and the horn isn't drowned.

### Performance HUD + draw profiler
- New `SRC/UTILS/draw_profiler.py` — `DrawSection("name")` context manager
  accumulates wall-clock time per section; zero overhead when disabled.
- `F` key toggles an on-screen HUD showing FPS, per-section timings
  (`update`, `draw`, `flip`, `sleep`) plus the top 6 `draw` sub-sections.
  Exponentially smoothed (α 0.12 / 0.20).
- All major draw paths wrapped: per-screen sections (`screen:menu`,
  `screen:team_select`, `screen:game_over`, `screen:cup_bracket`,
  `screen:cup_newspaper`, `screen:loading`, `screen:startup_load`,
  `screen:tutorial`) plus gameplay sub-sections (`bg`, `top_bar`,
  `goalie_panels`, `net`, `your_slots`, `opp_slots`, `pattern_lines`,
  `puck`, `period_boxes`, `hand`, `pattern_buttons`, `phase_faceup`,
  `phase_reveal`, `phase_facedown`, `phase_resolution_log`,
  `phase_continue_hint`, `arena_popup`, `card_preview`, `hints`).

### Caching — gameplay layer
- **Pre-scale cache**: `AssetLoader.load_card_image_scaled(file, w, h)`
  memoizes smoothscale results; applied to goalie card in panel, team
  thumbs, three-star thumbs. Plus `HockeyGameScreen._cached_scale` for
  per-screen caching keyed by `id(source_surface)`.
- **Top bar cache**: rebuilds only on score/period change (15 ms → <0.1 ms).
- **Goalie panels cache**: keyed by `(goalie name, image, fatigue_snap,
  save_pcts, dims)`. 13 font.renders per frame → 1 blit (50 ms → <1 ms).
- **Pattern lines cache + bbox**: was full-screen SRCALPHA (~35 ms even
  cached). Now computes a tight bounding box around drawn content and
  renders a small overlay blit (35 ms → ~5 ms).
- **Hand row cache**: full row with LBR pyramid as one **opaque** surface.
  Rebuilds on card/selection changes (40 ms → ~1 ms).
- **Net pre-composite**: opaque composite of `(collage slice + net
  overlaid)`, blits opaquely instead of SRCALPHA (20 ms → ~2 ms).
- **Your-slots plate+name**: merged dark plate + rendered card name into
  one cached surface per `(w, 16, name)` — eliminates 5 font.renders/frame.
- **Arena popup cache** keyed by period state (60 ms → ~1 ms).
- **BitmapFont render cache**: button labels were re-rasterizing per
  button per frame (12+ glyph blits + 2 fills + smoothscale each).
  Cache keyed by `(text, scale, color)`, capped at 256 entries.
- **`_draw_lbr_overlay` / `_draw_hand_lbr_pyramid`**: swapped inline
  smoothscales to `_cached_scale` (5 × 7 = 35 puck scales per frame → 0).
- **Wrapper/plate/empty-slot surface caches**: all the SRCALPHA
  allocations per slot per frame now keyed by dims + state.

### Caching — screen layer
- All non-gameplay screens now cached as **opaque** Surfaces blit in
  one op on cache hit, rebuilt only when inputs change:
  - `menu` (90 ms → opaque blit) — key: menu_level, hints_enabled, dims.
  - `team_select` (400 ms → opaque blit) — key includes a `hover_tuple`
    derived from mouse position so moving within a row doesn't invalidate.
  - `game_over` (203 ms → opaque blit) — key: scroll + score + winner.
  - `cup_bracket` (112 ms → opaque blit) — key: cup state + stats overlay.
  - `cup_newspaper` (80 ms → opaque blit) — key: `id(paper)` + dims.
  - `loading` (100 ms → opaque blit) — static once teams known.
  - `tutorial` (130 ms → opaque blit in `tutorial_v2.py`) — key includes
    button hover states, mode, lesson, scene.
- Hints toggle / toast drawn **live** on top of cached screens so fades
  and clicks stay responsive.
- Modal dialog backdrop: tried SRCALPHA overlay (~30 ms) and
  `BLEND_RGBA_MULT` (~25 ms) — both too slow on WASM. Settled on a
  **localized opaque dark rect** around the panel (panel rect inflated
  40 px) with a gold glow frame. Panel itself cached as opaque surface.
- Toast caching with `set_alpha()` for fade.

### Preload + startup
- **Collage animation removed**: static frame 0 only (saved 23 JPEG
  decodes + ~30 MB RAM; animation cycling didn't change per-frame blit
  cost anyway).
- **Wax-pack smoothscale cached** on menu.
- **Two-stage loader**:
  - `STATE_STARTUP_LOAD` (new) runs `CardPreloader` to warm all card
    images + SFX into caches before the main menu paints. Progress bar +
    current filename on collage background. Music deferred until preload
    completes (user sees a silent first frame instead of sound-with-no-
    visuals).
  - `STATE_LOADING` (team → gameplay transition) now prebuilds the game
    session (`_prepare_game_session`) up front and spends the 1.2 s
    window running warmup `game_screen.draw(scratch)` calls across 5
    frames — spreads the cold-cache cost (was ~300 ms in one frame,
    enough to cut audio) with `asyncio.sleep(1/FPS)` yields between.
    Live progress bar on top of the cached VS screen.

### Web frame loop
- `async_run` swapped `clock.tick(FPS) + asyncio.sleep(0)` for
  `await asyncio.sleep(frame_interval - elapsed)` — `clock.tick` on
  pygbag can busy-wait in the WASM thread; `asyncio.sleep(N)` hands
  those N ms to the browser's event loop, which is when the audio
  thread runs. Dt clamped to `1/FPS` on >100 ms spikes.
- **Web FPS cap dropped from 60 → 24** in `constants.py` (card game is
  static; gives audio 2.5× more CPU).

### UX improvements
- **ESC → main menu** everywhere (was inconsistent). `_menu_level`
  reset to "main", crowd volume restored. No longer quits from menu.
- **Space / Enter = auto-forward** across all gameplay and post-game
  states (`_handle_forward_key`, game-over, cup newspaper, cup bracket).
- **Auto-advance on 3rd face-up**: placing the 3rd face-up card now
  fires the CPU face-up + transitions to REVEAL without clicking Done.
- **Button color tint** on the Done Face-Up button: red at 1 placed
  (penalty), neutral at 2, green at 3. `_draw_image_button` gained a
  `tint=(r,g,b,a)` kwarg; tint surfaces cached per `(tint, w, h)`.
- **Ghost LBR puck preview**: when a card is selected and hovering over
  an empty slot, a translucent copy of the would-be LBR puck renders at
  the slot's puck position so the player can see the outcome before
  committing.
- **REVEAL → first click selects a hand card**: previously the first
  click on a card in REVEAL just dismissed reveal; now it dismisses AND
  selects the card in one action.
- **Hints toggle label**: added "Hints On" / "Hints Off" text next to
  the checkbox (was just a silent checkbox before).

### Dynamic goalie fatigue display
- Goalie panels previously read `goalie.fatigue` directly — but
  `self._gs.resolve_current_period()` mutates that dict synchronously
  (it fully resolves the period before the animation starts), so the
  panels jumped to the final fatigue state in frame 1 and the animation
  played catch-up around them.
- Introduced `self._display_fatigue: {"team1": {loc: count}, "team2":
  {...}}` — a display-only mirror, snapshotted from the goalie's
  pre-resolution state inside `_resolve_period`,
  `_resolve_all_periods`, and `_resolve_overtime` (captured *before*
  the game-state resolves the period).
- Ticks up during playback: when a log entry's text contains `"GOAL"`
  or `"SAVE"` and carries a valid `loc` + attacking-team key, the
  defender's fatigue bumps by 1 at that location.
- Snap-to-final on every skip path: click-during-resolve, space/enter
  forward, and `speed <= 0` (instant) all copy the live
  `goalie.fatigue` into `_display_fatigue` so the end state is
  consistent with the final score.
- `_draw_goalie_panel` now takes `display_fatigue` as a parameter,
  cache key includes it, and save% is recomputed inline as
  `base_save_pct[loc] - display_fatigue[loc] * FATIGUE_PER_SHOT` rather
  than `goalie.get_save_pct()` (which reads live fatigue).

### Horn audibility + pacing
- `fade_and_play_goal_horn`: the old `channel.fadeout(300) +
  channel.play(horn)` pattern left a fade state on the channel that
  attenuated the horn itself. Replaced with `channel.stop() +
  channel.play(horn)` at volume 1.0, clean state every time.
- `_play_effect` had a `channel.fadeout(length_ms)` that fades the
  sound to silent over its full duration — meaning every priority
  sound (horn, organ) started at full volume and linearly faded to
  zero over its own lifetime. Removed; OGGs have natural envelopes.
- `play_goal_horn` default volume 0.6 → 1.0.
- Resolution pacing: after firing a goal horn, push `_resolve_timer`
  negative by a slider-scaled amount (0.4 s at max speed, ~1.6 s at
  min) and `break` the event loop. Gives the horn a clear audible beat
  before the next log entry fires.

### Tests
- All 60 existing pytest tests pass throughout.

### Files touched
- `SRC/UI/game_window.py` — frame loop, HUD, screen caches, state flow,
  space/enter + ESC, warmup pipeline, loading progress bar.
- `SRC/UI/hockey_game_screen.py` — gameplay sub-section caches, button
  tint, ghost puck, auto-advance, forward-key handler, horn hold,
  arena popup cache.
- `SRC/UI/tutorial_v2.py` — screen cache with hover-aware key.
- `SRC/UI/hint_manager.py` — toast cache, modal darken → opaque frame.
- `SRC/UI/bitmap_font.py` — `render()` memoized per `(text, scale, color)`.
- `SRC/UTILS/sound_gen.py` — 2-channel layout, crowd as Sound, removed
  cheer/miss/snippets, horn reliability fixes, `fade_out` anti-pattern.
- `SRC/UTILS/asset_loader.py` — `CardPreloader`, `load_card_image_scaled`,
  `load_collage_frame(index)`, warm-on-demand scale cache.
- `SRC/UTILS/constants.py` — web FPS=24, `FOR_WEB`/`FOR_LOCAL` routing,
  `STATE_STARTUP_LOAD`.
- `SRC/UTILS/draw_profiler.py` (new) — section timing utility.
- `TOOLS/build_web.py` — pygbag orchestration, favicon swap, HTML
  patches.
- `TOOLS/reencode_sounds_for_web.py` (new) — streams OGG decode/encode
  with soundfile at compression_level 0.7 (~112 kbps).
- `pygbag.ini` — fixed for pygbag 0.9.3 API (`ignoredirs`/`ignorefiles`
  as Python lists).
- `ASSETS/SOUNDS/FOR_LOCAL/` (new) + `ASSETS/SOUNDS/FOR_WEB/` (new) —
  split sound asset directory.
- `WEB_BUILD/favicon.png` (new) — 256×256 Jamieson portrait.
- `WEB_BUILD/hockey_game_web.zip` — final upload artifact (~139 MB).

---

## 2026-04-20 — Font-wide bitmap swap + tutorial & game polish

- **BitmapFontWrapper** (`SRC/UI/bitmap_font.py`): `pygame.font.Font`-compatible drop-in. Routes through `AssetLoader.load_font()` and `UIConfig.font()`; above size 16 it renders via the sliced vintage sheet, below it falls back to a real TTF to keep small labels crisp. Added `make_font(size)` helper and swapped every bare `pygame.font.Font(None, size)` call across 6 UI files to use it (old lines kept as `# OLD:` comments for easy revert).
- **Color tint with anti-aliased edges**: two-step `BLEND_RGB_ADD` (lift ink to white) → `BLEND_RGB_MULT` (tint to target color) so anti-aliased gray pixels stay at the pure hue instead of producing a colored haze.
- **Ink-mask the font sheet**: the source PNG's non-ink area carries 175-alpha dark-red pixels; at load time the sheet's alpha is thresholded by luminance so only actual glyph ink survives — eliminates phantom haze around every tinted letter.
- **Explicit `_ROW_CHARS`** row grouping in BitmapFont so per-row baselines are computed from the real 6 rows (not a y-overlap heuristic that was merging `,` + `.` onto their own mini-row and anchoring the period to the comma's descender — the cause of `..` floating above baseline in "Drop of the ..").
- **Size downshift** (`_SIZE_DOWNSHIFT = 4`): bitmap renders internally at `size - 4` so requested sizes visually match the old TTF sizes.
- **Label sizing** on buttons: `_LABEL_MAX_W_PCT = 0.85`, `_LABEL_MAX_H_PCT = 0.45` equalize text height across short vs. long labels; `_LABEL_Y_OFFSET_PCT = 0.07` nudges the label down into the ice-panel sweet spot.
- **Digit + punctuation normalization**: `_scale_group` shrinks `0-9` and `-,.!?:` so their cap-height matches uppercase.
- **Game-window ad-hoc buttons converted** via a new `Button.draw_image_button` classmethod — main menu, difficulty, team select Back / Drop the Puck, Playoff Stats, Game Over, Eliminated. Scroll-arrow text `^`/`v` replaced with `pygame.draw.polygon` triangles so arrows render with the bitmap font active.
- **Tutorial (`tutorial_v2.py`)**:
  - Header "Scene X of 16" shrunk 36→24 and dropped to y=90; scene title 60→44 and to y=130; body font 44→30; heading 44→40; content_y 130→170.
  - Leading whitespace preserved across wrap so "  1." style indents survive word-wrapping.
  - Page 3 (`positions`): card gap 200→320.
  - Page 4 (`net_layout`): card 80→180; LBR circles swapped for actual `puck_A-E.png` sprites at ~34% of card width.
  - Page 7 (`pattern_view`): net 480→620, pattern example changed to `1→3→4→5→2`, each segment colored with the starting location's `LOCATION_COLORS` hue. P1/P2/P3 buttons 40×22 → 68×34.
  - Page 15 (`positional`): slot boxes 80×36 → 150×56; labels tightened onto the same line ("1: F best").
  - Lesson-selector buttons 500×50 → 720×80.
- **Main menu** (`game_window.py`):
  - Subtitle moved y 250→330 (clears the larger "Drop of the .." title), rewritten to `"proverbial ones --- A Hockey Card Game"`.
- **Hockey game screen**:
  - Top score shifted left of the arena strip, vertically centered on the arenas; font 36→28.
  - Arena strip `pbox_y` 30→14 in `layout_config.json`.
  - Goalie first/last name dropped 10 px for breathing room under the card art.
  - Coach "AUTO-LOSE" tag bg 30→38 tall and text y 17→24 so it clears the ability title.
  - Resolution log reverted to raw pygame TTF (`f_gs_log_title = 30`, `f_gs_log_text = 26` in `layout_config.json`); dynamic line-height from the font for tight packing.
  - Arena popup uses a new `_font_arena_title` at 16 instead of the team-name 22.
- **Game-over summary** (`game_window.py`): score 72→54, winner 42→32, team 28→24, period 24→20, button 30→26.
- **Team select**: record y 35→45, coach y 55→68 (clear the team logo thumbnail).
- **Card outlines in hover preview**: the large card blow-up uses `_blit_role_outline` (red F / blue D / green Coach) instead of a flat yellow rect border.
- **Post-tweaker deletions**: removed the "Click anywhere to close" hint on the cup-bracket stats overlay and the "Show gameplay hints" text label (checkbox still clickable).

---

## 2026-04-20 — Bitmap font + image-backed buttons + role-colored card outlines

- **Font slicer**: `TOOLS/slice_font.py` auto-detects 68 glyph boxes in `ASSETS/IMAGES/UI/font.png` via horizontal + vertical projection, writes `ASSETS/DATA/font_glyphs.json` and a debug overlay PNG.
- **BitmapFont** (`SRC/UI/bitmap_font.py`): renders text from the sliced sheet. Per-row baselines inferred from mode of glyph bottoms, so descenders (g/j/p/q/y), dots (i/j/!/?), and comma droop align correctly.
- **Button refactor** (`SRC/UI/components.py`): picks one of `button{5,10,15,20,25}.png` by label length (truncates > 25 chars), scales to caller-requested height with aspect-preserving width, draws label via BitmapFont, hover flash overlay. Backward-compatible constructor — existing color/font kwargs are now ignored.
- **Label cleanups** for chars absent from the font sheet: `"< Back"` → `"Back"` (menu, difficulty_select), `"^ Up"`/`"v Down"` → `"Up"`/`"Down"` (team_select), `"Place Cards (N left)"` → `"Place Cards: N left"` and `"Card X/3 (Face Down)"` → `"Card X/3: Face Down"` (game_screen).
- **hockey_game_screen ad-hoc buttons**: Done Face-Up, Auto-Fill, Next Period, Resolve Period — replaced `pygame.draw.rect` with `_draw_image_button` helper that uses the same scaled-image + BitmapFont look. Click rects now match the drawn image rects via shared `_faceup_done_btn`/`_autofill_btn`/`_next_period_btn`/`_resolve_btn` helpers.
- **Card role outlines**: loaded `{red,blue,green,yellow}_outline.png` from `ASSETS/IMAGES/UI/`. `_blit_role_outline` overlays red on Forward, blue on Defenseman, green on Coach; yellow replaces the role outline on the currently selected hand card. Applied to own placed cards and hand cards; opponent cards keep their location-colored borders.
- Files created: `TOOLS/slice_font.py`, `TOOLS/preview_buttons.py`, `SRC/UI/bitmap_font.py`, `ASSETS/DATA/font_glyphs.json`.
- Files modified: `SRC/UI/components.py`, `SRC/UI/menu.py`, `SRC/UI/difficulty_select.py`, `SRC/UI/team_select.py`, `SRC/UI/game_screen.py`, `SRC/UI/hockey_game_screen.py`.
- Tests still pass (60/60).

---

## 2026-04-17 — UI capture system and one-off tweaker tool

- Press **U** in-game to dump all visible UI elements to `DEBUG_OUTPUTS/ui_capture_*.json` + PNG screenshot
- Each element includes: type, content, bounding rect, center, font size, config key, draw method
- Added `SRC/UI/capture_surface.py`: `CaptureSurface` (blit interceptor) + `CaptureFont` (text logger)
- Added `TOOLS/ui_one_off_tweaker.py`: visual editor to select/edit captured elements, saves changes JSON
- Applied first tweaker changes: `f_menu_title` 72→76, removed version text from menu
- Files created: `SRC/UI/capture_surface.py`, `TOOLS/ui_one_off_tweaker.py`
- Files modified: `SRC/UI/game_window.py`, `SRC/UI/hockey_game_screen.py`, `CONFIG/layout_config.json`

---

## 2026-04-17 — Simulate remaining playoffs after elimination

- When player is eliminated, the rest of the tournament is simulated to completion
- Full bracket is filled in with all round winners through the championship
- Tournament champion shown on bracket screen below "ELIMINATED" banner
- Champion mentioned in the main elimination newspaper article
- New side article ("CHAMPIONS: ...") added to newspaper when eliminated
- `tournament_champion` attribute set for both elimination and player championship paths
- Files modified: `SRC/GAME/stanley_cup.py`, `SRC/UI/game_window.py`, `log.md`

---

## 2026-04-17 — Rename game to "Drop of the .." (aka Dotdotdot)

- Renamed from "DROP THE GLOVES" to "Drop of the .." throughout all files
- Updated: game_config.json, main.py, pygbag.ini, game_window.py, sound_gen.py
- Updated docs: README.md, main.md, rules.md, log.md
- "PCHC" (conference) and "Frozen Faceoff" (tournament) names kept as-is

---

## 2026-04-17 — Fix newspapers reporting "series" for single-game rounds

- Semifinals and Championship are single games, not best-of-3 series
- Added `is_single_game` property to `SeriesState` dataclass
- Updated headlines, score box, articles, and filler text to not say "series" for single-game rounds
- Files modified: `SRC/GAME/stanley_cup.py`
- All 60 tests pass

---

## 2026-04-15 — Major UI/UX Overhaul: Pucks, Patterns, Animation, Polish

### Game renamed to "Drop of the .."
- Title: "Drop of the .." (aka Dotdotdot) · A Hockey Card Game
- Updated: main.py, game_window.py, game_config.json, window title

### NCHC abbreviations replaced with game team names
- All JSON files (skaters, goalies, coaches, arenas, hockey_teams) now use team mascot names as identifiers (e.g., "Ball-O-Devils" instead of "ASU")
- Updated deck.py TEAM_CARD_IMAGES, sound_gen.py TEAM_GOAL_SONGS, all test files
- Scraper directory untouched (still uses NCHC abbreviations)

### Puck sprites replace drawn LBR circles
- Cut 5 individual puck sprites (A-E) from `A_E_pucks_stylized_nb.png` → `puck_A.png` through `puck_E.png`
- LBR ratings on placed cards and hand cards now show puck images instead of colored circles
- Pucks shifted to 60% down on both placed cards and hand card pyramid
- A-E pucks shown on face-down cards too (our cards always show rating)

### Animated PCHC logo puck during resolution
- PCHC logo puck (`puck_logo.png`) follows the active pattern dots during chain resolution
- Two pucks: home team (full opacity, 2x size) and away team (semi-transparent, 2x size)
- Smooth glide animation between locations with ease-out interpolation
- Fade-in at chain start, fade-out at chain end (goal/save/turnover)
- Jiggle effect on goals that decays smoothly
- Puck hidden when speed slider > 70% (too fast for human to follow)
- Works in regulation AND sudden death overtime
- Fixed walkback bug: puck only moves on Pass/Turnover battle entries, not shot result entries
- Fixed new-chain-after-fade: cancels fade-out and fades in fresh at new location

### Pattern line improvements
- Only active pattern shown during resolution (no dimmed inactive clutter)
- Square marker at first location (start of path), circles for the rest
- Gradient: dark at start → transparent at end (shows direction at a glance)
- Step numbers removed from dots (cleaner look)
- Purple color added for P4 bonus patterns
- P4 toggle button appears when a 4th pattern exists
- Resolution starts at max slowness (slider at 5%)

### Card image normalization
- All 154 cards in `cards_pchc_named/` resized to uniform 864×1216 (was mixed 864/3456)
- Card filenames changed from NCHC abbreviations to team names (e.g., `Ball-O-Devils_22_craig_leeds_F.png`)

### Animated collage backgrounds
- 6-frame breathing animation: cards grow/shrink 1-2 pixels with random phase offsets
- Each card anchored to a random corner (TL/TR/BL/BR) for organic feel
- Frame rate: ~5.5 FPS (0.18s per frame), subtle and smooth

### Wax pack card back
- Face-down opponent cards show solid wax pack back
- Face-down own cards show semi-transparent wax overlay (alpha 80)
- Wax pack image on main menu as frame behind buttons (adjustable via UI tweaker)
- card_wax_back.png moved to cards_pchc_named/

### Team songs
- All 9 PCHC teams mapped to unique goal songs (Habitants → Montreal, etc.)

### Cartoon hockey net
- Fixed net image reference: `net_slight_angle.png` → `Cartoon_net.png`
- Tutorial coach card fixed: portrait aspect (not landscape)

### Name changes and roster swaps
- Peter Jamieson ↔ Fenn Stryker swapped (both CC D)
- Michael Bomholt → CC goalie, Nathan Meckley → ASU goalie (swapped with existing goalies)
- Multiple last name renames: Hawthorne→Hastings, Lightning→Loyalist, Whitecap→Windsor, Lockwood→Athol, Broomfield→Broomfie, Ridgeback→Reilly, Gale→Grimsby, Westfall→Wales, Pike→Picton
- DU "Pioneers" renamed to "Mile-High" everywhere

### Letter stamping improvements
- Orange gap fill: right-edge column of previous letter copied to fill transparent gaps (no NAME bleed)
- Per-team Y offsets (4-6px down) to cover template "NAME" text
- Per-player fine-tuning (Steel Magma, Louis DeBiasio, Peter Jamieson)
- Wide letter extra spacing: D, A, K +3px; M, W +2px; N +1px
- Letter boxer tool upgraded with right-side preview panel showing all 26 cropped sprites

### End-game stats: two-column layout
- Team 1 on left, Team 2 on right, side by side with headers
- Mirror match bug fixed: player stats keyed by (team_key, card_id) instead of card_id alone

### Unique player card images
- Each of 135 skaters assigned a unique template (forward_1 through _9, defense_1 through _6)
- Card gallery generated at 432×608 in `ASSETS/IMAGES/CARD_GALLERY/`
- 60 tests passing

---

## 2026-04-10 — PCHC Card Naming, Letter Sprites, Game Polish

- Created `TOOLS/letter_boxer.py` — interactive tool with right-side preview panel showing live cropped letter sprites; click any slot to re-edit
- Created `TOOLS/stamp_names_on_cards.py` — composites player last names onto card ribbons at the angled `-6.9°` ribbon angle using letter sprites
- Generated 153 named cards in `ASSETS/IMAGES/CARDS/cards_pchc_named/` (135 skaters + 9 goalies + 9 coaches)
- Each card has the fake name on the orange banner: `LEEDS`, `HUDSON`, `PHANTOM`, etc.
- Letter sprites: 26 boxes saved in `letter_boxes.json` with axis-aligned rectangles capturing each letter at its natural ribbon angle
- Bug fixes:
  - Card image path was hardcoded to old NHL directory — fixed to `cards_pchc_named/`
  - `TEAM_CARD_IMAGES` in `deck.py` updated to PCHC named coach cards (used as team select thumbnails)
  - Cartoon hockey net path was `net_slight_angle.png` — fixed to `Cartoon_net.png`
  - Banner text changed from "1989-90 O-Pee-Chee Edition" to "PCHC Hockey Card Battle / 2025-26 PCHC Edition", version v1.0.0
  - Removed card "doubling" from `deck.py` — was producing duplicate forwards/defensemen because each card became 2 variants. PCHC has exactly 9F + 6D pre-selected per team, no doubling needed
  - Fixed name positioning bug in goalie/coach loops where boosted scale was used for both letter size AND ribbon position, putting names off-screen on larger templates
- Generated PCHC card collage (`collage_pchc.png`, 1920×1080) tiled from all 36 PCHC card templates
- Created `card_creation.md` — documents local Stable Diffusion workflow for generating 144 unique player illustrations
- All 60 tests passing

---

## 2026-04-09 — PCHC Game JSON Generation + Full Data Pipeline

- Created `generate_pchc_data.py` — generates all game JSON files from NCHC CSV data
- Produces 6 JSON files in `ASSETS/DATA/`:
  - `skaters.json` — 135 skater cards (9F/C + 6D per team) with LBR values, fake names, card images
  - `goalies.json` — 9 goalie cards with 5-location save percentages
  - `coaches.json` — 9 coach cards with unique PCHC-themed abilities
  - `arenas.json` — 9 arenas with boon/penalty effects, stadium images, preset patterns
  - `nhl_teams.json` — 9 teams with logos, rosters (forward/defense/goalie IDs), records
  - `schedule.json` — 223 regular season + 11 playoff games
- Team logos referenced in `nhl_teams.json` for team select screen (instead of manager cards)
- Each team has themed coach ability (Devil's Advocate, Cat's Pounce, Mile-High Altitude, etc.)
- Each arena has themed boon/penalty (Devil's Furnace, Thin Air, Highland Charge, etc.)
- LBR values generated from real NCHC stats: points/game, goals/game, +/- for quality scoring
- Goalie save % generated from real GAA and SV% with location variation (five-hole weakest)

---

## 2026-04-09 — NCHC Data Scraper + Roster Positions for PCHC Reskin

- Created `ASSETS/IMAGES/CARDS/SCRAPER_SCRIPT/PCHC/scrape_nchc_stats_2025.py`
- Scrapes 2025-26 NCHC season data from nchchockey.com using Selenium (JS-rendered site)
- Scrapes all 9 team stats pages for skater and goalie individual stats
- Scrapes standings page (parses combined W-L-T and GF-GA columns)
- Scrapes full schedule with proper team/score extraction from nested spans
- Filters out exhibition games, detects OT results
- Output CSVs: `nchc_teams.csv` (9 teams with reskin names/mascots), `skaters.csv` (238 players), `goalies.csv` (24 goalies), `schedule_regular.csv` (223 games)
- Team reskin mapping: ASU→Ball-O-Devils, CC→Felines, DU→Mile-high College, MU→RedTurtles, UMD→Poodles, UND→Fighting Scots, UNO→Red Cows, SCSU→Habitants, WMU→The Mutton Busters
- Saves debug HTML for each page to `debug_html/` for troubleshooting
- Note: NCHC site doesn't provide player positions or goalie W/L — these will need to be inferred or sourced separately
- Dependencies: `pip install selenium webdriver-manager beautifulsoup4 lxml`
- Created `ASSETS/IMAGES/CARDS/SCRAPER_SCRIPT/PCHC/scrape_rosters.py`
- Scrapes player positions from 9 different team athletic websites (each with different HTML format)
- Handles 3 Sidearm Sports format variants: card-based (ASU), list-based (CC/DU/UMD/UND/WMU), list-card (UNO), plus table-based (MU/SCSU)
- 240 roster entries across 9 teams, 231/238 skaters matched with positions (F/D/C/G)
- 7 unmatched: likely players on stats but not current roster
- Auto-merges positions into existing skaters.csv
- Manually extracted playoff bracket from PDF into `schedule_playoffs.csv` (11 games, QF/SF/Championship)
- Denver wins 2026 NCHC Frozen Faceoff Championship 4-3 (2OT) over Minnesota Duluth

---

## 2026-04-06 — Web Build, OGG Audio, Playoff Stats, Fixes

- Built web version via pygbag 0.9.3 (Step 12 complete)
- Converted all 20 sound files (WAV/MP3) to OGG format (56MB -> 15.6MB at q7 quality)
- Sound manager tries OGG first (from ASSETS/SOUNDS/OGG/), falls back to original
- Web audio best practices: `pre_init_mixer(44100, 16, 2, 4096)` before `pygame.init()`
- Crowd ambience uses `pygame.mixer.music` (streaming) instead of `Sound` (memory)
- Crowd ducking when effects play, 4 mixer channels on web, no organ on web
- Added `GameWindow.async_run()` for pygbag async game loop
- Emscripten detection: fixed 1600x1000 display on web (no RESIZABLE)
- Created web `main.py` entry point with red error screen for debugging
- Favicon changed from default to card pack (wax wrapper) image
- Playoff stats panel: "Playoff Stats" button on bracket screen shows accumulated stats overlay throughout entire campaign (not just on elimination)
- Fixed stats spilling onto bracket: rink border redrawn after clip release, stats_scroll reset on transition
- Fixed bracket playoff leaders using shared stats_scroll (now fixed position)
- Updated `rules.md`, `README.md`, `main.md` with web build and playoff stats docs
- `pygbag.ini` updated for hockey (title, package name, correct ignore_dirs)

---

## 2026-04-06 — Tutorial, Hints, Scroll Arrows, Stats Clipping

- Rewrote `SRC/UI/tutorial_v2.py` with 5 hockey-specific lessons (16 scenes): card types, net layout with actual net image, LBR ratings, chain building, shot resolution, face-up/face-down, boon/penalty, coach trade-off, goalie fatigue, positional strategy
- Created `SRC/UI/hint_manager.py`: toast/modal hint system with first-time hints, idle hints (8s timer), resolution event toasts, campaign hints; prefs persist to `~/.hockey_card_prefs.json`
- Rewrote `CONFIG/hints_config.json` with 38 hockey hints across 6 categories (first_time, per_period, contextual, on_resolution, on_game_end, campaign)
- Added "How to Play" button and "Show gameplay hints" toggle to main menu
- Added hints toggle to bracket, newspaper, and game over screens
- Idle hints during placement: suggest clicking cards/locations, mention period boxes for boon/penalty info
- Idle hints on team select: guide Home/Away selection after 8s inactivity
- Re-enabling hints clears shown_hints so first-time hints replay
- Fixed coach card aspect ratio in tutorial (landscape for team cards)
- Fixed win probability bar text to black for contrast on colored bars
- Fixed special characters in arena popup and data (em-dash, arrows, star → ASCII)
- Added scroll arrow buttons (^ v) to team select and game over stats screens for web/touch
- Added clip rect to game over stats to prevent content spilling over buttons/borders
- Updated `rules.md`, `README.md`, `main.md` with tutorial/hints documentation

---

## 2026-04-06 — Rewrite HintManager for Hockey

- Rewrote `SRC/UI/hint_manager.py` from baseball version to hockey-oriented hint system
- Changed prefs file from `.baseball_card_prefs.json` to `.hockey_card_prefs.json`
- Simplified prefs format to `hints_enabled` + `shown_hints` (removed legacy compat fields)
- Added `enabled` constructor parameter, `set_enabled()` method, `is_modal_active` property
- Robust condition parser: supports `==`, `!=`, `<=`, `>=`, `<`, `>`, `&&` compounds, booleans
- Removed `scale` parameter from draw methods (no longer needed)
- Removed unused `DESIGN_H` global constant
- Context keys now support hockey concepts: period, chain_len, location, coach_played, etc.

---

## 2026-04-06 — Comprehensive Documentation Update (v0.5.0)

- Updated `rules.md`, `README.md`, `main.md` to reflect all implemented features
- Bumped version to v0.5.0 Alpha
- Coach cards documented as placeable (auto-lose at location, green border, LBR=E)
- Hand sizes corrected: 5F+3D+coach=9 start, 3F+2D=5 draw between periods
- Goalie fatigue corrected to -10% per shot (matching code)
- Added deferred resolution description (place all 3 periods, then resolve)
- Added card doubling (offensive/defensive variants)
- Marked Steps 8-10 as DONE in build plan (deferred resolution, end-game stats, Road to Cup)
- Added sections for AI difficulty levels, end-game stats/Three Stars, Road to the Stanley Cup
- Updated architecture with new files: game_stats.py, stanley_cup.py, collages, newspaper tools
- Removed 3 obsolete baseball test files (test_card.py, test_ai.py, test_resolution.py)
- All 68 tests pass

---

## 2026-04-06 — Rename RPS to LBR in Documentation Files

- Updated all RPS/RPS_p references to LBR (Level Battle Rolls) in markdown documentation
- Files updated: `main.md`, `rules.md`, `README.md`, `CLAUDE.md`
- `log.md` historical entries left unchanged (only forward-looking refs updated)
- Preserved original spec files in `ASSETS/DATA/` as-is
- Examples: "Rock-Paper-Scissors (RPS)" → "Level Battle Rolls (LBR)", `rps.py` → `lbr.py`, `test_rps.py` → `test_lbr.py`, "RPS boost" → "LBR boost"

---

## 2026-04-06 — Rename RPS to LBR (Level Battle Rolls)

- Renamed all references from RPS/RPS_p to LBR (Level Battle Rolls) across the entire codebase
- Renamed `SRC/GAME/rps.py` to `SRC/GAME/lbr.py`, function `rps_lookup` to `lbr_lookup`
- Renamed `TESTS/test_rps.py` to `TESTS/test_lbr.py`, updated all test function names and imports
- Updated `SRC/GAME/card.py`: `RPS_LETTERS` to `LBR_LETTERS`, `get_rps` to `get_lbr`, `get_rps_letter` to `get_lbr_letter`, `avg_rps` to `avg_lbr`
- Updated `SRC/GAME/chain.py`: `attacker_rps`/`defender_rps` fields to `attacker_lbr`/`defender_lbr`, import updated
- Updated `SRC/GAME/ability_effects.py`: all `rps_*` field names to `lbr_*` (e.g., `rps_global` to `lbr_global`, `rps_card_boosts` to `lbr_card_boosts`), ability type strings `"rps_boost"`/`"rps_penalty"` to `"lbr_boost"`/`"lbr_penalty"`
- Updated `SRC/GAME/hockey_ai.py`, `SRC/GAME/ai_evaluation.py`, `SRC/GAME/ai_player.py`: all RPS references to LBR
- Updated all UI files: `hockey_game_screen.py`, `game_screen.py`, `tutorial_v2.py`, `tutorial_screen.py`, `loading_screen.py`, `game_components.py`, `capture_anchors.py`
- Updated JSON data files: `ASSETS/DATA/arenas.json`, `ASSETS/DATA/coaches.json` (ability types and descriptions)
- Updated CONFIG files: `CONFIG/layout_config.json`, `CONFIG/hints_config.json`, `CONFIG/dialog_config.json`
- Updated all test files: `test_chain.py`, `test_ability_effects.py`, `test_hockey_card.py`, `test_card.py`, `test_ai.py`, `test_resolution.py`
- Updated tools: `TOOLS/ui_tweaker.py`, `TOOLS/restore_custom_json.py`
- Updated generators: `generate_hockey_data.py`, `generate_team_data.py`
- Pure naming refactor: no game logic changes

---

## 2026-04-03 — v0.4.0: RPS_p, 3 AI Bots, Deferred Resolution, Monte Carlo

### RPS_p Combat System
- Replaced circular 5-point star RPS with percentage-based system
- Better cards genuinely win more: gap 1=60%, gap 2=75%, gap 3=85%, gap 4=95%
- No guaranteed outcomes — every matchup has upset potential
- Ties still favor offense

### Three AI Bot Levels
- **Rookie (Jack Jamieson)**: random with mild position heuristics
- **Veteran (Chris Terrien)**: greedy best-match, counter-places vs opponent reveals
- **All-Star (Gretzky)**: chain-value evaluation, goalie targeting, strategic face-up based on hand strength
- Two-phase placement: bots choose face-up, see opponent reveals, then counter-place face-down
- Menu difficulty selection wires to bot choice

### Deferred Resolution
- Place all 3 periods before resolving (strategic: hide players across periods)
- "Next Period" button between periods
- Resolve all 3 at once with animated log playback
- Speed slider (Slow to Fast) controls playback rate
- Score revealed incrementally as log plays
- Period view switches automatically during playback
- Active pattern highlighted, others semi-transparent

### Monte Carlo Season Simulator
- `TOOLS/MONTE_CARLO_TESTER/run_hockey_season.py` with two-phase bot placement
- 6 matchup combos: AvA, BvB, CvC, AvB, BvC, CvA (10 reps each)
- Results: +0.65-0.87 correlation with real 1989-90 standings
- BvC: 11/21 teams within 3 wins, avg 5.4 win diff
- `TOOLS/restore_custom_json.py` to recover arenas/coaches after data regeneration

### Stat Rebalancing
- RPS distribution retuned: bell-curve centered on C
- Goalie save % raised (avg ~79%)
- French accented name matching fixed for card images (270 skaters with images)
- Team quality modifier removed — RPS_p handles quality naturally

### UI Improvements
- Auto-fill button with weighted face-up count
- Landscape team cards in team select (correct aspect ratio)
- Coach cards displayed with correct aspect ratio
- Semi-transparent card wrapper overlay on face-down player cards
- Period boxes render in front of net
- Resolution log word wrapping
- Window resize support
- 60+ tweaker parameters including pattern lines, buttons, RPS circles

### Arena Boon Rebalance
- Converted several RPS-boost boons to chain-level boosts (rewards winning battles, not flat quality)
- Detroit: all chains +1 length, Calgary: chain 2+ extra -10% goalie, Boston: chain boost at loc 4/5

---

## 2026-04-03 — Overtime, Arena Visuals, Arena Info Popup

### Overtime System
- When tied after 3 periods, game enters **overtime**
- Cycles through existing P1/P2/P3 placements (round-robin) with the corresponding arena
- Each OT round generates **3 fresh random patterns** (not the arena presets)
- First OT round with a **goal differential** ends it — winner gets **+1 goal**
- Goalie fatigue continues accumulating across OT rounds
- Safety cap at 20 OT rounds (virtually impossible to hit)
- Top bar shows "OVERTIME" in gold during OT
- Resolution log shows OT rounds with source period info

### Arena Visuals
- **Brighter backdrop**: reduced dim overlay from alpha 160 → 80 (arena photos much more visible)
- **Semi-transparent net**: net image alpha set to 160 so arena shows through behind it
- Arena photo now clearly visible as the background during gameplay

### Arena Info Popup
- **Clickable arena name** in the top bar
- Click opens a centered popup showing:
  - Arena name in gold
  - Preset pattern sequence
  - BOON (3 face-up): name + word-wrapped description in green
  - PENALTY (1 face-up): name + word-wrapped description in red
- Click anywhere to dismiss
- Dark semi-transparent background with gold border

### Files Modified
- `SRC/GAME/game_state.py` — added `resolve_overtime()`, `in_overtime`, `ot_round` tracking
- `SRC/GAME/resolution.py` — OT periods (num > 3) use 3 random patterns instead of preset
- `SRC/UI/hockey_game_screen.py` — OT resolution UI, arena popup, brighter backdrop, transparent net

---

## 2026-04-03 — Coach Cards: Placeable Ability Cards with Auto-Lose

### Coach Mechanic
- Coach is now a **playable card** in the hand alongside skaters
- Place it at any location — **auto-loses** the battle there
- Activates a **powerful ability** for the entire period (3+ patterns)
- **One use per game** — once placed, removed from hand permanently
- Strategic trade-off: sacrifice one location for a period-wide advantage

### Data (`ASSETS/DATA/coaches.json`)
- Rewrote all 21 coaches with effects-based structure
- Each coach has `effects` array with typed abilities (same system as arenas)
- Examples: Mike Keenan "Iron Mike" (all +1 RPS), Pat Burns "Defensive Mastermind" (+15% goalie, cap opponent chains), Glen Sather "Dynasty Wisdom" (free first step except coach loc)

### CoachCard Model (`SRC/GAME/card.py`)
- CoachCard now duck-types with SkaterCard: has `get_rps()`, `get_rps_letter()`, `avg_rps()`, `position`, `image_file`, `is_coach`
- Added `PlaceableCard = Union[SkaterCard, CoachCard]` type alias
- Added `is_coach = False` to SkaterCard for easy checking

### Deck (`SRC/GAME/deck.py`)
- Coach now starts in hand with skaters (hand = 11 cards: 6F + 4D + 1 coach)
- `coach_played` flag tracks one-time usage
- `play_cards()` sets `coach_played` when coach is placed

### Coach Effects (`SRC/GAME/ability_effects.py`)
- Added `build_coach_effects()` — converts coach JSON effects into ArenaEffects
- Added `merge_effects()` — combines arena effects with coach effects (additive)
- Supports conditional effects: trailing, not_trailing, trailing_period_3, score_first
- New ArenaEffects fields: `cascade_debuff_on_all_wins`, `tie_advantage_all`, `fatigue_immunity`, `chain_3_plus_extra_modifier`, `opponent_rps_penalty_locations`, `branch_except_coach_loc`, `coach_location`

### Chain Engine (`SRC/GAME/chain.py`)
- Coach at attacker location → auto-lose (chain breaks)
- Coach at defender location → auto-win for attacker
- Free first step respects coach location exception (Sather's ability)
- Branch respects coach location exception (Murray's ability)
- Cascade debuff: every win → opponent -1 RPS next battle (Holmgren)
- Tie advantage on all battles (Brophy), not just first
- Chain 3+ extra goalie modifier (Bergeron)
- Global RPS modifier from coach (`rps_global`)

### Resolution & Game State
- `resolve_period()` now accepts coach info and merges effects
- `game_state.py` finds coach in placements, checks disabled flags, passes to resolution
- Coach disabled by arena penalty (Ballard's Curse) prevents activation

### AI (`SRC/GAME/hockey_ai.py`)
- Bot places coach at highest-numbered open location (least chain disruption)
- Coach excluded from face-up selection (placed face-down)

### UI (`SRC/UI/hockey_game_screen.py`)
- Coach card in hand shows: gold "★ COACH" label, ability name, "AUTO-LOSE" warning
- Distinct brown tint on fallback (no image) coach cards
- Team card images used as coach card art

### Tests
- 67 tests pass (added coach model tests, updated hand size assertions)

### Files Modified
- `ASSETS/DATA/coaches.json` (rewritten)
- `SRC/GAME/card.py`, `SRC/GAME/deck.py`, `SRC/GAME/ability_effects.py`
- `SRC/GAME/chain.py`, `SRC/GAME/resolution.py`, `SRC/GAME/game_state.py`
- `SRC/GAME/hockey_ai.py`, `SRC/UI/hockey_game_screen.py`
- `TESTS/test_game_state.py`, `TESTS/test_hockey_card.py`

---

## 2026-04-03 — Arena Locations: Boon/Penalty System + 3-Arena-Per-Game

### Arena Data Overhaul
- Rewrote `ASSETS/DATA/arenas.json` — all 21 NHL arenas now have boon (3 face-up), penalty (1 face-up), preset pattern, image_file, and team_id
- Each arena has unique named abilities (e.g. "Small Ice", "Brass Bonanza", "The Trap")

### ArenaCard Model (`SRC/GAME/card.py`)
- Replaced `style`, `ability_name`, `ability_text` with `boon` dict, `penalty` dict, `image_file`, `team_id`

### Arena Effects Engine (`SRC/GAME/ability_effects.py`)
- Complete rewrite from baseball stadium tiers to hockey boon/penalty system
- `determine_tier()` — 3 face-up = boon, 2 = neutral, 1 = penalty
- `build_arena_effects()` — converts arena JSON + face-up count into flat `ArenaEffects` struct
- Supports all effect types: RPS boosts/penalties, chain caps, branch mechanic, chain skip, free first step, tie advantage, goalie modifiers, fatigue multipliers, save rerolls, extra patterns, shot boosts, auto-goal, coach disable, opponent debuffs

### Chain Engine (`SRC/GAME/chain.py`)
- `resolve_chain()` now accepts `ArenaEffects` for both attacker and defender
- Implements: chain cap, chain skip (auto-lose first N steps), free first step (auto-win step 0), branch on break (Hartford/Toronto), per-card RPS boosts, per-pattern penalties, first-battle modifiers, tie advantage, opponent debuff propagation
- `resolve_shot()` applies goalie modifiers, fatigue multiplier, shot location boosts, auto-goal

### Pattern Engine (`SRC/GAME/pattern.py`)
- Added `add_extra_pattern()` — generates 4th pattern (random or reverse of P1) for arena boons

### Resolution Engine (`SRC/GAME/resolution.py`)
- `resolve_period()` now builds `ArenaEffects` per team from face-up count
- Passes effects into chain/shot resolution
- Handles extra pattern generation (trailing, trailing 2+, score-first conditions)
- Implements save reroll (Montreal boon)
- `PeriodResult` now includes arena and effects metadata

### 3-Arena-Per-Game (`SRC/GAME/game_state.py`)
- `HockeyGameState` now takes 3 arenas: team1 home (P1), team2 home (P2), neutral (P3)
- `period_arenas` dict maps period number to arena
- `.arena` property returns current period's arena
- `select_neutral_arena()` picks random arena excluding both teams' homes
- Tracks `coach_disabled_game` flag (Ballard's Curse)

### UI — Arena Backdrop (`SRC/UI/hockey_game_screen.py`)
- Added `_draw_arena_backdrop()` — loads stadium photo, dims with overlay, draws behind net
- Top bar now shows arena name alongside period indicator
- Uses existing `AssetLoader.load_stadium_image()` + `ASSETS/IMAGES/STADIUMS/` photos

### Game Window (`SRC/UI/game_window.py`)
- `_start_game()` now builds 3 ArenaCards (team1 home, team2 home, random neutral)
- Passes all 3 to HockeyGameState constructor

### Tests
- Rewrote `TESTS/test_ability_effects.py` — 27 new tests covering all arena boon/penalty types
- Updated `TESTS/test_hockey_card.py` — ArenaCard test checks boon/penalty/image_file
- Updated `TESTS/test_game_state.py` — full game tests use 3-arena constructor
- All 66 tests pass (3 old baseball tests remain excluded: test_ai, test_card, test_resolution)

### Files Modified
- `ASSETS/DATA/arenas.json` (rewritten)
- `SRC/GAME/card.py`, `SRC/GAME/ability_effects.py` (rewritten), `SRC/GAME/chain.py` (rewritten)
- `SRC/GAME/pattern.py`, `SRC/GAME/resolution.py` (rewritten), `SRC/GAME/game_state.py` (rewritten)
- `SRC/UI/hockey_game_screen.py`, `SRC/UI/game_window.py`
- `TESTS/test_ability_effects.py` (rewritten), `TESTS/test_hockey_card.py`, `TESTS/test_game_state.py`

---

## 2026-04-02 — UI Polish: Theme, Tweaker, Game Flow, Visual Feedback

### OPC Ice Rink Theme
- Created `SRC/UI/opc_theme.py` — ice texture backgrounds, rink board borders (red/blue lines), gold accents
- Menu: collage-dimmed ice background, rink border frame, difficulty sub-menu, responsive scaling
- Team Select: scrollable rows with team card thumbnails (correct aspect ratio), coach names, W-L-T records, two-column picker, "Drop the Puck!" button
- Loading screen: team names in blue/orange with "vs", red center line, "Dropping the puck..."

### Net Image & Card Layout
- Replaced drawn red-line net with actual `net_slight_angle.png` photo
- Cards placed inside the net in pyramid layout (1,2 top — 5 center — 3,4 bottom)
- Both player and opponent cards side-by-side at each location inside the net
- Empty slots: white semi-transparent with thick **location-colored borders** (no text)
- Placed cards: **D = red border (3px), F/C = yellow border (4px)**

### Face-Up Reveal Flow
- Phase 1: Player places 1-3 cards face-up
- CPU places matching number of face-ups
- **REVEAL phase**: Both teams' face-up cards shown simultaneously — opponent's face-up cards visible
- **RPS rating overlay**: Large letter (A-E) centered on face-up cards, color-coded by grade
- Phase 2: Player places remaining cards face-down (informed by opponent's reveals)

### Location Color System
- 5 distinct colors: High Glove=blue, High Blocker=orange, Low Glove=green, Low Blocker=purple, Five Hole=gold
- Used consistently on: hand card RPS tags, net slot borders, goalie save % labels

### Card Doubling (Offensive + Defensive Variants)
- Each real card becomes two game cards with shifted RPS values
- Offensive variant: better at locations 1,2,5; weaker at 3,4
- Defensive variant: better at locations 3,4; weaker at 1,2
- Rosters only include cards with actual O-Pee-Chee images

### Sound Integration
- Crowd ambience loop (5-min hockey crowd track)
- Team-specific goal horns (CGY, EDM, MTL, NYI, TOR, VAN, WIN + generic)
- Whistle, shot, skate scrape, organ sounds

### UI Tweaker Tool (TOOLS/ui_tweaker.py)
- D key cycles preview through all screens (Menu, Team Select, Loading, Game)
- Per-screen sliders: layout parameters + individual font sizes for every text element
- Active slider highlights its region on the preview in red
- Live font preview — drag font slider, see text update instantly
- Ctrl+S saves to `CONFIG/layout_config.json`, game reads on startup
- Show-all mode (S key) for layout debugging

### Layout Config System
- All UI values read from `CONFIG/layout_config.json`
- `_P(key, default)` pattern in game_window.py, `_lc` dict in hockey_game_screen.py
- Fixed card aspect ratio (0.714 = 5:7) used everywhere
- `TOOLS/ui_overlap_check.py` validates layout at 5 resolutions

### Files Created
- `SRC/UI/opc_theme.py`, `SRC/GAME/hockey_ai.py`

### Files Modified
- `SRC/UI/game_window.py` (full rewrite: hockey theme, team select, config-driven)
- `SRC/UI/hockey_game_screen.py` (net image, paired slots, reveal flow, location colors, position borders)
- `SRC/GAME/card.py` (offensive/defensive variants)
- `SRC/GAME/deck.py` (image-only filtering, card doubling, team card images for coaches)
- `SRC/UTILS/constants.py` (CARD_ASPECT)
- `SRC/UTILS/sound_gen.py` (hockey sound manager)
- `TOOLS/ui_tweaker.py` (full rewrite: per-screen, per-element fonts)
- `TOOLS/ui_overlap_check.py` (hockey version)

---

## 2026-04-02 — Playable Hockey Game MVP (Phases 1-8 Complete)

- **Phase 1**: Core card models — `SRC/GAME/card.py` (SkaterCard, CoachCard, ArenaCard), `SRC/GAME/goalie.py` (GoalieCard with fatigue)
- **Phase 2**: Chain & pattern engines — `SRC/GAME/chain.py` (chain resolution + shot on goal), `SRC/GAME/pattern.py` (preset + random patterns)
- **Phase 3**: Game logic — `SRC/GAME/deck.py` (HockeyDeck: 9F+6D+1G+1C), `SRC/GAME/resolution.py` (period resolution), `SRC/GAME/game_state.py` (3-period game state)
- **Phase 4**: Constants & data — `SRC/UTILS/constants.py` (hockey config), `SRC/UTILS/asset_loader.py` (CardDatabase for hockey JSON)
- **Phase 5-7**: UI — `SRC/UI/game_window.py` (menu, team select, loading, game over), `SRC/UI/hockey_game_screen.py` (pyramid net layout, card placement, resolution log)
- **Phase 8**: AI — `SRC/GAME/hockey_ai.py` (SimpleBot: forwards to offensive locs, D to defensive)
- **Card images**: 330/330 O-Pee-Chee cards scraped, 247 skaters + 30 goalies matched to stats data
- **Arena photos**: 89 images across all 21 arenas from Wikipedia/Wikimedia Commons
- 35 passing tests across card models, chain resolution, pattern generation, deck building, and full game state
- Files created: `card.py`, `goalie.py`, `chain.py`, `pattern.py`, `deck.py`, `resolution.py`, `game_state.py`, `hockey_ai.py`, `hockey_game_screen.py`
- Files modified: `main.py`, `game_window.py`, `constants.py`, `asset_loader.py`

---

## 2026-04-02 — Scraped 330 O-Pee-Chee Card Images + Mapped to Data

- Created `scraper_opc_checklist.py` — scrapes all 330 cards from TCDB checklist pages
- Created `map_cards_to_data.py` — matches card images to skaters.json/goalies.json by name
- Matched 247 skaters + 30 goalies to card images; 53 unmatched (team cards, checklists, minor players)
- Updated skaters.json and goalies.json with image_file references
- Output: `opc_cards.csv` (330 cards), `unmatched_opc_cards.csv` (53 unmatched)

---

## 2026-04-02 — Generated Game JSON Data from Scraped Stats

- Created `generate_hockey_data.py` — converts scraped CSVs into game JSON files
- Skater RPS values generated from real stats: goals/assists/points/+−/shots weighted by position (forwards strong at locs 1-3, D strong at 4-5)
- Goalie save percentages generated from GAA/SV%/SO, with realistic location variation (five-hole weakest, blocker side strongest)
- 21 coach abilities hand-authored (one per team, e.g. Burns "Defense First" +15% goalie, Keenan "Iron Mike" +2 RPS)
- 21 arena abilities + preset patterns by team style (offensive/defensive/balanced/cycle/counterattack)
- Full schedule included: 840 regular season + 85 playoff games + 15 playoff series
- Output: skaters.json (716), goalies.json (76), coaches.json (21), arenas.json (21), nhl_teams.json (21), schedule.json
- Files created: `generate_hockey_data.py`

---

## 2026-04-02 — Scraped 1989-90 NHL Season Stats from Hockey-Reference

- Created `scrape_nhl_stats_1990.py` — fetches team standings, skater stats, and goalie stats
- Fixed Hockey-Reference `data-stat` column names (`name_display`, `team_name_abbr`, `goalie_games`, etc.)
- Matched team abbreviations to Hockey-Reference conventions (HAR, MNS, WIN not HFD, MIN, WPG)
- Handles traded players (`2TM`/`3TM` combined lines)
- Output: `nhl_teams.csv` (21 teams), `skaters.csv` (775 players), `goalies.csv` (80 goalies)
- Also fixed `scraper_opeechee_1989_90.py`: updated title regex for O-Pee-Chee, removed hardcoded Chrome version
- Files created: `scrape_nhl_stats_1990.py`
- Files modified: `scraper_opeechee_1989_90.py`

---

## 2026-04-02 — Project Rebrand: Baseball → Hockey (1989-90 O-Pee-Chee)

- Rebranded entire project from Baseball Card Battle (1989 Upper Deck) to Hockey Card Battle (1989-90 O-Pee-Chee)
- Updated CLAUDE.md: spec references now point to `hockey_main.md` and `hockey_rules.md`, venv renamed to `hockey_card`, build order updated for hockey migration path
- Rewrote README.md for hockey identity: chain battles, 5 goalie-hole locations, 3 periods, goalie fatigue, coach abilities, O-Pee-Chee theme
- Updated `CONFIG/game_config.json`: renamed innings→periods, players/pitchers→skaters/goalies/coaches/arenas, added goalie fatigue config, location names (High Glove, High Blocker, etc.), matchup mapping
- Updated `hockey_main.md`: all "1989 Topps" references changed to "1989-90 O-Pee-Chee", image dir to `1989_opc_hockey`, theme file to `opc_theme.py`
- Original baseball spec (`main.md`) retained for reference during migration
- Files modified: `CLAUDE.md`, `README.md`, `CONFIG/game_config.json`, `hockey_main.md`, `log.md`
