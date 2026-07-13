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
