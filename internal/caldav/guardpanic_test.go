package caldav

import (
	"bytes"
	"testing"

	"github.com/emersion/go-ical"
)

// TestGuardICalPanicContainsDecoderPanic covers the sync-path crash: go-webdav
// decodes server calendar-data with go-ical, whose decoder panics (index out of
// range) on a content line ending mid-parameter. guardICalPanic must convert
// that panic into an ordinary error so a hostile/buggy server response degrades
// to a skipped resource instead of crashing the app. If the recover regressed,
// this test would panic rather than fail.
func TestGuardICalPanicContainsDecoderPanic(t *testing.T) {
	err := guardICalPanic(func() error {
		_, decErr := ical.NewDecoder(bytes.NewReader([]byte("0;0="))).Decode()
		return decErr
	})
	if err == nil {
		t.Fatal("expected an error from a panicking decode, got nil")
	}
}

// TestGuardICalPanicPassesThroughNormalError confirms the guard is transparent
// when fn returns an ordinary error (no panic to recover).
func TestGuardICalPanicPassesThroughNormalError(t *testing.T) {
	sentinel := "boom"
	err := guardICalPanic(func() error { return &stringErr{sentinel} })
	if err == nil || err.Error() != sentinel {
		t.Fatalf("guard altered a normal error: got %v, want %q", err, sentinel)
	}
}

type stringErr struct{ s string }

func (e *stringErr) Error() string { return e.s }
