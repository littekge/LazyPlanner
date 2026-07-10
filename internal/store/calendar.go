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
		// Copy so a later mutation of the caller's slice can't reach store state
		// (matches SetCalendarComponents).
		components: append([]string(nil), components...),
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

// SetCalendarReadOnly records the server's read-only status for a calendar
// (whether the user has write privilege). It is a no-op if the value is
// unchanged, so a routine sync doesn't rewrite the sidecar needlessly.
func (s *Store) SetCalendarReadOnly(ctx context.Context, calID string, readOnly bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cs := s.cals[calID]
	if cs == nil {
		return fmt.Errorf("store: unknown calendar %q", calID)
	}
	if cs.readOnly == readOnly {
		return nil
	}
	cs.readOnly = readOnly
	if err := writeSidecar(s.root, cs); err != nil {
		return fmt.Errorf("updating sidecar for %q: %w", calID, err)
	}
	return nil
}

// UpdateCalendarMeta edits a calendar's display name and/or color locally and
// marks it for a server PROPPATCH on the next sync (offline-first). An empty
// argument leaves that property unchanged. A calendar still pending creation just
// carries the new values into its MKCALENDAR (no separate PROPPATCH needed).
func (s *Store) UpdateCalendarMeta(ctx context.Context, id, displayName, color string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cs := s.cals[id]
	if cs == nil {
		return fmt.Errorf("store: unknown calendar %q", id)
	}
	if displayName != "" {
		cs.displayName = displayName
	}
	if color != "" {
		cs.color = color
	}
	// A never-pushed calendar carries its metadata in the pending MKCALENDAR; only
	// an existing one needs a PROPPATCH.
	if !cs.pendingCreate {
		cs.pendingProps = true
	}
	if err := writeSidecar(s.root, cs); err != nil {
		return fmt.Errorf("updating sidecar for %q: %w", id, err)
	}
	return nil
}

// SyncCalendarColor adopts the server's calendar color for a calendar, so the
// in-app display stays consistent with NextCloud web and other clients. The
// server is authoritative — except when a local color edit is still pending a
// PROPPATCH (that edit wins until it is pushed, so a routine pull doesn't clobber
// it). An empty server color or an unchanged value is a no-op, so a routine sync
// doesn't rewrite the sidecar needlessly.
func (s *Store) SyncCalendarColor(ctx context.Context, calID, serverColor string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if serverColor == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cs := s.cals[calID]
	if cs == nil {
		return fmt.Errorf("store: unknown calendar %q", calID)
	}
	if cs.pendingProps || cs.color == serverColor {
		return nil
	}
	cs.color = serverColor
	if err := writeSidecar(s.root, cs); err != nil {
		return fmt.Errorf("updating sidecar for %q: %w", calID, err)
	}
	return nil
}

// SyncCalendarName adopts the server's display name for a calendar (mirroring
// SyncCalendarColor), so a rename done on NextCloud web or another client shows
// up locally. The server is authoritative except when a local rename is still
// pending a PROPPATCH (that edit wins until pushed). Empty/unchanged is a no-op.
func (s *Store) SyncCalendarName(ctx context.Context, calID, serverName string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if serverName == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cs := s.cals[calID]
	if cs == nil {
		return fmt.Errorf("store: unknown calendar %q", calID)
	}
	if cs.pendingProps || cs.displayName == serverName {
		return nil
	}
	cs.displayName = serverName
	if err := writeSidecar(s.root, cs); err != nil {
		return fmt.Errorf("updating sidecar for %q: %w", calID, err)
	}
	return nil
}

// MarkCalendarPropsSynced clears a calendar's pending-props flag after a
// successful server PROPPATCH.
func (s *Store) MarkCalendarPropsSynced(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cs := s.cals[id]
	if cs == nil {
		return fmt.Errorf("store: unknown calendar %q", id)
	}
	cs.pendingProps = false
	if err := writeSidecar(s.root, cs); err != nil {
		return fmt.Errorf("updating sidecar for %q: %w", id, err)
	}
	return nil
}

// CalendarPropUpdate is a calendar whose metadata changed locally and must be
// pushed to the server with a PROPPATCH.
type CalendarPropUpdate struct {
	ID          string
	Href        string
	DisplayName string
	Color       string
}

// PendingCalendarProps returns calendars whose display name/color changed locally
// and need a server PROPPATCH (only those already on the server).
func (s *Store) PendingCalendarProps() []CalendarPropUpdate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []CalendarPropUpdate
	for _, cs := range s.cals {
		if cs.pendingProps && cs.href != "" {
			out = append(out, CalendarPropUpdate{ID: cs.id, Href: cs.href, DisplayName: cs.displayName, Color: cs.color})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// SetCalendarComponents records the calendar's supported iCalendar component set
// (from the server's supported-calendar-component-set), so the UI can tell a task
// list from an event calendar even when it is empty. It is a no-op when unchanged
// so a routine sync doesn't rewrite the sidecar needlessly, and it leaves a
// locally-created calendar's own component set untouched (empty input).
func (s *Store) SetCalendarComponents(ctx context.Context, calID string, components []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(components) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cs := s.cals[calID]
	if cs == nil {
		return fmt.Errorf("store: unknown calendar %q", calID)
	}
	if equalStrings(cs.components, components) {
		return nil
	}
	cs.components = append([]string(nil), components...)
	if err := writeSidecar(s.root, cs); err != nil {
		return fmt.Errorf("updating sidecar for %q: %w", calID, err)
	}
	return nil
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
