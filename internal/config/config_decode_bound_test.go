package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLoadDoesNotHangOnDeeplyNestedConfig guards the O(depth^2) TOML decode: a
// deeply-nested inline-table config well under maxConfigBytes hangs the unbounded
// decoder for tens of seconds (measured ~1s at depth 4000, quadratic), defeating
// the read cap's "never hang startup" purpose. Load must reject it near-instantly.
func TestLoadDoesNotHangOnDeeplyNestedConfig(t *testing.T) {
	dir := withConfigDir(t)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// ~120 KB, deep enough that the unbounded decode would run for ~30s.
	const depth = 20000
	body := "k = " + strings.Repeat("{k = ", depth) + "1" + strings.Repeat("}", depth) + "\n"
	if err := os.WriteFile(filepath.Join(dir, configName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, _, _, err := Load()
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Load accepted a pathologically nested config; want a rejection error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Load did not return within 2s on a deeply-nested config — the decode bound is missing")
	}
}

// TestCheckNestingDepth pins the guard precisely and deterministically: a shallow
// config whose only brackets live inside strings/comments must pass (no false
// rejection), nesting at exactly the cap passes, and one level past it is rejected.
func TestCheckNestingDepth(t *testing.T) {
	ok := `
[[account]]
name = "personal"
password = "` + strings.Repeat("{[", 100) + `"  # a comment with }}}} and {{{{
url = "https://x/[a]/{b}"
notes = '''` + strings.Repeat("{", 50) + `'''
`
	if err := checkNestingDepth([]byte(ok)); err != nil {
		t.Errorf("rejected a shallow config whose brackets are only in strings/comments: %v", err)
	}

	atCap := "k = " + strings.Repeat("{k = ", maxTOMLNestingDepth) + "1" + strings.Repeat("}", maxTOMLNestingDepth)
	if err := checkNestingDepth([]byte(atCap)); err != nil {
		t.Errorf("rejected nesting at exactly the cap %d: %v", maxTOMLNestingDepth, err)
	}

	overCap := "k = " + strings.Repeat("{k = ", maxTOMLNestingDepth+1) + "1" + strings.Repeat("}", maxTOMLNestingDepth+1)
	if err := checkNestingDepth([]byte(overCap)); err == nil {
		t.Errorf("accepted nesting one level past the cap of %d", maxTOMLNestingDepth)
	}
}
