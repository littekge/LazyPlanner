# TZID-Anchored Recurrence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Store a recurring item's recurrence anchor as local-time-with-TZID (plus a matching VTIMEZONE) instead of UTC, so a recurrence rule is interpreted in the same zone the user authored it in.

**Architecture:** LazyPlanner derives a rule's `BY*` parts from the user's local anchor but serializes `DTSTART`/`DUE` in UTC. RFC 5545 evaluates `BY*` in the anchor's own zone, so the two disagree whenever an item's local date differs from its UTC date — every recurring evening event in the Americas, every after-midnight event in Europe. Writing the anchor as `DTSTART;TZID=<IANA>:<local wall time>` makes the rule and its anchor agree by construction. The read side already resolves TZID via the embedded IANA database and needs no change (verified). Three changes are needed on the write side: resolve the host's IANA zone name (Go only exposes it when `$TZ` is set), generate a conformant VTIMEZONE for the zones we reference, and switch the anchor's serialization — but **only when we are already authoring the rule**, so an existing series' occurrence set is never silently re-interpreted.

**Tech Stack:** Go, `emersion/go-ical`, `teambition/rrule-go`, Go's embedded tzdata (already embedded in `cmd/lazyplanner`).

## Global Constraints

- `internal/model` does **no I/O** — the zone-name resolver therefore lives in `internal/config`, not `internal/model`.
- Only `internal/ui` imports tview/tcell.
- `gofmt` is law; imports in goimports order (stdlib → third-party → project).
- Full gate every commit: `go test ./...`, `go vet ./...`, `staticcheck ./...`, `go build ./...`.
- Zone-sensitive tests run over several zones — at minimum `UTC`, `America/New_York`, `Europe/Berlin`, `Asia/Kolkata` (no DST), `Australia/Sydney` (southern hemisphere). Run `TZ=Asia/Kolkata go test ./...` before claiming done.
- **Iron rule**: never drop or mangle iCal properties the app doesn't understand. Adding a VTIMEZONE is additive; an existing foreign VTIMEZONE is never removed or rewritten.
- A regression test for a UI-reachable bug **must drive the real entry point** (the form/handler), not the helper in isolation.
- Every existing-resource write stays on `store.PutIfUnchanged`; this plan adds no new write path.
- One commit per task, repro-first (failing test → fix → green).

---

## Background: the three confirmed defects

All three were reproduced end-to-end through the real forms before this plan was written.

1. **Spurious rule rewrite (the Pass-23 carried lead).** `NewRepeatChoices` is seeded from the item's raw anchor (`ev.Start`/`td.Due` — UTC for any `Z` value) while `Resolve` uses the form's local start. Opening the edit form and pressing Save **without touching the Repeat dropdown** rewrites the rule (`BYDAY=MO` → `BYDAY=TU`), shifting the series a day and routing the save through `RewriteEventRule`, which also **drops orphaned overrides**. Affects the event form and the todo form.
2. **Wrong-day series.** A New York user creating a Tue 20:00 weekly meeting and picking the dropdown labelled *"Weekly on Tue"* gets `DTSTART=20260826T000000Z` + `RRULE:FREQ=WEEKLY;BYDAY=TU`, which fires every **Monday** — and the created event's own start is not in its series.
3. **DST drift.** The same series moves from 20:00 EDT to 19:00 EST after the November transition, because a UTC-anchored recurring series has no zone to hold its wall-clock time.

Verified target state (read side, already working today):
`DTSTART;TZID=America/New_York:20260825T200000` + `RRULE:FREQ=WEEKLY;BYDAY=TU` → Tue 20:00 every week, 20:00 preserved across the DST boundary, first occurrence == the event's own start.

Because a TZID-anchored item decodes with `ev.Start.Location()` == the local zone, defect 1 also dissolves for newly-written items — Task 4 closes it for items already stored in UTC form.

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/config/zone.go` (create) | Resolve the host's IANA zone name → `*time.Location`. Environment I/O only. |
| `internal/config/zone_test.go` (create) | Resolver table tests. |
| `internal/model/vtimezone.go` (create) | Pure VTIMEZONE generation from a `*time.Location`. |
| `internal/model/vtimezone_test.go` (create) | Per-zone VTIMEZONE vectors + round-trip through our own decoder. |
| `internal/model/edit.go` (modify) | Anchor serialization: TZID form when authoring a rule. |
| `internal/model/tzanchor_test.go` (create) | Anchor-form matrix + iron-rule guards. |
| `internal/ui/app.go` (modify) | `Options.Location` → `a.loc`. |
| `cmd/lazyplanner/main.go` (modify) | Wire `config.LocalZone()` into `ui.Options`. |
| `internal/ui/itemforms.go` (modify) | Seed `RepeatChoices` on the same anchor basis `Resolve` uses. |
| `internal/ui/repeatanchor_test.go` (create) | End-to-end form guards for defects 1 and 2. |

---

### Task 1: Resolve the host's IANA zone name

Go exposes the IANA name through `time.Local.String()` **only when `$TZ` is set**; with `$TZ` unset it returns the literal `"Local"`. Probed on this machine: `TZ=America/New_York` → `"America/New_York"`; unset → `"Local"` with `/etc/localtime` symlinked to `/usr/share/zoneinfo/America/New_York`. Without a real IANA name there is no TZID to write, so the write path must be able to detect that and fall back to today's UTC behavior.

**Files:**
- Create: `internal/config/zone.go`
- Test: `internal/config/zone_test.go`

**Interfaces:**
- Produces: `func LocalZone() *time.Location` — a location whose `String()` is an IANA name when one can be determined, else `time.Local` unchanged. Never returns nil, never errors (a zone we cannot name is a degraded-but-working case, not a startup failure).

- [ ] **Step 1: Write the failing test**

```go
package config

import (
	"os"
	"testing"
	"time"
)

func TestLocalZonePrefersTZEnv(t *testing.T) {
	t.Setenv("TZ", "America/New_York")
	if got := LocalZone().String(); got != "America/New_York" {
		t.Errorf("LocalZone() = %q, want America/New_York", got)
	}
}

// With TZ unset the resolver falls back to the system zone files. On a host
// with neither, it must still return a usable location rather than nil.
func TestLocalZoneWithoutTZEnvIsUsable(t *testing.T) {
	t.Setenv("TZ", "")
	os.Unsetenv("TZ")
	loc := LocalZone()
	if loc == nil {
		t.Fatal("LocalZone() = nil")
	}
	// Whatever it resolves to must round-trip: a name we cannot load is worse
	// than no name at all, because the write path would emit an unresolvable TZID.
	if name := loc.String(); name != "Local" && name != "UTC" {
		if _, err := time.LoadLocation(name); err != nil {
			t.Errorf("LocalZone() = %q, which does not load: %v", name, err)
		}
	}
}

func TestLocalZoneIgnoresUnloadableTZ(t *testing.T) {
	t.Setenv("TZ", "Not/AZone")
	if loc := LocalZone(); loc == nil {
		t.Fatal("LocalZone() = nil for a bogus TZ")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLocalZone -v`
Expected: FAIL — `undefined: LocalZone`

- [ ] **Step 3: Write minimal implementation**

```go
package config

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// zoneInfoRoots are the directories a resolved /etc/localtime symlink is
// expected to point inside; the IANA name is the path relative to one of them.
var zoneInfoRoots = []string{"/usr/share/zoneinfo/", "/usr/lib/zoneinfo/", "/etc/zoneinfo/"}

// LocalZone returns the host's local time zone, preferring a location whose
// name is a real IANA identifier.
//
// Go names time.Local "Local" unless $TZ is set, and an anchor written as
// DTSTART;TZID=Local:... is meaningless to every other CalDAV client — so the
// name matters, not just the offset. Falling back to time.Local unnamed is safe:
// the write path keeps a UTC anchor when it cannot name the zone.
func LocalZone() *time.Location {
	if tz := os.Getenv("TZ"); tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}
	if name := zoneNameFromEtc(); name != "" {
		if loc, err := time.LoadLocation(name); err == nil {
			return loc
		}
	}
	return time.Local
}

// zoneNameFromEtc reads the IANA name Debian/Ubuntu record in /etc/timezone, or
// derives it from the /etc/localtime symlink target used by most other distros.
func zoneNameFromEtc() string {
	if b, err := os.ReadFile("/etc/timezone"); err == nil {
		if name := strings.TrimSpace(string(b)); name != "" {
			return name
		}
	}
	target, err := filepath.EvalSymlinks("/etc/localtime")
	if err != nil {
		return ""
	}
	target = filepath.ToSlash(target)
	for _, root := range zoneInfoRoots {
		if strings.HasPrefix(target, root) {
			return strings.TrimPrefix(target, root)
		}
	}
	return ""
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/config/ -v -run TestLocalZone`
Expected: PASS

- [ ] **Step 5: Full gate and commit**

```bash
go test ./... && go vet ./... && staticcheck ./... && go build ./...
git add internal/config/zone.go internal/config/zone_test.go
git commit -m "feat: resolve the host's IANA zone name for TZID-anchored recurrence"
```

---

### Task 2: Generate a conformant VTIMEZONE

RFC 5545 §3.6.5 requires any referenced TZID to be defined by a VTIMEZONE in the same object. LazyPlanner's own reader resolves zones from the embedded IANA database and does not need it (`dropUnusableTimezones`' doc comment records this), but the phone and NextCloud web that read the same resource do — and "a well-behaved CalDAV citizen" is a stated design goal.

Emit the conventional compact shape other clients emit: one STANDARD and (when the zone observes DST) one DAYLIGHT observance, each with a yearly `RRULE` derived from the most recent transitions. The generated component must satisfy `timezoneUsable` so it survives our own ingest heal.

**Files:**
- Create: `internal/model/vtimezone.go`
- Test: `internal/model/vtimezone_test.go`

**Interfaces:**
- Consumes: nothing from Task 1 (takes a `*time.Location` directly).
- Produces:
  - `func BuildVTimezone(loc *time.Location, around time.Time) *ical.Component` — returns nil when `loc` has no IANA-usable name (`""`, `"Local"`, or a fixed-offset zone), so callers can skip emitting a TZID entirely.
  - `func IsNamedZone(loc *time.Location) bool` — whether `loc` can be referenced as a TZID. Task 3 uses this as its gate.

- [ ] **Step 1: Write the failing test**

```go
package model

import (
	"strings"
	"testing"
	"time"
)

func TestIsNamedZone(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	for _, tc := range []struct {
		loc  *time.Location
		want bool
	}{
		{ny, true},
		{time.UTC, false},                          // UTC needs no TZID; Z is the correct form
		{time.FixedZone("EDT", -4*3600), false},    // an offset is not a zone identity
		{nil, false},
	} {
		if got := IsNamedZone(tc.loc); got != tc.want {
			t.Errorf("IsNamedZone(%v) = %v, want %v", tc.loc, got, tc.want)
		}
	}
}

// A DST zone gets both observances, each carrying the props timezoneUsable (and
// therefore go-ical's encoder) requires.
func TestBuildVTimezoneDSTZone(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	comp := BuildVTimezone(ny, time.Date(2026, 8, 25, 0, 0, 0, 0, ny))
	if comp == nil {
		t.Fatal("BuildVTimezone returned nil for a named DST zone")
	}
	if got := comp.Props.Get("TZID").Value; got != "America/New_York" {
		t.Errorf("TZID = %q", got)
	}
	var std, day *ical.Component
	for _, sub := range comp.Children {
		switch sub.Name {
		case ical.CompTimezoneStandard:
			std = sub
		case ical.CompTimezoneDaylight:
			day = sub
		}
	}
	if std == nil || day == nil {
		t.Fatalf("want both STANDARD and DAYLIGHT, got %d children", len(comp.Children))
	}
	for name, sub := range map[string]*ical.Component{"STANDARD": std, "DAYLIGHT": day} {
		for _, p := range []string{"DTSTART", "TZOFFSETFROM", "TZOFFSETTO"} {
			if sub.Props.Get(p) == nil {
				t.Errorf("%s missing %s", name, p)
			}
		}
	}
	if got := day.Props.Get("TZOFFSETTO").Value; got != "-0400" {
		t.Errorf("DAYLIGHT TZOFFSETTO = %q, want -0400", got)
	}
	if got := std.Props.Get("TZOFFSETTO").Value; got != "-0500" {
		t.Errorf("STANDARD TZOFFSETTO = %q, want -0500", got)
	}
	if r := day.Props.Get("RRULE"); r == nil || !strings.Contains(r.Value, "BYMONTH=3") {
		t.Errorf("DAYLIGHT RRULE = %v, want a March yearly rule", r)
	}
}

// A zone with no DST gets a single STANDARD observance and no RRULE.
func TestBuildVTimezoneFixedZone(t *testing.T) {
	kol, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Skip(err)
	}
	comp := BuildVTimezone(kol, time.Date(2026, 8, 25, 0, 0, 0, 0, kol))
	if comp == nil {
		t.Fatal("nil for Asia/Kolkata")
	}
	if len(comp.Children) != 1 || comp.Children[0].Name != ical.CompTimezoneStandard {
		t.Fatalf("want one STANDARD, got %v", comp.Children)
	}
	if got := comp.Children[0].Props.Get("TZOFFSETTO").Value; got != "+0530" {
		t.Errorf("TZOFFSETTO = %q, want +0530", got)
	}
}

// Southern hemisphere: DST starts in the second half of the year.
func TestBuildVTimezoneSouthernHemisphere(t *testing.T) {
	syd, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		t.Skip(err)
	}
	comp := BuildVTimezone(syd, time.Date(2026, 8, 25, 0, 0, 0, 0, syd))
	if comp == nil {
		t.Fatal("nil for Australia/Sydney")
	}
	if len(comp.Children) != 2 {
		t.Fatalf("want two observances, got %d", len(comp.Children))
	}
}

// The generated component must survive our own ingest heal, which drops any
// VTIMEZONE go-ical's encoder would reject.
func TestBuildVTimezoneIsUsable(t *testing.T) {
	for _, name := range []string{"America/New_York", "Europe/Berlin", "Asia/Kolkata", "Australia/Sydney"} {
		loc, err := time.LoadLocation(name)
		if err != nil {
			t.Skip(err)
		}
		comp := BuildVTimezone(loc, time.Date(2026, 8, 25, 0, 0, 0, 0, loc))
		if comp == nil || !timezoneUsable(comp) {
			t.Errorf("%s: generated VTIMEZONE is not usable", name)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run 'TestIsNamedZone|TestBuildVTimezone' -v`
Expected: FAIL — `undefined: IsNamedZone`, `undefined: BuildVTimezone`

- [ ] **Step 3: Write the implementation**

```go
package model

import (
	"fmt"
	"time"

	"github.com/emersion/go-ical"
)

// vtimezoneProbeYears is how far around the anchor transitions are searched. Two
// years is enough to see a full DST cycle on either side of any anchor while
// keeping the day-stepping scan trivial.
const vtimezoneProbeYears = 2

// IsNamedZone reports whether loc can be referenced by TZID. UTC is excluded
// deliberately: a UTC value's correct serialization is the Z form, which needs no
// TZID and no VTIMEZONE. A fixed-offset zone (time.FixedZone) carries an
// abbreviation, not an identity, so it is not referenceable either.
func IsNamedZone(loc *time.Location) bool {
	if loc == nil || loc == time.UTC {
		return false
	}
	name := loc.String()
	if name == "" || name == "Local" || name == "UTC" {
		return false
	}
	// A name that does not load is not one another client can resolve.
	_, err := time.LoadLocation(name)
	return err == nil
}

// BuildVTimezone returns a VTIMEZONE describing loc's current offset rules, or
// nil when loc is not TZID-referenceable.
//
// The shape is the conventional compact one — one observance per offset with a
// yearly RRULE derived from the most recent transition — rather than an exhaustive
// historical record: it is what NextCloud, Google and Apple emit, and what other
// clients are tested against.
func BuildVTimezone(loc *time.Location, around time.Time) *ical.Component {
	if !IsNamedZone(loc) {
		return nil
	}
	tz := ical.NewComponent(ical.CompTimezone)
	tz.Props.SetText(ical.PropTimezoneID, loc.String())

	from := around.AddDate(-vtimezoneProbeYears, 0, 0)
	to := around.AddDate(vtimezoneProbeYears, 0, 0)
	transitions := zoneTransitions(loc, from, to)

	if len(transitions) == 0 {
		// No DST in the window: one STANDARD observance carrying the fixed offset.
		_, off := around.In(loc).Zone()
		tz.Children = append(tz.Children, observance(ical.CompTimezoneStandard, around.In(loc), off, off, false))
		return tz
	}

	// Keep the most recent transition of each kind, so the component carries at
	// most one STANDARD and one DAYLIGHT.
	seen := map[string]bool{}
	for i := len(transitions) - 1; i >= 0; i-- {
		at := transitions[i].In(loc)
		name := ical.CompTimezoneStandard
		if at.IsDST() {
			name = ical.CompTimezoneDaylight
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		_, offTo := at.Zone()
		_, offFrom := at.Add(-time.Second).In(loc).Zone()
		tz.Children = append(tz.Children, observance(name, at, offFrom, offTo, true))
	}
	return tz
}

// zoneTransitions returns the instants in [from, to) at which loc changes offset,
// found by day-stepping and then bisecting to the minute. Go exposes no
// transition table, so probing is the only portable way to read one.
func zoneTransitions(loc *time.Location, from, to time.Time) []time.Time {
	var out []time.Time
	prev := from
	_, prevOff := prev.In(loc).Zone()
	for t := from.AddDate(0, 0, 1); t.Before(to); t = t.AddDate(0, 0, 1) {
		_, off := t.In(loc).Zone()
		if off == prevOff {
			prev = t
			continue
		}
		lo, hi := prev, t
		for hi.Sub(lo) > time.Minute {
			mid := lo.Add(hi.Sub(lo) / 2)
			if _, o := mid.In(loc).Zone(); o == prevOff {
				lo = mid
			} else {
				hi = mid
			}
		}
		out = append(out, hi.Truncate(time.Minute))
		prevOff, prev = off, t
	}
	return out
}

// observance builds one STANDARD/DAYLIGHT subcomponent. DTSTART is the local wall
// clock of the transition expressed in the offset being switched *to*, per RFC 5545.
func observance(name string, at time.Time, offFrom, offTo int, recurring bool) *ical.Component {
	sub := ical.NewComponent(name)
	sub.Props.SetText(ical.PropDateTimeStart, at.Format("20060102T150405"))
	sub.Props.SetText("TZOFFSETFROM", icalUTCOffset(offFrom))
	sub.Props.SetText("TZOFFSETTO", icalUTCOffset(offTo))
	if abbrev, _ := at.Zone(); abbrev != "" {
		sub.Props.SetText("TZNAME", abbrev)
	}
	if recurring {
		sub.Props.SetText(ical.PropRecurrenceRule,
			fmt.Sprintf("FREQ=YEARLY;BYMONTH=%d;BYDAY=%s", int(at.Month()), icalNthWeekday(at)))
	}
	return sub
}

// icalUTCOffset renders seconds east of UTC as the ±HHMM form UTC-OFFSET requires.
func icalUTCOffset(seconds int) string {
	sign := "+"
	if seconds < 0 {
		sign, seconds = "-", -seconds
	}
	return fmt.Sprintf("%s%02d%02d", sign, seconds/3600, (seconds%3600)/60)
}

// icalNthWeekday renders t's weekday as the BYDAY ordinal form ("2SU"), using -1
// for a date in the final week of its month ("last Sunday") the way zone rules
// are conventionally expressed.
func icalNthWeekday(t time.Time) string {
	abbrev := [...]string{"SU", "MO", "TU", "WE", "TH", "FR", "SA"}[t.Weekday()]
	if t.AddDate(0, 0, 7).Month() != t.Month() {
		return "-1" + abbrev
	}
	return fmt.Sprintf("%d%s", (t.Day()-1)/7+1, abbrev)
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/model/ -run 'TestIsNamedZone|TestBuildVTimezone' -v`
Expected: PASS

- [ ] **Step 5: Full gate and commit**

```bash
go test ./... && go vet ./... && staticcheck ./... && go build ./...
git add internal/model/vtimezone.go internal/model/vtimezone_test.go
git commit -m "feat: generate a conformant VTIMEZONE for a named IANA zone"
```

---

### Task 3: Write the recurrence anchor in TZID form when authoring a rule

The gate is deliberately narrow: **the TZID form is written only when this call is also authoring the rule** (`d.Recur != nil`, which is exactly the create-with-a-rule and rewrite-the-rule cases). An edit that leaves the rule alone keeps the anchor's existing serialization byte-for-byte.

That narrowness is the whole safety argument. Re-anchoring an existing series without touching its `BY*` **changes its occurrence set**: a correct legacy event (`DTSTART:20260824T230000Z` Monday-UTC + `BYDAY=MO`, firing Tuesdays in Berlin) would start firing Mondays if its anchor were moved to `TZID=Europe/Berlin` with `BYDAY=MO` intact. When `d.Recur != nil` the `BY*` parts are being re-derived from the same local anchor in the same call, so the two agree by construction.

**Files:**
- Modify: `internal/model/edit.go` — `newDateOrTimeProp`, `setDateOrTime`, `applyEvent`, `applyTodo`, `NewEventObject`, `NewTodoObject`, `editComponent`
- Modify: `internal/model/recur_edit.go:309,332,747,755` — the four anchor writers must preserve an anchor's zone form
- Create: `internal/model/tzanchor_test.go`

**Three facts established by a pre-flight read of the vendored library and the call sites — do not re-derive them:**

1. **go-ical already emits the TZID form.** `vendor/github.com/emersion/go-ical/ical.go:167-176` — `SetDateTime` sets the `TZID` param and formats local wall clock whenever `t.Location()` is neither nil nor `time.UTC`. Today's `newDateOrTimeProp` gets the UTC form *only* because it passes `t.UTC()`. So the switch is `prop.SetDateTime(t)` vs `prop.SetDateTime(t.UTC())` — nothing more.
2. **Never use `prop.SetText` for a date-time.** It calls `SetValueType(ValueText)`, which stamps `VALUE=TEXT` on the property and corrupts it.
3. **`setDateOrTime`/`newDateOrTimeProp` have eight call sites, not four** — `edit.go:279,295,301`, `recur_edit.go:309,332,747,755`, and `recurfield_test.go:14`. Keep the existing 4-argument signature so the unrelated sites stay untouched; add the zone decision as a separate helper the anchor writers call.

**Interfaces:**
- Consumes: `IsNamedZone`, `BuildVTimezone` (Task 2).
- Produces: no new exported API. `applyEvent`/`applyTodo` keep their signatures; the anchor form is derived from `d.Start`/`d.Due`'s own `Location()`, which the UI already builds with `a.loc`.

- [ ] **Step 1: Write the failing test**

```go
package model

import (
	"strings"
	"testing"
	"time"
)

func mustZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("zone %q unavailable: %v", name, err)
	}
	return loc
}

func propOf(t *testing.T, obj *Parsed, name string) *ical.Prop {
	t.Helper()
	for _, c := range obj.Calendar.Children {
		if c.Name == ical.CompEvent || c.Name == ical.CompToDo {
			return c.Props.Get(name)
		}
	}
	t.Fatalf("no item component carrying %s", name)
	return nil
}

// Creating a recurring event anchors it in the user's zone, so the rule the app
// derived from the local weekday is evaluated against that same weekday.
func TestNewRecurringEventAnchorsWithTZID(t *testing.T) {
	ny := mustZone(t, "America/New_York")
	start := time.Date(2026, 8, 25, 20, 0, 0, 0, ny) // Tuesday 8pm
	obj, err := NewEventObject(EventDraft{
		Summary: "Evening sync",
		Start:   start,
		End:     start.Add(time.Hour),
		Recur:   &RecurSpec{Freq: FreqWeekly, Weekdays: []time.Weekday{time.Tuesday}},
	}, start)
	if err != nil {
		t.Fatal(err)
	}
	dtstart := propOf(t, obj, ical.PropDateTimeStart)
	if got := dtstart.Params.Get(ical.ParamTimezoneID); got != "America/New_York" {
		t.Errorf("DTSTART TZID = %q, want America/New_York", got)
	}
	if got := dtstart.Value; got != "20260825T200000" {
		t.Errorf("DTSTART = %q, want the local wall clock 20260825T200000", got)
	}

	// The whole point: the series must fall on Tuesdays and hold 20:00 across DST.
	raw, err := obj.Encode()
	if err != nil {
		t.Fatal(err)
	}
	reparsed, err := Decode(raw, ny)
	if err != nil {
		t.Fatal(err)
	}
	ev := reparsed.Events[0]
	occs, err := ev.Occurrences(time.Date(2026, 8, 20, 0, 0, 0, 0, ny), time.Date(2026, 11, 20, 0, 0, 0, 0, ny))
	if err != nil {
		t.Fatal(err)
	}
	if len(occs) == 0 {
		t.Fatal("no occurrences")
	}
	if !occs[0].Start.Equal(start) {
		t.Errorf("first occurrence = %v, want the event's own start %v", occs[0].Start, start)
	}
	for _, o := range occs {
		local := o.Start.In(ny)
		if local.Weekday() != time.Tuesday {
			t.Errorf("occurrence %v is a %v, want Tuesday", local, local.Weekday())
		}
		if local.Hour() != 20 {
			t.Errorf("occurrence %v drifted off 20:00", local)
		}
	}
}

// A VTIMEZONE for the referenced zone travels with the object.
func TestNewRecurringEventCarriesVTimezone(t *testing.T) {
	ny := mustZone(t, "America/New_York")
	start := time.Date(2026, 8, 25, 20, 0, 0, 0, ny)
	obj, err := NewEventObject(EventDraft{
		Summary: "Evening sync", Start: start, End: start.Add(time.Hour),
		Recur: &RecurSpec{Freq: FreqWeekly, Weekdays: []time.Weekday{time.Tuesday}},
	}, start)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, c := range obj.Calendar.Children {
		if c.Name == ical.CompTimezone && c.Props.Get(ical.PropTimezoneID).Value == "America/New_York" {
			found = true
		}
	}
	if !found {
		t.Error("no VTIMEZONE for the referenced TZID")
	}
}

// A NON-recurring event keeps the UTC form: no TZID, no VTIMEZONE, no churn.
func TestNonRecurringEventStaysUTC(t *testing.T) {
	ny := mustZone(t, "America/New_York")
	start := time.Date(2026, 8, 25, 20, 0, 0, 0, ny)
	obj, err := NewEventObject(EventDraft{Summary: "One-off", Start: start, End: start.Add(time.Hour)}, start)
	if err != nil {
		t.Fatal(err)
	}
	dtstart := propOf(t, obj, ical.PropDateTimeStart)
	if tzid := dtstart.Params.Get(ical.ParamTimezoneID); tzid != "" {
		t.Errorf("non-recurring DTSTART carries TZID %q", tzid)
	}
	if !strings.HasSuffix(dtstart.Value, "Z") {
		t.Errorf("non-recurring DTSTART = %q, want the UTC Z form", dtstart.Value)
	}
}

// THE SAFETY PROPERTY: editing a recurring item WITHOUT authoring a rule must not
// re-anchor it — re-interpreting BY* against a new zone would move the series.
func TestEditWithoutRuleChangeKeepsAnchorForm(t *testing.T) {
	berlin := mustZone(t, "Europe/Berlin")
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\nBEGIN:VEVENT\r\nUID:keep-1\r\n" +
		"DTSTAMP:20260101T000000Z\r\nDTSTART:20260824T230000Z\r\nDTEND:20260825T000000Z\r\n" +
		"RRULE:FREQ=WEEKLY;BYDAY=MO\r\nSUMMARY:old\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	obj, err := Decode([]byte(ics), berlin)
	if err != nil {
		t.Fatal(err)
	}
	before, err := obj.Events[0].Occurrences(
		time.Date(2026, 8, 20, 0, 0, 0, 0, berlin), time.Date(2026, 9, 20, 0, 0, 0, 0, berlin))
	if err != nil {
		t.Fatal(err)
	}

	// A summary-only edit: Recur is nil, so the rule is untouched.
	edited, err := EditEvent(obj, "keep-1", EventDraft{
		Summary: "renamed",
		Start:   obj.Events[0].Start,
		End:     obj.Events[0].End,
	}, time.Now(), berlin)
	if err != nil {
		t.Fatal(err)
	}
	dtstart := propOf(t, edited, ical.PropDateTimeStart)
	if tzid := dtstart.Params.Get(ical.ParamTimezoneID); tzid != "" {
		t.Errorf("untouched rule was re-anchored to TZID %q", tzid)
	}
	after, err := edited.Events[0].Occurrences(
		time.Date(2026, 8, 20, 0, 0, 0, 0, berlin), time.Date(2026, 9, 20, 0, 0, 0, 0, berlin))
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("occurrence count changed: %d → %d", len(before), len(after))
	}
	for i := range before {
		if !before[i].Start.Equal(after[i].Start) {
			t.Errorf("occurrence %d moved: %v → %v", i, before[i].Start, after[i].Start)
		}
	}
}

// A recurring TODO's DUE is its anchor and gets the same treatment.
func TestNewRecurringTodoAnchorsDueWithTZID(t *testing.T) {
	ny := mustZone(t, "America/New_York")
	due := time.Date(2026, 8, 25, 20, 0, 0, 0, ny)
	obj, err := NewTodoObject(TodoDraft{
		Summary: "Water plants", HasDue: true, Due: due,
		Recur: &RecurSpec{Freq: FreqWeekly, Weekdays: []time.Weekday{time.Tuesday}},
	}, due)
	if err != nil {
		t.Fatal(err)
	}
	if got := propOf(t, obj, ical.PropDue).Params.Get(ical.ParamTimezoneID); got != "America/New_York" {
		t.Errorf("DUE TZID = %q, want America/New_York", got)
	}
}
```

> Check `NewTodoObject`'s actual signature before writing the last test — mirror `NewEventObject`'s.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run 'TZID|AnchorForm|StaysUTC' -v`
Expected: FAIL — `DTSTART TZID = "", want America/New_York`

- [ ] **Step 3: Write the implementation**

In `internal/model/edit.go`, leave `newDateOrTimeProp`/`setDateOrTime` exactly as they are (they keep serving the six non-anchor call sites) and add the zone-aware pair beside them:

```go
// setAnchorDateOrTime writes a value that a recurrence rule may be anchored to.
//
// It differs from setDateOrTime in one respect: when zone is non-nil the value is
// written as local wall clock + TZID rather than the UTC Z form. That matters
// because RFC 5545 evaluates a rule's BY* parts in its anchor's own zone, so a
// UTC-anchored rule authored from a local weekday fires on the wrong day whenever
// the local and UTC dates differ — and drifts an hour across DST.
//
// go-ical's SetDateTime already emits the TZID form for any non-UTC location, so
// the zone choice is expressed entirely by which location t carries.
func setAnchorDateOrTime(comp *ical.Component, name string, t time.Time, allDay bool, zone *time.Location) {
	prop := ical.NewProp(name)
	switch {
	case allDay:
		prop.SetDate(t)
	case zone != nil:
		prop.SetDateTime(t.In(zone))
	default:
		prop.SetDateTime(t.UTC())
	}
	comp.Props.Set(prop)
}

// anchorZone returns the zone a recurrence anchor should be written in, or nil for
// the UTC form.
//
// Two cases produce a zoned anchor, and the order matters:
//
//   - The component's existing anchor already carries a resolvable TZID — keep
//     writing in THAT zone. It may be a server's own zone, and re-expressing its
//     data in ours would churn it (iron rule); flattening it to UTC, which this
//     code did before, silently broke the rule the server authored.
//   - This call is authoring the rule itself (recur != nil) and t is in a zone
//     another client can resolve — anchor in it, so the BY* parts being derived
//     from t agree with the anchor by construction.
//
// Anything else keeps the UTC form, including every non-recurring value and every
// edit that leaves an existing rule alone. That last exclusion is deliberate:
// re-anchoring a series without re-deriving its BY* would move it.
func anchorZone(comp *ical.Component, name string, recur *RecurSpec, allDay bool, t time.Time) *time.Location {
	if allDay {
		return nil
	}
	if existing := comp.Props.Get(name); existing != nil {
		if tzid := existing.Params.Get(ical.ParamTimezoneID); tzid != "" {
			if loc, err := time.LoadLocation(tzid); err == nil {
				return loc
			}
		}
	}
	if recur != nil && IsNamedZone(t.Location()) {
		return t.Location()
	}
	return nil
}
```

In `applyEvent`, decide once from DTSTART and use the same zone for DTEND, so the two keep matching value types:

```go
zone := anchorZone(comp, ical.PropDateTimeStart, d.Recur, d.AllDay, d.Start)
setAnchorDateOrTime(comp, ical.PropDateTimeStart, d.Start, d.AllDay, zone)
comp.Props.Del(ical.PropDuration)
if !d.End.IsZero() {
	setAnchorDateOrTime(comp, ical.PropDateTimeEnd, d.End, d.AllDay, zone)
} else {
	comp.Props.Del(ical.PropDateTimeEnd)
}
```

In `applyTodo`:

```go
if d.HasDue {
	zone := anchorZone(comp, ical.PropDue, d.Recur, d.DueAllDay, d.Due)
	setAnchorDateOrTime(comp, ical.PropDue, d.Due, d.DueAllDay, zone)
} else {
	comp.Props.Del(ical.PropDue)
}
```

**The four `recur_edit.go` writers must stop flattening a zoned anchor.** `setDateOrTime` forces UTC, so once anchors can carry a TZID, `advanceRecurringTodo` (`:755`) would strip it every time a recurring todo is completed, the grab/shift path (`:747`) on every nudge, and the override/EXDATE writers (`:309`, `:332`) would emit a `RECURRENCE-ID`/`EXDATE` whose value type disagrees with the `DTSTART` it must match. At each of the four sites, keep the existing form by passing the property's own zone:

```go
// Preserve the anchor's serialization form: a RECURRENCE-ID must match its
// DTSTART's value type and zone, and re-writing a zoned anchor as UTC would
// re-interpret the rule's BY* parts against a different zone.
setAnchorDateOrTime(comp, name, value, allDay, anchorZone(comp, name, nil, allDay, value))
```

For `:309` (`RECURRENCE-ID`) and `:332` (`EXDATE`), the zone to preserve is the **master's `DTSTART`** zone, not the override property's own (which does not exist yet) — read it from the master component with `anchorZone(master, ical.PropDateTimeStart, nil, allDay, occ)`.

Then make the VTIMEZONE travel with the object. `applyEvent`/`applyTodo` only see the component, so add the calendar-level step where the `*ical.Calendar` is in hand — `NewEventObject`, `NewTodoObject`, and `editComponent`:

```go
// ensureVTimezone adds a VTIMEZONE for every TZID the object's items reference
// and that the object does not already define. Additive only: an existing
// VTIMEZONE (ours or a foreign server's) is never replaced or removed — per the
// iron rule, and because a server's own definition is the authority for its data.
func ensureVTimezone(cal *ical.Calendar, around time.Time) {
	defined := map[string]bool{}
	for _, c := range cal.Children {
		if c.Name == ical.CompTimezone {
			if p := c.Props.Get(ical.PropTimezoneID); p != nil {
				defined[p.Value] = true
			}
		}
	}
	for _, c := range cal.Children {
		if c.Name != ical.CompEvent && c.Name != ical.CompToDo {
			continue
		}
		for _, name := range []string{ical.PropDateTimeStart, ical.PropDateTimeEnd, ical.PropDue} {
			p := c.Props.Get(name)
			if p == nil {
				continue
			}
			tzid := p.Params.Get(ical.ParamTimezoneID)
			if tzid == "" || defined[tzid] {
				continue
			}
			loc, err := time.LoadLocation(tzid)
			if err != nil {
				continue
			}
			if tz := BuildVTimezone(loc, around); tz != nil {
				// VTIMEZONE must precede the components referencing it.
				cal.Children = append([]*ical.Component{tz}, cal.Children...)
				defined[tzid] = true
			}
		}
	}
}
```

Call `ensureVTimezone(cal, now)` at the end of `NewEventObject` and `NewTodoObject` (before `Parse`), and in `editComponent` after the mutation runs.

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/model/ -v` then `go test ./...`
Expected: PASS. If existing tests assert a `Z`-suffixed DTSTART on a **recurring** item, they are asserting the defect — update them and note it in the commit body. Assertions on non-recurring items must keep passing untouched; if one breaks, the `anchorZoned` gate is wrong.

- [ ] **Step 5: Zone sweep**

```bash
for z in UTC America/New_York Europe/Berlin Asia/Kolkata Australia/Sydney Pacific/Kiritimati; do
  echo "== $z"; TZ=$z go test ./internal/model/ ./internal/store/ ./internal/ui/ || break
done
```
Expected: PASS in every zone.

- [ ] **Step 6: Full gate and commit**

```bash
go test ./... && go vet ./... && staticcheck ./... && go build ./...
git add internal/model/edit.go internal/model/tzanchor_test.go
git commit -m "fix: anchor a recurring item in the zone its rule was authored in"
```

---

### Task 4: Seed the Repeat dropdown on the same anchor basis it resolves with

Task 3 fixes items the app writes from now on. Items already stored with a UTC anchor still hit defect 1: `NewRepeatChoices` seeds from the raw UTC anchor while `Resolve` uses the local one, so an **untouched** dropdown reports a rewrite. Make both sides use the local anchor.

This also wires up the zone from Task 1, so `a.loc` carries an IANA name in production rather than the anonymous `time.Local`.

**Files:**
- Modify: `internal/ui/itemforms.go:125-138` (`newTodoRepeat`), `internal/ui/itemforms.go:332-337` (`newEventRepeat`)
- Modify: `internal/ui/app.go:385-397` (`Run`) — add `Options.Location`
- Modify: `cmd/lazyplanner/main.go` — pass `config.LocalZone()`
- Create: `internal/ui/repeatanchor_test.go`

**Interfaces:**
- Consumes: `config.LocalZone()` (Task 1).
- Produces: `Options.Location *time.Location` — nil keeps `newApp`'s `time.Local` default so existing tests are unaffected.

- [ ] **Step 1: Write the failing test**

```go
package ui

import (
	"context"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// Opening the edit form on a recurring event and pressing Save WITHOUT touching
// the Repeat dropdown must not rewrite the rule. The seed anchor (the raw UTC
// start) and the resolve anchor (the form's local start) disagreed on the
// weekday, so "unchanged" compared false and the series silently moved a day.
func TestUntouchedRepeatDropdownDoesNotRewriteRule(t *testing.T) {
	for _, zone := range []string{"UTC", "America/New_York", "Europe/Berlin", "Asia/Kolkata", "Pacific/Kiritimati"} {
		t.Run(zone, func(t *testing.T) {
			loc, err := time.LoadLocation(zone)
			if err != nil {
				t.Skipf("zone %q unavailable: %v", zone, err)
			}
			a := newRootedTestApp(t, time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC))
			a.loc = loc

			uid := "anchor-1"
			ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\nBEGIN:VEVENT\r\nUID:" + uid +
				"\r\nSUMMARY:Standup\r\nDTSTAMP:20260701T000000Z\r\n" +
				"DTSTART:20260824T230000Z\r\nDTEND:20260825T000000Z\r\n" +
				"RRULE:FREQ=WEEKLY;BYDAY=MO\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
			parsed, err := model.Decode([]byte(ics), time.Local)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := a.store.Put(context.Background(), "ev", store.ResourceName(uid), parsed); err != nil {
				t.Fatal(err)
			}
			located, ok := a.store.Locate(uid)
			if !ok {
				t.Fatal("seeded event not found")
			}
			before := rruleValue(t, located.Object, uid)

			a.showEventForm(located, uid)
			name, front := a.root.GetFrontPage()
			if name != pageForm {
				t.Fatalf("front page = %q, want %q", name, pageForm)
			}
			f := findCaretFormIn(front)
			if f == nil {
				t.Fatal("event form not found")
			}
			pressButton(t, f, "Save")

			after, ok := a.store.Locate(uid)
			if !ok {
				t.Fatal("event gone after save")
			}
			if got := rruleValue(t, after.Object, uid); got != before {
				t.Errorf("untouched Repeat dropdown rewrote the rule: %q → %q", before, got)
			}
		})
	}
}

// The todo form shares the seed/resolve anchor and must share the guard.
func TestUntouchedRepeatDropdownDoesNotRewriteTodoRule(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip(err)
	}
	a := newRootedTestApp(t, time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC))
	a.loc = berlin

	uid := "anchor-todo"
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\nBEGIN:VTODO\r\nUID:" + uid +
		"\r\nSUMMARY:Water plants\r\nDTSTAMP:20260701T000000Z\r\n" +
		"DUE:20260824T230000Z\r\nRRULE:FREQ=WEEKLY;BYDAY=MO\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"
	parsed, err := model.Decode([]byte(ics), time.Local)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.Put(context.Background(), "tasks", store.ResourceName(uid), parsed); err != nil {
		t.Fatal(err)
	}
	located, _ := a.store.Locate(uid)
	before := rruleValue(t, located.Object, uid)

	a.showTodoForm(located, uid)
	_, front := a.root.GetFrontPage()
	pressButton(t, findCaretFormIn(front), "Save")

	after, _ := a.store.Locate(uid)
	if got := rruleValue(t, after.Object, uid); got != before {
		t.Errorf("untouched Repeat dropdown rewrote the todo rule: %q → %q", before, got)
	}
}

// rruleValue returns the RRULE of the component carrying uid, or "" when absent.
func rruleValue(t *testing.T, obj *model.Parsed, uid string) string {
	t.Helper()
	for _, c := range obj.Calendar.Children {
		if p := c.Props.Get("UID"); p != nil && p.Value == uid {
			if r := c.Props.Get("RRULE"); r != nil {
				return r.Value
			}
			return ""
		}
	}
	t.Fatalf("component %q not found", uid)
	return ""
}
```

Add a second guard for defect 2, driving the create form end-to-end:

```go
// A weekly event created at a local time whose UTC date differs must recur on the
// day the user picked — the dropdown said "Weekly on Tue", so Tuesday it is.
func TestCreatedWeeklyEventRecursOnThePickedDay(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	a := newRootedTestApp(t, time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC))
	a.loc = ny

	base := time.Date(2026, 8, 25, 0, 0, 0, 0, ny) // Tuesday
	a.showCreateEventForm("ev", base)
	_, front := a.root.GetFrontPage()
	f := findCaretFormIn(front)
	formItemByLabel(t, f, "Summary").(*tview.InputField).SetText("Evening sync")
	formItemByLabel(t, f, "All day").(*tview.Checkbox).SetChecked(false)
	formItemByLabel(t, f, "Start date").(*tview.InputField).SetText("2026-08-25")
	formItemByLabel(t, f, "Start time").(*tview.InputField).SetText("20:00")
	formItemByLabel(t, f, "End time").(*tview.InputField).SetText("21:00")
	formItemByLabel(t, f, "Repeat").(*tview.DropDown).SetCurrentOption(2) // Weekly on Tue
	pressButton(t, f, "Create")

	cs, ok := a.store.Calendar("ev")
	if !ok {
		t.Fatal("calendar missing")
	}
	for _, r := range cs.Resources {
		for _, ev := range r.Object.Events {
			if ev.Summary != "Evening sync" {
				continue
			}
			occs, err := ev.Occurrences(
				time.Date(2026, 8, 20, 0, 0, 0, 0, ny), time.Date(2026, 11, 20, 0, 0, 0, 0, ny))
			if err != nil {
				t.Fatal(err)
			}
			if len(occs) == 0 {
				t.Fatal("no occurrences")
			}
			for _, o := range occs {
				local := o.Start.In(ny)
				if local.Weekday() != time.Tuesday {
					t.Errorf("occurrence %v is a %v, want Tuesday", local, local.Weekday())
				}
				if local.Hour() != 20 {
					t.Errorf("occurrence %v drifted off 20:00 (DST)", local)
				}
			}
			return
		}
	}
	t.Fatal("created event not found")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run 'UntouchedRepeat|RecursOnThePickedDay' -v`
Expected: FAIL — the untouched-dropdown tests report `"FREQ=WEEKLY;BYDAY=MO" → "FREQ=WEEKLY;BYDAY=TU"` in the non-UTC zones. `TestCreatedWeeklyEventRecursOnThePickedDay` should already PASS once Task 3 has landed; if it fails, Task 3's gate is not firing on the create path.

- [ ] **Step 3: Write the implementation**

`internal/ui/itemforms.go` — seed on the same basis `Resolve` uses. In `newEventRepeat`:

```go
// newEventRepeat builds the Repeat dropdown state for an event (nil ev = a create
// form), anchored at the given seed start/occurrence.
//
// The anchor is converted to the app's zone because readEventDraft resolves the
// dropdown against the form's start, which parseDateField builds in a.loc. Seeding
// from the raw anchor instead let the two disagree on the weekday whenever the
// item's local and UTC dates differ, so an UNTOUCHED dropdown compared unequal to
// its own seed and reported a rewrite — silently moving the series a day.
func (a *app) newEventRepeat(ev *model.Event, anchor time.Time) *model.RepeatChoices {
	anchor = anchor.In(a.loc)
	if ev == nil {
		return model.NewRepeatChoices(nil, anchor, a.loc)
	}
	return model.NewRepeatChoices(ev.Raw, anchor, a.loc)
}
```

Same one-line conversion in `newTodoRepeat` (`anchor = anchor.In(a.loc)` after the `td.Due` branch, covering the `a.now` default too).

`internal/ui/app.go` — accept the resolved zone:

```go
// Location is the zone times are displayed in and recurrence anchors are written
// in. Nil keeps time.Local.
Location *time.Location
```

and in `Run`, right after `newApp`:

```go
if opts.Location != nil {
	a.loc = opts.Location
}
```

`cmd/lazyplanner/main.go` — set `Location: config.LocalZone()` on the `ui.Options` literal.

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/ui/ -run 'UntouchedRepeat|RecursOnThePickedDay' -v`, then `go test ./...`
Expected: PASS

- [ ] **Step 5: Verify the guard bites**

Revert the `anchor = anchor.In(a.loc)` line in `newEventRepeat`, re-run `TestUntouchedRepeatDropdownDoesNotRewriteRule`, confirm RED in the non-UTC subtests, then restore. A guard that cannot fail is not a guard.

- [ ] **Step 6: Full gate and commit**

```bash
go test ./... && go vet ./... && staticcheck ./... && go build ./...
git add internal/ui/itemforms.go internal/ui/app.go cmd/lazyplanner/main.go internal/ui/repeatanchor_test.go
git commit -m "fix: an untouched Repeat dropdown no longer rewrites the rule"
```

---

### Task 5: Docs ripple and coverage ledger

**Files:**
- Modify: `main.md` — the "Colors and window chrome"-adjacent storage sentence in **Calendar views** (*"All timed values are displayed in the local timezone; ones LazyPlanner writes are stored in UTC…"*) rewritten **in place**, plus a `v1.5.x` entry
- Modify: `log.md` — one dated entry at the top
- Modify: `docs/audit/COVERAGE.md` — resolve carried-residual item 1
- Modify: `README.md` — only if user-visible behavior needs describing (see step 3)

- [ ] **Step 1: Rewrite the main.md storage decision in place**

Replace the UTC-storage sentence with the settled rule, keeping it short:

> All timed values are **displayed in the local timezone**. A **recurring** item's anchor (`DTSTART`, or `DUE` for a VTODO) is written as **local time with a `TZID`**, with a matching `VTIMEZONE` in the object — a recurrence rule's `BY*` parts are evaluated in the anchor's own zone, so a UTC anchor makes a locally-authored rule fire on the wrong day and drift across DST. Non-recurring timed values are still written in **UTC**, and a value imported from the server is preserved as-is per the iron rule. All-day items stay date-only. The anchor is re-serialized only when LazyPlanner is authoring the rule in the same edit; an edit that leaves the rule alone never re-anchors, because re-interpreting `BY*` against a different zone would move the series.

- [ ] **Step 2: Record the known residual**

Add to the same section or the v1.5.x entry: items **already** stored with a UTC anchor keep their existing (possibly wrong-day) behavior until their rule is next edited — a blanket migration would have to re-derive every `BY*` and was rejected as riskier than the defect. Also record the Windows limitation: `LocalZone` resolves via `$TZ` and the `/etc` zone files, so a Windows host without `$TZ` keeps UTC anchors.

- [ ] **Step 3: README**

The keybindings and usage text do not change. Add nothing unless the Syncing section claims UTC storage — check with `grep -n "UTC" README.md` and correct it in place if so.

- [ ] **Step 4: log.md entry**

Newest at the top, directly under the intro blockquote, own `## 2026-07-26 — …` heading, previous entry untouched. Cover: the three defects with the reproduced evidence, the owner's direction decision, the narrow re-anchor gate and why, the VTIMEZONE shape, and the two named residuals. Then verify: `grep -c '^## ' log.md` equals the entry count.

- [ ] **Step 5: COVERAGE.md**

Mark carried-residual item 1 (the unverified product-bug lead) **RESOLVED**, naming what it actually was and the commits that closed it, and note that it was found by recovering the killed agent's transcript rather than by re-auditing.

- [ ] **Step 6: Commit**

```bash
git add main.md log.md README.md docs/audit/COVERAGE.md
git commit -m "docs: record TZID-anchored recurrence and close the Pass-23 lead"
```

---

## Self-Review

**Spec coverage.** Defect 1 → Tasks 3 (new items, by construction) + 4 (existing UTC-anchored items). Defect 2 → Task 3, guarded end-to-end in Task 4. Defect 3 (DST drift) → Task 3, asserted by the `local.Hour() != 20` checks. Owner's chosen direction (TZID + VTIMEZONE) → Tasks 2 and 3. Zone-name prerequisite → Task 1, wired in Task 4. Docs → Task 5.

**Placeholders.** None: every step carries runnable code or an exact command. Two flagged transcription checks (`NewTodoObject`'s signature; the stray non-ASCII word in the `anchorZoned` comment) are called out at their step rather than left implicit.

**Type consistency.** `IsNamedZone`/`BuildVTimezone` are defined in Task 2 and consumed in Task 3 with matching signatures. `newDateOrTimeProp`/`setDateOrTime` gain the same `zoned bool` parameter in both their definition and all four call sites. `Options.Location` is defined and consumed in Task 4. `icalDateTimeLocal` is reused from `internal/model/tz.go`, not redeclared.

**Known risk.** Task 3 touches the single serialization choke point for every timed value in the app; the `anchorZoned` gate is what keeps non-recurring and rule-untouched writes byte-identical. `TestNonRecurringEventStaysUTC` and `TestEditWithoutRuleChangeKeepsAnchorForm` are the two tests that must never be weakened to make something else pass.
