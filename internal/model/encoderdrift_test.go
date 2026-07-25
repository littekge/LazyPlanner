package model

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/emersion/go-ical"
)

// Drift tripwire for the four hand-maintained tables in this package that mirror
// go-ical's encoder rules: allowedChildren, encoderValidatedComponents,
// singleValuedProps, and the DTSTAMP-heal set. Re-diffing them BY HAND against the
// vendored encoder is the manual step that failed in three of the four reopenings
// of the "decodes but can't re-encode" class (passes 10, 16, 21, 23), so these
// tests do the diff mechanically instead: they parse
// vendor/github.com/emersion/go-ical/encoder.go and fail when checkComponent and
// our tables have moved apart.
//
// Parsing is done with go/ast rather than a regex. The switch bodies mix case
// clauses, nested `if` guards over comp.Children, and two composite literals per
// case; a regex would have to model Go's block structure to attribute a literal to
// the right case, and would silently under-match (i.e. pass) on any formatting
// change — the exact failure mode this tripwire exists to prevent. go/ast is in the
// stdlib, so it costs no dependency.

const goICalVendorDir = "../../vendor/github.com/emersion/go-ical"

// encoderRules is what checkComponent actually enforces, extracted from source.
type encoderRules struct {
	// components is every component name checkComponent has a case for.
	components []string
	// cardinality maps a component name to the union of its exactlyOneProps and
	// atMostOneProps lists — the properties whose duplication fails an encode.
	cardinality map[string][]string
	// requiresDTStamp is the subset of components whose exactlyOneProps lists
	// DTSTAMP.
	requiresDTStamp []string
	// hasDefault reports whether the switch grew a `default:` clause with a body.
	hasDefault bool
}

// TestEncoderValidatedComponentsMatchesVendoredCheckComponent fails when
// checkComponent's switch and encoderValidatedComponents have drifted apart in
// either direction.
//
// A MISSING entry is the dangerous direction: encoderValidatedComponents is the
// strip criterion for an unknown container's children (stripForbiddenChildren), so
// a component type go-ical newly validates but we don't list survives nested inside
// an X-/VAVAILABILITY container, fails checkComponent at encode time, and bricks
// the whole resource — every valid sibling item with it.
func TestEncoderValidatedComponentsMatchesVendoredCheckComponent(t *testing.T) {
	rules := parseEncoderRules(t)

	if rules.hasDefault {
		t.Errorf("go-ical's checkComponent switch has grown a `default:` clause.\n"+
			"That invalidates the premise of encoderValidatedComponents (%s): membership is\n"+
			"the set of types that CAN fail an encode, which relied on an unlisted type getting\n"+
			"nil prop lists and always returning nil. Re-read checkComponent and rewrite\n"+
			"stripForbiddenChildren's criterion before updating the table.", "internal/model/decode.go")
	}

	inSource := map[string]bool{}
	for _, name := range rules.components {
		inSource[name] = true
	}

	var missing, stale []string
	for _, name := range rules.components {
		if !encoderValidatedComponents[name] {
			missing = append(missing, name)
		}
	}
	for name := range encoderValidatedComponents {
		if !inSource[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)

	if len(missing) > 0 {
		t.Errorf("go-ical's checkComponent validates %v, which encoderValidatedComponents does not list.\n"+
			"TO FIX (internal/model/decode.go):\n"+
			"  1. Add each name to encoderValidatedComponents, so stripForbiddenChildren removes it\n"+
			"     when it appears nested inside an unknown container (else one phantom instance\n"+
			"     makes the whole resource unencodable — items included).\n"+
			"  2. Decide whether the type is a legal CONTAINER: if checkComponent recurses into or\n"+
			"     constrains its children, give it an allowedChildren entry (an empty map means\n"+
			"     'admits nothing').\n"+
			"  3. Mirror its exactlyOneProps/atMostOneProps into singleValuedProps, and if its\n"+
			"     exactlyOneProps includes DTSTAMP, make sure Parse/healComponentConstraints heals it.\n"+
			"See the 'Malformed iCalendar is contained/healed at ingest' guardrail in CLAUDE.md.", missing)
	}
	if len(stale) > 0 {
		t.Errorf("encoderValidatedComponents lists %v, which checkComponent no longer validates.\n"+
			"TO FIX (internal/model/decode.go): drop the stale entries. Keeping them makes\n"+
			"stripForbiddenChildren delete children that can never fail an encode — destroying\n"+
			"user data (an RFC 7953 VAVAILABILITY's sub-components, a vendor's X- payload) to buy\n"+
			"nothing.", stale)
	}
}

// TestSingleValuedPropsMatchesVendoredCardinalityRules fails when a component's
// exactlyOneProps/atMostOneProps in checkComponent and our singleValuedProps entry
// have drifted. A property go-ical newly caps that we don't dedupe means a foreign
// object carrying a duplicate decodes but cannot be saved; a property we dedupe
// that go-ical no longer caps means we silently drop a legitimate repeat (iron-rule
// violation).
func TestSingleValuedPropsMatchesVendoredCardinalityRules(t *testing.T) {
	rules := parseEncoderRules(t)

	for _, comp := range rules.components {
		want := rules.cardinality[comp]
		got := singleValuedProps[comp]
		missing := setDiff(want, got)
		extra := setDiff(got, want)
		if len(missing) > 0 {
			t.Errorf("singleValuedProps[%q] is missing %v — go-ical caps these but dedupeSingleValued does not,\n"+
				"so a foreign object carrying a duplicate decodes and then fails to encode, bricking the\n"+
				"whole resource. TO FIX: add them to singleValuedProps in internal/model/decode.go.", comp, missing)
		}
		if len(extra) > 0 {
			t.Errorf("singleValuedProps[%q] lists %v, which go-ical no longer caps.\n"+
				"TO FIX: drop them from singleValuedProps in internal/model/decode.go — deduping a property\n"+
				"the encoder accepts repeated silently destroys data the iron rule protects.", comp, extra)
		}
	}
	for comp := range singleValuedProps {
		if len(rules.cardinality[comp]) == 0 {
			t.Errorf("singleValuedProps has an entry for %q, but checkComponent enforces no cardinality on it.\n"+
				"TO FIX: drop the entry (internal/model/decode.go) — it can only destroy data.", comp)
		}
	}
}

// TestDTStampHealCoversEveryComponentThatRequiresIt derives the DTSTAMP-heal set
// from the vendored encoder instead of restating it: every component type whose
// exactlyOneProps includes DTSTAMP must round-trip through Decode→Encode when the
// input omits DTSTAMP. Parse heals VEVENT/VTODO directly and
// healComponentConstraints heals VJOURNAL/VFREEBUSY; if go-ical adds a fifth, this
// fails rather than waiting for a foreign object to brick a resource in the field.
func TestDTStampHealCoversEveryComponentThatRequiresIt(t *testing.T) {
	rules := parseEncoderRules(t)
	if len(rules.requiresDTStamp) == 0 {
		t.Fatal("parsed no DTSTAMP-requiring components from checkComponent; the extraction is broken, not the tables")
	}

	for _, comp := range rules.requiresDTStamp {
		t.Run(comp, func(t *testing.T) {
			// UID is the one required prop deliberately never healed (a fabricated UID
			// churns sync identity), so supply it and omit only DTSTAMP. DTSTART is
			// supplied too: it is at-most-one (never a duplicate hazard) on every one of
			// these components, and this package's own ingest rejects a dateless VEVENT
			// — which would mask the DTSTAMP question this test asks.
			ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
				"BEGIN:" + comp + "\r\nUID:drift@test\r\nDTSTART:20260704T120000Z\r\nEND:" + comp + "\r\n" +
				"END:VCALENDAR\r\n"
			obj, err := Decode([]byte(ics), nil)
			if err != nil {
				t.Fatalf("a %s without DTSTAMP failed to DECODE: %v\n"+
					"TO FIX: ingest must not reject it; heal it (internal/model/decode.go).", comp, err)
			}
			if _, err := obj.Encode(); err != nil {
				t.Fatalf("a %s without DTSTAMP decodes but cannot be re-encoded: %v\n"+
					"go-ical's checkComponent requires exactly one DTSTAMP on %s, so this object is\n"+
					"unsavable — every valid sibling item in the resource with it.\n"+
					"TO FIX (internal/model/decode.go): call ensureDTStamp on %s, in Parse if the app\n"+
					"parses it into a typed item, else in healComponentConstraints.", comp, err, comp, comp)
			}
		})
	}
}

// parseEncoderRules extracts checkComponent's switch from the vendored source.
func parseEncoderRules(t *testing.T) encoderRules {
	t.Helper()
	consts := parseVendoredConsts(t)

	path := filepath.Join(goICalVendorDir, "encoder.go")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cannot read %s: %v\n"+
			"Dependencies are vendored and committed in this repo (CLAUDE.md), so the file must exist;\n"+
			"if go-ical moved or was replaced, update goICalVendorDir and re-diff the tables by hand once.", path, err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}

	sw := findCheckComponentSwitch(file)
	if sw == nil {
		t.Fatalf("could not find checkComponent's `switch comp.Name` in %s.\n"+
			"go-ical restructured its encoder, so this tripwire no longer checks anything: re-diff\n"+
			"allowedChildren / encoderValidatedComponents / singleValuedProps / the DTSTAMP-heal set\n"+
			"against the new code BY HAND, then update this extraction to match.", path)
	}

	rules := encoderRules{cardinality: map[string][]string{}}
	for _, stmt := range sw.Body.List {
		clause, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}
		if clause.List == nil {
			rules.hasDefault = len(clause.Body) > 0
			continue
		}
		exactlyOne := propListAssignedTo(clause, "exactlyOneProps", consts, t)
		atMostOne := propListAssignedTo(clause, "atMostOneProps", consts, t)
		for _, expr := range clause.List {
			ident, ok := expr.(*ast.Ident)
			if !ok {
				t.Fatalf("checkComponent has a non-identifier case expression (%T); the extraction needs updating", expr)
			}
			name, ok := consts[ident.Name]
			if !ok {
				t.Fatalf("case %q in checkComponent resolves to no string constant in the vendored package;\n"+
					"the extraction needs updating.", ident.Name)
			}
			rules.components = append(rules.components, name)
			rules.cardinality[name] = append(append([]string{}, exactlyOne...), atMostOne...)
			for _, p := range exactlyOne {
				if p == ical.PropDateTimeStamp {
					rules.requiresDTStamp = append(rules.requiresDTStamp, name)
				}
			}
		}
	}
	if len(rules.components) == 0 {
		t.Fatal("extracted no component cases from checkComponent; the extraction is broken, not the tables")
	}
	return rules
}

// findCheckComponentSwitch returns the `switch comp.Name` statement inside
// checkComponent, or nil.
func findCheckComponentSwitch(file *ast.File) *ast.SwitchStmt {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "checkComponent" || fn.Body == nil {
			continue
		}
		var found *ast.SwitchStmt
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			sw, ok := n.(*ast.SwitchStmt)
			if !ok || found != nil {
				return true
			}
			if sel, ok := sw.Tag.(*ast.SelectorExpr); ok && sel.Sel.Name == "Name" {
				found = sw
			}
			return true
		})
		return found
	}
	return nil
}

// propListAssignedTo returns the resolved string values of the []string composite
// literal assigned to the named variable directly in this case clause's body.
func propListAssignedTo(clause *ast.CaseClause, varName string, consts map[string]string, t *testing.T) []string {
	t.Helper()
	var out []string
	for _, stmt := range clause.Body {
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			continue
		}
		lhs, ok := assign.Lhs[0].(*ast.Ident)
		if !ok || lhs.Name != varName {
			continue
		}
		lit, ok := assign.Rhs[0].(*ast.CompositeLit)
		if !ok {
			t.Fatalf("%s is assigned a %T, not a composite literal; the extraction needs updating", varName, assign.Rhs[0])
		}
		for _, elt := range lit.Elts {
			ident, ok := elt.(*ast.Ident)
			if !ok {
				t.Fatalf("%s contains a non-identifier element (%T); the extraction needs updating", varName, elt)
			}
			value, ok := consts[ident.Name]
			if !ok {
				t.Fatalf("%s references %q, which resolves to no string constant in the vendored package", varName, ident.Name)
			}
			out = append(out, value)
		}
	}
	return out
}

// parseVendoredConsts maps every top-level string constant in the vendored go-ical
// package to its value, so the AST's identifiers (CompEvent, PropDateTimeStamp, …)
// can be resolved to the wire names our tables are keyed by.
func parseVendoredConsts(t *testing.T) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(goICalVendorDir)
	if err != nil {
		t.Fatalf("cannot read %s: %v (dependencies are vendored and committed; see CLAUDE.md)", goICalVendorDir, err)
	}
	consts := map[string]string{}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(goICalVendorDir, e.Name()), nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", e.Name(), err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if i >= len(vs.Values) {
						continue
					}
					lit, ok := vs.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					if v, err := strconv.Unquote(lit.Value); err == nil {
						consts[name.Name] = v
					}
				}
			}
		}
	}
	if len(consts) == 0 {
		t.Fatalf("found no string constants under %s; the extraction is broken", goICalVendorDir)
	}
	return consts
}

// setDiff returns the members of a that are absent from b, sorted and deduped.
func setDiff(a, b []string) []string {
	inB := map[string]bool{}
	for _, s := range b {
		inB[s] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, s := range a {
		if !inB[s] && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
