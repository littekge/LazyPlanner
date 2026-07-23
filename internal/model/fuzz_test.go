package model_test

import (
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-ical"

	"github.com/littekge/LazyPlanner/internal/model"
)

// The fuzz targets below exercise LazyPlanner's input trust boundary: the
// iCalendar parser (fed arbitrary bytes written by any other CalDAV client or
// returned by a server) and the quick-add smart parser (fed arbitrary user
// text). The invariant everywhere is the error-handling standard from CLAUDE.md
// and main.md's iron rule — the model must never panic and must degrade
// gracefully rather than lose or corrupt data.
//
// `go test` runs the seed corpus (every f.Add case plus any saved crashers in
// testdata/fuzz) as ordinary deterministic tests, so these guard against
// regressions on the normal gate; `go test -fuzz=Fuzz...` explores new inputs.

// fuzzLoc is fixed (not time.Local) so a corpus entry reproduces identically
// regardless of the machine's timezone.
var fuzzLoc = time.UTC

// icalSeeds are representative iCalendar bodies fed to the decode/expansion
// fuzzers: valid fixtures plus the malformed shapes that were live bugs during
// hardening pass 3 (a malformed RRULE that blanked the calendar; UID-less and
// cyclically-linked todos), so the fuzzer starts from known-interesting inputs.
var icalSeeds = []string{
	"BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\nBEGIN:VEVENT\r\nUID:a\r\nDTSTART:20260101T100000Z\r\nDTEND:20260101T110000Z\r\nSUMMARY:Timed\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
	"BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\nBEGIN:VEVENT\r\nUID:b\r\nDTSTART;VALUE=DATE:20260101\r\nSUMMARY:All day\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
	"BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\nBEGIN:VEVENT\r\nUID:c\r\nDTSTART:20260101T100000Z\r\nRRULE:FREQ=WEEKLY;COUNT=3\r\nSUMMARY:Weekly\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
	// Semantically-broken RRULE: the pass-3 high-severity crasher. Must decode
	// and expand to the base instance, never error out.
	"BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\nBEGIN:VEVENT\r\nUID:d\r\nDTSTART:20260101T100000Z\r\nRRULE:FREQ=NONSENSE\r\nSUMMARY:Bad rule\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
	// Windows/Outlook zone name on DTSTART — recovered via the CLDR table.
	"BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\nBEGIN:VEVENT\r\nUID:e\r\nDTSTART;TZID=Eastern Standard Time:20260101T100000\r\nSUMMARY:Winzone\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
	"BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\nBEGIN:VTODO\r\nUID:t1\r\nSUMMARY:Task\r\nDUE:20260101T090000Z\r\nPRIORITY:1\r\nSTATUS:NEEDS-ACTION\r\nCATEGORIES:home,work\r\nEND:VTODO\r\nEND:VCALENDAR\r\n",
	// Two UID-less todos + a keyed one (pass-3 #7): all three must survive.
	"BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\nBEGIN:VTODO\r\nSUMMARY:NoUID one\r\nEND:VTODO\r\nBEGIN:VTODO\r\nSUMMARY:NoUID two\r\nEND:VTODO\r\nBEGIN:VTODO\r\nUID:k\r\nSUMMARY:Keyed\r\nEND:VTODO\r\nEND:VCALENDAR\r\n",
	// Cyclic RELATED-TO between two todos: BuildTree must not loop forever.
	"BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\nBEGIN:VTODO\r\nUID:p\r\nSUMMARY:P\r\nRELATED-TO:q\r\nEND:VTODO\r\nBEGIN:VTODO\r\nUID:q\r\nSUMMARY:Q\r\nRELATED-TO:p\r\nEND:VTODO\r\nEND:VCALENDAR\r\n",
	// A recurring event with a RECURRENCE-ID override sharing the UID.
	"BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\nBEGIN:VEVENT\r\nUID:o\r\nDTSTART:20260101T100000Z\r\nRRULE:FREQ=DAILY;COUNT=5\r\nSUMMARY:Series\r\nEND:VEVENT\r\nBEGIN:VEVENT\r\nUID:o\r\nRECURRENCE-ID:20260103T100000Z\r\nDTSTART:20260103T140000Z\r\nSUMMARY:Moved\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
	"",
	"BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n",
}

// FuzzDecode checks that decoding arbitrary bytes never panics, and that
// anything that decodes cleanly round-trips: Encode must succeed and re-decoding
// the encoded form must succeed with the same event/todo counts. A body we can
// parse but cannot re-encode-and-reparse would be a silent data-loss path,
// violating the property-preservation iron rule.
func FuzzDecode(f *testing.F) {
	for _, s := range icalSeeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		p, err := model.Decode(data, fuzzLoc)
		if err != nil {
			return // rejecting bad input is fine; it must just not panic.
		}
		if p == nil || p.Calendar == nil {
			t.Fatalf("Decode returned nil Parsed/Calendar with no error")
		}
		if len(p.Events) == 0 && len(p.Todos) == 0 {
			// go-ical refuses to encode a calendar with no components, and
			// LazyPlanner never writes such a resource (deleting the last item
			// removes the file), so there is nothing to round-trip.
			return
		}
		if !allHaveUID(p) {
			// A UID-less component is RFC-invalid and cannot be re-encoded. Per
			// pass-3 #7 such a todo is displayed best-effort but deliberately not
			// given a fabricated identity (that would churn under sync), so it is
			// out of scope for the writability round-trip.
			return
		}

		encoded, err := p.Encode()
		if err != nil {
			t.Fatalf("Encode failed for a value that decoded cleanly: %v", err)
		}

		reparsed, err := model.Decode(encoded, fuzzLoc)
		if err != nil {
			t.Fatalf("re-decoding our own Encode output failed: %v", err)
		}
		if len(reparsed.Events) != len(p.Events) {
			t.Fatalf("event count changed across encode round-trip: %d -> %d",
				len(p.Events), len(reparsed.Events))
		}
		if len(reparsed.Todos) != len(p.Todos) {
			t.Fatalf("todo count changed across encode round-trip: %d -> %d",
				len(p.Todos), len(reparsed.Todos))
		}
	})
}

// allHaveUID reports whether every event and todo carries a non-empty UID.
func allHaveUID(p *model.Parsed) bool {
	for _, ev := range p.Events {
		if ev.UID == "" {
			return false
		}
	}
	for _, td := range p.Todos {
		if td.UID == "" {
			return false
		}
	}
	return true
}

// FuzzEventOccurrences targets the recurrence-expansion path that blanked the
// calendar in pass 3. For any body that decodes, neither per-event Occurrences
// nor Parsed.EventOccurrences may error or panic — a malformed rule must degrade
// to the base instance, never propagate a failure that empties the view.
func FuzzEventOccurrences(f *testing.F) {
	for _, s := range icalSeeds {
		f.Add([]byte(s))
	}

	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)

	f.Fuzz(func(t *testing.T, data []byte) {
		p, err := model.Decode(data, fuzzLoc)
		if err != nil {
			return
		}
		for _, ev := range p.Events {
			if _, err := ev.Occurrences(from, to); err != nil {
				t.Fatalf("Event.Occurrences errored instead of degrading: %v", err)
			}
		}
		if _, err := p.EventOccurrences(from, to); err != nil {
			t.Fatalf("Parsed.EventOccurrences errored instead of degrading: %v", err)
		}
	})
}

// FuzzBuildTree drives the subtask-forest builder from a topology derived
// directly from fuzz bytes, so parent/child links, cycles, self-references,
// duplicate UIDs, and UID-less nodes are all reachable. Invariants: the build
// terminates and never panics (even on cyclic RELATED-TO); no *TodoNode appears
// twice — the double-append that dropped/duplicated tasks in pass-3 #7; it never
// surfaces more nodes than exist; and every UID-less todo surfaces exactly once
// (the core of #7 — UID-less todos must not collapse onto a shared slot). Fewer
// nodes than inputs is allowed only via BuildTree's documented dropping of nodes
// reachable solely through a cycle.
func FuzzBuildTree(f *testing.F) {
	// Seeds: a chain, a cycle, duplicate UIDs, and UID-less rows, expressed as
	// (uid, parentUID) byte pairs over a tiny id space.
	f.Add([]byte{1, 0, 2, 1, 3, 2}, true)  // 1<-2<-3 chain
	f.Add([]byte{1, 2, 2, 1}, true)        // 1<->2 cycle
	f.Add([]byte{1, 0, 1, 0, 2, 1}, false) // duplicate UID 1
	f.Add([]byte{0, 0, 0, 5, 1, 0}, true)  // UID-less rows (id 0 == "")
	f.Add([]byte{1, 1, 2, 2, 3, 3}, true)  // self-references

	f.Fuzz(func(t *testing.T, data []byte, includeCompleted bool) {
		todos := todosFromBytes(data)

		distinctUID := map[string]bool{}
		uidLess := 0
		for _, td := range todos {
			if !includeCompleted && td.Completed() {
				continue
			}
			if td.UID == "" {
				uidLess++
			} else {
				distinctUID[td.UID] = true
			}
		}
		maxNodes := len(distinctUID) + uidLess

		roots := model.BuildTree(todos, includeCompleted)

		seen := map[*model.TodoNode]bool{}
		total, uidLessSeen := walkForest(t, roots, seen)
		if total > maxNodes {
			t.Fatalf("BuildTree surfaced %d nodes, more than the %d possible", total, maxNodes)
		}
		if uidLessSeen != uidLess {
			t.Fatalf("BuildTree surfaced %d UID-less nodes, want %d (they must never collapse)",
				uidLessSeen, uidLess)
		}
	})
}

// walkForest walks the forest, failing if any *TodoNode is reached twice (a
// double-append or an output cycle), and returns the total node count and how
// many of those carry an empty UID.
func walkForest(t *testing.T, nodes []*model.TodoNode, seen map[*model.TodoNode]bool) (total, uidLess int) {
	t.Helper()
	for _, n := range nodes {
		if seen[n] {
			t.Fatalf("node %p appears more than once in the forest", n)
		}
		seen[n] = true
		total++
		if n.Todo.UID == "" {
			uidLess++
		}
		ct, cu := walkForest(t, n.Children, seen)
		total += ct
		uidLess += cu
	}
	return total, uidLess
}

// todosFromBytes reads the input as (uid, parentUID) pairs over a small id
// space, so the fuzzer densely explores tree topologies. id 0 maps to the empty
// UID (a malformed, unkeyable todo); a high parent bit marks the todo completed.
func todosFromBytes(data []byte) []*model.Todo {
	const maxTodos = 64
	var todos []*model.Todo
	for i := 0; i+1 < len(data) && len(todos) < maxTodos; i += 2 {
		uid := idName(data[i])
		parent := idName(data[i+1])
		td := &model.Todo{UID: uid, ParentUID: parent, Summary: uid}
		if data[i+1]%2 == 0 {
			td.Status = model.StatusCompleted
		}
		todos = append(todos, td)
	}
	return todos
}

// idName maps a byte to a small alphabet of UID strings; 0 becomes "" so
// UID-less rows are reachable.
func idName(b byte) string {
	n := b % 8
	if n == 0 {
		return ""
	}
	return string(rune('a' + n))
}

// recurEditSeeds are recurrence bodies specifically for the mutation fuzzer: a
// near-zero anchor (the write-side panic of pass-9 H2), an alarmed recurring
// event (the dropped-VALARM of H3/H4), an all-day recurring event (the H6 UNTIL
// value-type bug), and a series with a future override (the H5 dropped-override).
var recurEditSeeds = []string{
	"BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\nBEGIN:VTODO\r\nUID:z\r\nDTSTAMP:20260101T000000Z\r\nDTSTART:00000101T000000Z\r\nDUE:00000102T000000Z\r\nRRULE:FREQ=WEEKLY\r\nSUMMARY:t\r\nEND:VTODO\r\nEND:VCALENDAR\r\n",
	"BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\nBEGIN:VEVENT\r\nUID:al\r\nDTSTAMP:20260101T000000Z\r\nDTSTART:20260106T090000Z\r\nDTEND:20260106T100000Z\r\nRRULE:FREQ=WEEKLY\r\nSUMMARY:e\r\nBEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT15M\r\nEND:VALARM\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
	"BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\nBEGIN:VEVENT\r\nUID:ad\r\nDTSTAMP:20260101T000000Z\r\nDTSTART;VALUE=DATE:20260106\r\nRRULE:FREQ=WEEKLY\r\nSUMMARY:allday\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
}

// FuzzRecurrenceMutations drives the write-side recurrence primitives — the class
// of bugs from hardening pass 9 that sits just outside the decode-only fuzzers.
// For any body that decodes, editing an occurrence, splitting a series, adding an
// exception, and advancing a recurring todo must all (a) never panic and (b)
// produce an object that re-encodes — so a degenerate rule can't crash the app
// and a mutation can't yield an unsaveable object.
func FuzzRecurrenceMutations(f *testing.F) {
	for _, s := range icalSeeds {
		f.Add([]byte(s))
	}
	for _, s := range recurEditSeeds {
		f.Add([]byte(s))
	}
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	noop := func(*ical.Component) {}

	mustEncode := func(t *testing.T, p *model.Parsed) {
		if p == nil {
			return
		}
		if len(p.Events) == 0 && len(p.Todos) == 0 {
			return // go-ical won't encode a component-less calendar; not a mutation bug.
		}
		if _, err := p.Encode(); err != nil {
			t.Fatalf("recurrence mutation produced an unencodable object: %v", err)
		}
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		p, err := model.Decode(data, fuzzLoc)
		if err != nil {
			return
		}
		for _, ev := range p.Events {
			if ev.UID == "" {
				continue
			}
			occ := ev.Start
			if out, err := model.AddOccurrenceOverride(p, ev.UID, occ, ev.AllDay, noop, now, fuzzLoc); err == nil {
				mustEncode(t, out)
			}
			if out, err := model.AddException(p, ev.UID, occ, ev.AllDay, now, fuzzLoc); err == nil {
				mustEncode(t, out)
			}
			if capped, future, err := model.SplitEvent(p, ev.UID, occ.Add(24*time.Hour), model.EventDraft{Summary: "x"}, now, fuzzLoc); err == nil {
				mustEncode(t, capped)
				mustEncode(t, future)
			}
			// v1.3.0 rule-rewrite primitives: rewrite to a new rule, and Repeat→None.
			evDraft := model.EventDraft{Summary: "x", Start: ev.Start, End: ev.Start.Add(time.Hour), AllDay: ev.AllDay}
			rewrite := evDraft
			rewrite.Recur = &model.RecurSpec{Freq: model.FreqWeekly, Interval: 2}
			if out, _, err := model.RewriteEventRule(p, ev.UID, rewrite, now, fuzzLoc); err == nil {
				mustEncode(t, out)
			}
			remove := evDraft
			remove.RecurRemove = true
			if out, _, err := model.RewriteEventRule(p, ev.UID, remove, now, fuzzLoc); err == nil {
				mustEncode(t, out)
			}
		}
		for _, td := range p.Todos {
			if td.UID == "" {
				continue
			}
			if out, _, err := model.AdvanceRecurringTodo(p, td.UID, now, fuzzLoc); err == nil {
				mustEncode(t, out)
			}
			// v1.3.0: a todo rule rewrite and Repeat→None through EditTodo.
			tdDraft := model.TodoDraft{Summary: "x", HasDue: td.HasDue, Due: td.Due}
			tdRewrite := tdDraft
			tdRewrite.Recur = &model.RecurSpec{Freq: model.FreqDaily, Count: 5}
			if out, err := model.EditTodo(p, td.UID, tdRewrite, now, fuzzLoc); err == nil {
				mustEncode(t, out)
			}
			tdRemove := tdDraft
			tdRemove.RecurRemove = true
			if out, err := model.EditTodo(p, td.UID, tdRemove, now, fuzzLoc); err == nil {
				mustEncode(t, out)
			}
		}
	})
}

// hasIntentAnchor reports whether input carries at least one token whose shape
// could plausibly anchor a warning: a !-prefixed run, an @-prefixed token, an
// anchor word (next/every/in), or a token containing a date/time separator. It
// is intentionally coarse and independent of the parser so it can cross-check
// the "warnings only fire on an intent anchor" invariant.
func hasIntentAnchor(input string) bool {
	for _, tok := range strings.Fields(input) {
		switch lt := strings.ToLower(tok); {
		case strings.HasPrefix(tok, "!") && len(tok) > 1,
			strings.HasPrefix(tok, "@"),
			lt == "next" || lt == "every" || lt == "in",
			strings.ContainsAny(tok, ":-/"):
			return true
		}
	}
	return false
}

// FuzzParseQuickAdd checks the smart parser never panics and always returns a
// well-formed result: priority within range, any parsed clock in-range, and the
// derived At() time computed without panic. The parser must leave ambiguous
// tokens in the title rather than crash on hostile text.
func FuzzParseQuickAdd(f *testing.F) {
	seeds := []string{
		"buy milk tomorrow 3pm !high #home",
		"jul 20 meeting", "7/20/2026 !3 #work", "2026-07-20 15:00",
		"!9 !1 fri sat 3:30pm 9am", "#", "!", "###tag", "!!!", "12:99",
		"25:00", "0/0 99/99", "  ", "\t\n", "mon tue wed", "tod tom tmr",
		// v1.2.0 grammar: relative dates, time ranges, recurrence, location, warnings.
		"call next fri", "ship in 3 days", "x next month", "in 999 months",
		"meeting 5-6pm", "party 11pm-1am", "sync 14:00-15:30", "5-6xm", "5pm-",
		"standup daily", "gym every mon", "party every jul 20", "rent monthly",
		"lunch @cafeteria", "class @\"room 204\" 9am", "x @\"unclosed",
		"task !hgh", "next tuedsay", "in 3 dayz", "2026-07-40", "2/30/2026",
		"My Event!!!!!", "email bob@example.com", "24/7 support", "http://x.com",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	base := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)

	f.Fuzz(func(t *testing.T, input string) {
		qa := model.ParseQuickAdd(input, now, fuzzLoc)

		if qa.Priority < 0 || qa.Priority > 9 {
			t.Fatalf("priority %d out of the 0..9 range for input %q", qa.Priority, input)
		}
		if qa.HasTime {
			if qa.Hour < 0 || qa.Hour > 23 {
				t.Fatalf("hour %d out of range for input %q", qa.Hour, input)
			}
			if qa.Minute < 0 || qa.Minute > 59 {
				t.Fatalf("minute %d out of range for input %q", qa.Minute, input)
			}
		}
		if qa.HasEnd {
			if qa.EndHour < 0 || qa.EndHour > 23 || qa.EndMinute < 0 || qa.EndMinute > 59 {
				t.Fatalf("end %d:%02d out of range for input %q", qa.EndHour, qa.EndMinute, input)
			}
		}
		// At must not panic for either the parsed-date or context-day path; EndAt
		// must not panic when a range was parsed.
		start, _ := qa.At(base, fuzzLoc)
		if qa.HasEnd {
			qa.EndAt(start)
		}

		// A warning must only ever fire alongside an intent anchor — the whole point
		// of the warning system is that plausible text is silent. This detector is
		// deliberately independent of the parser's own warning code (coarse token
		// shapes over strings.Fields), so a false positive on clean text is caught.
		if len(qa.Warnings) > 0 && !hasIntentAnchor(input) {
			t.Fatalf("warning(s) %v fired with no intent anchor in %q", qa.Warnings, input)
		}
	})
}
