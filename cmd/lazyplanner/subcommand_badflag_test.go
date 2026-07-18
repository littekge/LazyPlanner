package main

import (
	"os"
	"strings"
	"testing"
)

// TestSubcommandBadFlagPrintsErrorOnce guards the fix: a bad/unknown subcommand
// flag prints its error message exactly once. Previously flag.ContinueOnError
// wrote the error + usage to fs.Output(), then report() printed the same error
// again; parseFlags now tags it errFlagParsed so report() exits without re-printing.
func TestSubcommandBadFlagPrintsErrorOnce(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	code := run([]string{"import", "-badflag"})

	w.Close()
	os.Stderr = origStderr
	buf := make([]byte, 64*1024)
	n, _ := r.Read(buf)
	out := string(buf[:n])

	msg := "flag provided but not defined: -badflag"
	got := strings.Count(out, msg)

	t.Logf("exit code=%d\nstderr:\n%s", code, out)
	if got != 1 {
		t.Errorf("error message printed %d times, want 1:\n%s", got, out)
	}
}
