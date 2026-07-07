package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// CreateCalendarLocal creates a calendar collection locally, marked to be
// created on the server (MKCALENDAR) on the next sync (offline-first). meta
// carries the display name and optional color; components is the iCalendar
// component set (e.g. ["VTODO"] for a task list; empty means both VEVENT and
// VTODO). It errors if a calendar with that id already exists.
func (s *Store) CreateCalendarLocal(ctx context.Context, id string, meta CalendarMeta, components []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if id == "" {
		return errors.New("store: CreateCalendarLocal requires a calendar id")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.cals[id]; exists {
		return fmt.Errorf("store: calendar %q already exists", id)
	}
	if err := os.MkdirAll(filepath.Join(s.root, id), dirPerm); err != nil {
		return fmt.Errorf("creating calendar %q: %w", id, err)
	}
	cs := &calState{
		id:            id,
		displayName:   meta.DisplayName,
		color:         meta.Color,
		resources:     map[string]*Resource{},
		conflicts:     map[string]conflictMeta{},
		pendingCreate: true,
		components:    components,
	}
	s.cals[id] = cs
	if err := writeSidecar(s.root, cs); err != nil {
		return fmt.Errorf("updating sidecar for %q: %w", id, err)
	}
	return nil
}

// MarkCalendarDeleted marks a calendar for deletion. It disappears from
// Calendars() immediately; the next sync issues the server DELETE and then
// removes it locally. A calendar that was never pushed (still pending-create) is
// removed outright, with no server round-trip.
func (s *Store) MarkCalendarDeleted(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	cs := s.cals[id]
	if cs == nil {
		s.mu.Unlock()
		return fmt.Errorf("store: unknown calendar %q", id)
	}
	neverPushed := cs.pendingCreate
	if !neverPushed {
		cs.pendingDelete = true
		err := writeSidecar(s.root, cs)
		s.mu.Unlock()
		if err != nil {
			return fmt.Errorf("updating sidecar for %q: %w", id, err)
		}
		return nil
	}
	s.mu.Unlock()
	// Never synced — nothing on the server to delete; drop it locally.
	return s.RemoveCalendarLocal(ctx, id)
}

// RemoveCalendarLocal removes a calendar's directory and index entry outright.
// The sync layer calls it after a successful server DELETE; MarkCalendarDeleted
// calls it directly for a never-pushed calendar.
func (s *Store) RemoveCalendarLocal(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.cals[id]; !ok {
		return fmt.Errorf("store: unknown calendar %q", id)
	}
	if err := os.RemoveAll(filepath.Join(s.root, id)); err != nil {
		return fmt.Errorf("removing calendar %q: %w", id, err)
	}
	delete(s.cals, id)
	return nil
}

// MarkCalendarSynced records that a pending-create calendar now exists on the
// server at href, clearing its pending-create flag. The sync layer calls it
// after a successful MKCALENDAR.
func (s *Store) MarkCalendarSynced(ctx context.Context, id, href string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cs := s.cals[id]
	if cs == nil {
		return fmt.Errorf("store: unknown calendar %q", id)
	}
	cs.pendingCreate = false
	cs.href = href
	if err := writeSidecar(s.root, cs); err != nil {
		return fmt.Errorf("updating sidecar for %q: %w", id, err)
	}
	return nil
}

// CalendarDeletion is a calendar marked for deletion that the sync layer must
// remove on the server. Href is empty for a calendar that was never pushed.
type CalendarDeletion struct {
	ID   string
	Href string
}

// PendingCalendarDeletes returns calendars marked for deletion (which are hidden
// from Calendars()), for the sync layer to remove server-side.
func (s *Store) PendingCalendarDeletes() []CalendarDeletion {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []CalendarDeletion
	for _, cs := range s.cals {
		if cs.pendingDelete {
			out = append(out, CalendarDeletion{ID: cs.id, Href: cs.href})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
