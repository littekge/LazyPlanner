package store_test

import (
	"context"
	"testing"

	"github.com/littekge/LazyPlanner/internal/store"
)

func TestSetCalendarReadOnlyPersists(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetCalendarMeta(ctx, "birthdays", store.CalendarMeta{DisplayName: "Contact Birthdays", Href: "/dav/birthdays/"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCalendarReadOnly(ctx, "birthdays", true); err != nil {
		t.Fatal(err)
	}

	cal, _ := s.Calendar("birthdays")
	if !cal.ReadOnly {
		t.Fatal("ReadOnly not set on snapshot")
	}

	// Survives a reload from disk (so the UI knows offline, before any sync).
	s2, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	cal2, _ := s2.Calendar("birthdays")
	if !cal2.ReadOnly {
		t.Error("ReadOnly flag lost across reload")
	}
}
