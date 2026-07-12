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

// CommitPush finalizes a successful server write of `pushed` — the exact
// *Resource the sync goroutine encoded and PUT. Sync runs on a background
// goroutine while the UI keeps editing on the event loop, so between the PUT
// starting and this call a local edit may have replaced the resource. Because
// every mutation swaps in a fresh *Resource (copy-on-write), pointer identity is
// the concurrency signal: if the current resource is still `pushed`, adopt the
// server's new ETag/href and mark it clean; if a concurrent edit replaced it,
// keep that newer content and leave it dirty, but advance the ETag baseline to
// etag so the next push is conditional on what is now on the server. This avoids
// the lost update where the stale pushed snapshot was written back as clean,
// silently reverting the edit and suppressing its push.
func (s *Store) CommitPush(ctx context.Context, calID, name string, pushed *Resource, etag, href string) (*Resource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if calID == "" || name == "" {
		return nil, errors.New("store: CommitPush requires a calendar id and resource name")
	}
	if pushed == nil || pushed.Object == nil {
		return nil, errors.New("store: CommitPush requires the pushed resource")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeResourceLocked(calID, name, func(cur *Resource) *Resource {
		if cur == nil || cur == pushed {
			return &Resource{Name: name, Object: pushed.Object, ETag: etag, Href: href, Dirty: false}
		}
		return &Resource{Name: name, Object: cur.Object, ETag: etag, Href: href, Dirty: true}
	})
}

// PullRemote writes a server object into the cache as a clean resource, like
// PutRemote, but guarded against clobbering a concurrent local edit. When
// expectedPrev is non-nil, the write is applied only if the current resource is
// still that same snapshot (pointer identity); if a local edit replaced it while
// the sync was reconciling, the pull is skipped (applied=false) so the edit is
// preserved and the next sync reconciles it as a proper conflict. A nil
// expectedPrev means an unconditional write (a brand-new remote resource).
func (s *Store) PullRemote(ctx context.Context, calID, name string, obj *model.Parsed, etag, href string, expectedPrev *Resource) (applied bool, err error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if calID == "" || name == "" {
		return false, errors.New("store: PullRemote requires a calendar id and resource name")
	}
	if obj == nil || obj.Calendar == nil {
		return false, errors.New("store: PullRemote requires a decoded object")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if expectedPrev != nil {
		var cur *Resource
		if cs := s.cals[calID]; cs != nil {
			cur = cs.resources[name]
		}
		if cur != expectedPrev {
			return false, nil // a concurrent local edit landed; don't overwrite it
		}
	}
	if _, err := s.writeResourceLocked(calID, name, func(*Resource) *Resource {
		return &Resource{Name: name, Object: obj, ETag: etag, Href: href, Dirty: false}
	}); err != nil {
		return false, err
	}
	return true, nil
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
	if cs.syncToken == token {
		return nil
	}
	cs.syncToken = token
	if err := writeSidecar(s.root, cs); err != nil {
		return fmt.Errorf("updating sidecar for %q: %w", calID, err)
	}
	return nil
}
