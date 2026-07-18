package main

import (
	"os"
	"strings"
	"testing"
)

// TestSubcommandHelpFlagExitsZero guards the fix: `-h`/`--help` on a subcommand
// prints usage and exits 0 without the spurious "lazyplanner: flag: help
// requested" line. Previously the subcommands returned flag.ErrHelp from fs.Parse
// straight into report(); parseFlags + report now treat ErrHelp as a clean exit.
func TestSubcommandHelpFlagExitsZero(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	code := run([]string{"import", "-h"})

	w.Close()
	os.Stderr = origStderr
	buf := make([]byte, 64*1024)
	n, _ := r.Read(buf)
	out := string(buf[:n])

	t.Logf("exit code=%d\nstderr:\n%s", code, out)
	if code != 0 {
		t.Errorf("`import -h` exit code = %d, want 0", code)
	}
	if strings.Contains(out, "flag: help requested") {
		t.Errorf("`import -h` emitted spurious error line:\n%s", out)
	}
}
