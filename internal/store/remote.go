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
// starting and this call a local edit or delete may have replaced or removed the
// resource. Because every mutation swaps in a fresh *Resource (copy-on-write),
// pointer identity is the concurrency signal:
//
//   - current resource is still `pushed` → adopt the server's new ETag/href and
//     mark it clean.
//   - a concurrent edit replaced it → keep that newer content and leave it dirty,
//     but advance the ETag baseline to etag so the next push is conditional on
//     what is now on the server. This avoids the lost update where the stale
//     pushed snapshot was written back as clean, silently reverting the edit.
//   - the resource is gone (a concurrent local delete landed mid-push) → honor the
//     deletion instead of resurrecting it. Our PUT just wrote `pushed` to the
//     server, so the deletion still has server work to do: ensure a tombstone
//     carrying the post-PUT href/ETag so the next sync issues the conditional
//     DELETE. This covers both a delete of a synced resource (Delete already left
//     a tombstone — advance its ETag to what our PUT put there so the If-Match
//     matches) and a delete of a never-synced create (Delete left no tombstone
//     because there was no server identity yet — create one, or the server copy we
//     just created is re-pulled and silently resurrected).
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

	cs := s.cals[calID]
	var cur *Resource
	if cs != nil {
		cur = cs.resources[name]
	}
	if cur == nil {
		// The resource was deleted on the event loop while our PUT was in flight.
		// Don't resurrect it; schedule the server-side deletion of what we just wrote.
		return nil, s.honorMidPushDeleteLocked(cs, calID, name, href, etag)
	}

	return s.writeResourceLocked(calID, name, func(cur *Resource) *Resource {
		if cur == pushed {
			return &Resource{Name: name, Object: pushed.Object, ETag: etag, Href: href, Dirty: false}
		}
		return &Resource{Name: name, Object: cur.Object, ETag: etag, Href: href, Dirty: true}
	})
}

// honorMidPushDeleteLocked records the server-side deletion of a resource that was
// deleted locally while its push was in flight: it ensures a tombstone carrying the
// post-PUT href/ETag (creating it for a never-synced create whose local delete left
// none, or advancing an existing one's ETag), then persists the sidecar. The caller
// must hold s.mu. A nil cs (the whole calendar was removed) means the collection-level
// DELETE handles server cleanup, so there is nothing to do.
func (s *Store) honorMidPushDeleteLocked(cs *calState, calID, name, href, etag string) error {
	if cs == nil || href == "" {
		return nil
	}
	if cs.tombstones == nil {
		cs.tombstones = map[string]tombstoneMeta{}
	}
	cs.tombstones[name] = tombstoneMeta{Href: href, ETag: etag}
	if err := writeSidecar(s.root, cs); err != nil {
		return fmt.Errorf("updating sidecar for %q: %w", calID, err)
	}
	return nil
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

// RemoteObject is one server resource to write via PullRemoteBatch.
type RemoteObject struct {
	Name   string
	Object *model.Parsed
	ETag   string
	Href   string
}

// ErrKeptLocalEdit is the per-resource result PullRemoteBatch reports when it
// skipped a pull because a concurrent local edit was pending on that name. It is
// not a failure: the local edit survives and the next sync reconciles it. Callers
// must treat it distinctly from a real per-resource error — neither counting it
// as pulled nor recording it as a skipped failure.
var ErrKeptLocalEdit = errors.New("store: kept a concurrent local edit; pull skipped")

// PullRemoteBatch writes many server objects into one calendar as clean
// resources — each .ics atomically, but the sidecar only once at the end. It
// turns a bulk pull's O(N) per-resource sidecar rewrites (which are O(N) each, so
// O(N²) overall) into a single O(N) write, the cost that dominated a first-time
// sync or import of a large calendar. results[i] is the error (or nil) for
// objs[i], so the caller can skip-and-continue per resource as the one-at-a-time
// path did; a non-nil error is a fatal per-calendar failure (context/sidecar).
//
// This is a pull-only phase (import; sync's "new on server" step — never for
// pushes). It holds s.mu for the whole batch, so a concurrent UI edit is fully
// serialized before or after it (never interleaved). But "serialized" is not
// "safe to overwrite": sync builds the pull list from a pre-lock snapshot, so an
// edit that lands after that snapshot but before the batch runs is invisible to
// the include-in-batch decision. A brand-new remote resource has no local
// counterpart, so its write can't lose anything; but a name that already exists
// locally and is Dirty carries a pending local edit (a race, or a crash-orphan
// the user just re-edited) — overwriting it would silently discard that edit. So
// each stage skips a currently-Dirty resource (reporting ErrKeptLocalEdit) and
// leaves it for the next sync; a clean local resource is still overwritten, which
// is the pass-5 crash-orphan self-heal (a clean, href-less .ics re-pulls). A
// crash mid-batch leaves .ics files whose sidecar entry wasn't flushed; sync
// re-pulls those cleanly, so a crash never yields a server-side duplicate.
func (s *Store) PullRemoteBatch(ctx context.Context, calID string, objs []RemoteObject) (results []error, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if calID == "" {
		return nil, errors.New("store: PullRemoteBatch requires a calendar id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	cs, err := s.ensureCalendar(calID)
	if err != nil {
		return nil, err
	}

	results = make([]error, len(objs))
	staged := false
	for i, o := range objs {
		if cerr := ctx.Err(); cerr != nil {
			return results, cerr
		}
		if o.Name == "" || o.Object == nil || o.Object.Calendar == nil {
			results[i] = errors.New("store: PullRemoteBatch requires a named, decoded object")
			continue
		}
		if cur := cs.resources[o.Name]; cur != nil && cur.Dirty {
			// A concurrent local edit is pending on this name (a race with the UI, or
			// a crash-orphan the user just re-edited). The write below is
			// unconditional, so applying it would silently overwrite that edit; keep
			// the edit and let the next sync reconcile it.
			results[i] = ErrKeptLocalEdit
			continue
		}
		o := o // capture for the closure
		if _, _, serr := s.stageResourceLocked(cs, calID, o.Name, func(*Resource) *Resource {
			return &Resource{Name: o.Name, Object: o.Object, ETag: o.ETag, Href: o.Href, Dirty: false}
		}); serr != nil {
			results[i] = serr
			continue
		}
		staged = true
	}

	if staged {
		if werr := writeSidecar(s.root, cs); werr != nil {
			// The staged .ics files are on disk but this sidecar write failed, so
			// their sync state (ETag/href) isn't recorded. They are harmless pull
			// orphans that the next sync re-pulls cleanly; surface the error.
			return results, fmt.Errorf("updating sidecar for %q: %w", calID, werr)
		}
	}
	return results, nil
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
