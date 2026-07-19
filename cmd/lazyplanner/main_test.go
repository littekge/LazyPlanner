package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRunUnknownCommand guards B1: an unrecognized subcommand exits non-zero
// (rather than silently opening the TUI, which hid typos like "imprt").
func TestRunUnknownCommand(t *testing.T) {
	if code := run([]string{"imprt"}); code != 2 {
		t.Errorf("run(unknown) exit code = %d, want 2", code)
	}
}

// TestRunHelpAndVersion: help and version exit 0 without touching the TUI.
func TestRunHelpAndVersion(t *testing.T) {
	for _, arg := range []string{"help", "-h", "--help", "version", "-v", "--version"} {
		if code := run([]string{arg}); code != 0 {
			t.Errorf("run(%q) exit code = %d, want 0", arg, code)
		}
	}
}

// TestVersionStringSurfacesInjectedVersion guards the build-time version
// injection: `version` output must include the appVersion var (set via
// -ldflags "-X main.appVersion=..."), so a git-tag build reports its tag rather
// than a hardcoded string. If the version case stopped using appVersion, an
// injected value would silently not appear.
func TestVersionStringSurfacesInjectedVersion(t *testing.T) {
	if appVersion == "" {
		t.Fatal("appVersion is empty; the ldflags default should be non-empty")
	}
	got := versionString()
	if !strings.Contains(got, appName) {
		t.Errorf("versionString() = %q, want it to contain appName %q", got, appName)
	}
	if !strings.Contains(got, appVersion) {
		t.Errorf("versionString() = %q, want it to contain appVersion %q (the injected value)", got, appVersion)
	}
}

// TestPrintUsageListsSubcommands: usage names each real subcommand so the help is
// actually useful.
func TestPrintUsageListsSubcommands(t *testing.T) {
	var b bytes.Buffer
	printUsage(&b)
	out := b.String()
	for _, want := range []string{"import", "sync", "calendar", "version", "help"} {
		if !strings.Contains(out, want) {
			t.Errorf("usage missing %q:\n%s", want, out)
		}
	}
}

// TestEditorCommandSplitsArgs guards pass-10 MED #6: a flag-bearing $EDITOR must
// split into command + args so it isn't treated as one (nonexistent) binary name.
func TestEditorCommandSplitsArgs(t *testing.T) {
	cases := map[string][]string{
		"code --wait":    {"code", "--wait", "/cfg"},
		"vim":            {"vim", "/cfg"},
		"emacsclient -c": {"emacsclient", "-c", "/cfg"},
		"":               {"vi", "/cfg"}, // default
	}
	for env, want := range cases {
		got := editorCommand(env, "/cfg").Args
		if len(got) != len(want) {
			t.Errorf("editorCommand(%q).Args = %v, want %v", env, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("editorCommand(%q).Args = %v, want %v", env, got, want)
				break
			}
		}
	}
}
