package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/littekge/LazyPlanner/internal/model"
)

// CalendarMeta is the server-owned metadata for a calendar collection, cached
// locally in the sidecar. It is data, not config — the CalDAV server is the
// source of truth for a calendar's name and color.
type CalendarMeta struct {
	DisplayName string
	Color       string
	Href        string
	SyncToken   string
}

// SetCalendarMeta records server-owned metadata for a calendar, creating its
// directory on first use. All fields are replaced (the server is authoritative
// for metadata); the resource set is untouched.
func (s *Store) SetCalendarMeta(ctx context.Context, calID string, meta CalendarMeta) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if calID == "" {
		return errors.New("store: SetCalendarMeta requires a calendar id")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cs, err := s.ensureCalendar(calID)
	if err != nil {
		return err
	}
	cs.displayName = meta.DisplayName
	cs.color = meta.Color
	cs.href = meta.Href
	cs.syncToken = meta.SyncToken

	if err := writeSidecar(s.root, cs); err != nil {
		return fmt.Errorf("updating sidecar for %q: %w", calID, err)
	}
	return nil
}

// PutRemote writes a resource fetched from the server: clean (not dirty), with
// the server's ETag and href. Used by import and, later, sync when pulling
// remote changes into the local cache.
func (s *Store) PutRemote(ctx context.Context, calID, name string, obj *model.Parsed, etag, href string) (*Resource, error) {
	return s.writeResource(ctx, calID, name, obj, func(*Resource) *Resource {
		return &Resource{Name: name, Object: obj, ETag: etag, Href: href, Dirty: false}
	})
}

// SetSyncToken updates the cached CalDAV sync token for a calendar and persists
// the sidecar. It is a no-op error if the calendar is unknown.
func (s *Store) SetSyncToken(ctx context.Context, calID, token string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cs := s.cals[calID]
	if cs == nil {
		return fmt.Errorf("store: unknown calendar %q", calID)
	}
	cs.syncToken = token
	return writeSidecar(s.root, cs)
}
