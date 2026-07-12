package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/littekge/LazyPlanner/internal/model"
)

// Put writes obj to calID/name atomically and updates the in-memory index and
// sidecar. The .ics file is the source of truth, so it is written first
// (write-temp-then-rename); the resource is marked Dirty (a local change not
// yet pushed). Overwriting a resource preserves its server identity (ETag,
// Href) so the next sync can detect the local edit. A calendar directory is
// created on first write.
//
// name is the target file name. For a new resource, derive it with
// ResourceName; when rewriting a resource loaded from disk or the server, reuse
// its existing Name so the file maps back to the same server resource.
func (s *Store) Put(ctx context.Context, calID, name string, obj *model.Parsed) (*Resource, error) {
	return s.writeResource(ctx, calID, name, obj, func(prev *Resource) *Resource {
		res := &Resource{Name: name, Object: obj, Dirty: true}
		if prev != nil {
			// Keep the server identity so the next sync sees a local edit.
			res.ETag = prev.ETag
			res.Href = prev.Href
		}
		return res
	})
}

// writeResource is the shared write path for Put and PutRemote: encode, write
// the .ics atomically, then update the index and sidecar. build produces the
// new resource snapshot from the previous one (nil if new), letting callers set
// dirty/ETag/Href appropriately.
func (s *Store) writeResource(ctx context.Context, calID, name string, obj *model.Parsed, build func(prev *Resource) *Resource) (*Resource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if calID == "" || name == "" {
		return nil, errors.New("store: write requires a calendar id and resource name")
	}
	if obj == nil || obj.Calendar == nil {
		return nil, errors.New("store: write requires a decoded object")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeResourceLocked(calID, name, build)
}

// writeResourceLocked is the write core shared by writeResource and the
// compare-and-set sync writers (CommitPush/PullRemote). The caller must hold
// s.mu. build derives the new resource from the current one (nil if none); the
// object persisted to disk is build's returned Object, so a build that inspects
// the current resource can choose which content to keep under the lock.
func (s *Store) writeResourceLocked(calID, name string, build func(prev *Resource) *Resource) (*Resource, error) {
	cs, err := s.ensureCalendar(calID)
	if err != nil {
		return nil, err
	}

	prevRes := cs.resources[name]
	prevConf, hadConf := cs.conflicts[name]
	prevTomb, hadTomb := cs.tombstones[name]

	res := build(prevRes)
	if res == nil || res.Object == nil || res.Object.Calendar == nil {
		return nil, errors.New("store: write requires a decoded object")
	}
	data, err := res.Object.Encode()
	if err != nil {
		return nil, fmt.Errorf("encoding %s/%s: %w", calID, name, err)
	}

	if err := writeFileAtomic(filepath.Join(s.root, calID, name), data, filePerm); err != nil {
		return nil, fmt.Errorf("writing %s/%s: %w", calID, name, err)
	}

	cs.resources[name] = res
	// Writing a resource cancels any pending deletion of the same name — this is
	// how undo (Restore) resurrects a just-deleted resource — and supersedes any
	// stashed conflict (a deliberate write resolves it).
	delete(cs.tombstones, name)
	delete(cs.conflicts, name)

	if err := writeSidecar(s.root, cs); err != nil {
		// The sidecar didn't persist — revert the .ics and in-memory state so the
		// two on-disk files can't diverge (a stale sidecar could otherwise strand
		// this edit or resurrect a deleted item after a restart).
		s.revertMutation(calID, name, prevRes, prevConf, hadConf, prevTomb, hadTomb)
		return nil, fmt.Errorf("updating sidecar for %q: %w", calID, err)
	}
	return res, nil
}

// ensureCalendar returns the calendar state for calID, creating the directory
// and index entry on first use. Callers must hold s.mu.
func (s *Store) ensureCalendar(calID string) (*calState, error) {
	cs := s.cals[calID]
	if cs == nil {
		if err := os.MkdirAll(filepath.Join(s.root, calID), dirPerm); err != nil {
			return nil, fmt.Errorf("creating calendar %q: %w", calID, err)
		}
		cs = &calState{id: calID, resources: map[string]*Resource{}}
		s.cals[calID] = cs
	}
	return cs, nil
}

// Located identifies the cached resource that holds a given event or todo, so
// the UI can edit or delete it by UID without tracking file names itself. Object
// is the resource's parsed contents (edit a clone of it, then Put); Prev is the
// current immutable snapshot, suitable for stashing on an undo stack.
type Located struct {
	CalID  string
	Name   string
	Object *model.Parsed
	Prev   *Resource
}

// Locate finds the resource containing the event or todo with the given UID.
func (s *Store) Locate(uid string) (Located, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, cs := range s.cals {
		for name, r := range cs.resources {
			for _, ev := range r.Object.Events {
				if ev.UID == uid {
					return Located{CalID: cs.id, Name: name, Object: r.Object, Prev: r}, true
				}
			}
			for _, td := range r.Object.Todos {
				if td.UID == uid {
					return Located{CalID: cs.id, Name: name, Object: r.Object, Prev: r}, true
				}
			}
		}
	}
	return Located{}, false
}

// Restore writes a prior resource snapshot back to calID/name exactly, keeping
// its sync state (ETag/Href/Dirty). It is the inverse of an edit or delete for
// the session undo stack: pair it with a snapshot captured before the change.
func (s *Store) Restore(ctx context.Context, calID, name string, res *Resource) (*Resource, error) {
	if res == nil || res.Object == nil {
		return nil, errors.New("store: restore requires a resource snapshot")
	}
	return s.writeResource(ctx, calID, name, res.Object, func(*Resource) *Resource {
		return &Resource{Name: name, ETag: res.ETag, Href: res.Href, Dirty: res.Dirty, Object: res.Object}
	})
}

// Delete removes calID/name from disk and the in-memory index. This is a local
// deletion; propagating it to the server is the sync layer's job. If the
// resource had a server identity, a tombstone is left so the next sync deletes
// it there too.
func (s *Store) Delete(ctx context.Context, calID, name string) error {
	return s.remove(ctx, calID, name, true)
}

// Forget removes calID/name locally without leaving a tombstone. The sync layer
// uses it when the server no longer has the resource (deleted remotely), so the
// local copy is dropped without a pointless server DELETE.
func (s *Store) Forget(ctx context.Context, calID, name string) error {
	return s.remove(ctx, calID, name, false)
}

func (s *Store) remove(ctx context.Context, calID, name string, tombstone bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cs := s.cals[calID]
	if cs == nil {
		return fmt.Errorf("store: unknown calendar %q", calID)
	}
	r, ok := cs.resources[name]
	if !ok {
		return fmt.Errorf("store: unknown resource %s/%s", calID, name)
	}

	prevConf, hadConf := cs.conflicts[name]
	prevTomb, hadTomb := cs.tombstones[name]

	if err := os.Remove(filepath.Join(s.root, calID, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("deleting %s/%s: %w", calID, name, err)
	}
	delete(cs.resources, name)
	// A removed resource carries no unresolved conflict with it.
	delete(cs.conflicts, name)

	// If the resource had a server identity, remember to delete it there on the
	// next sync. A never-synced local resource (no Href) leaves no tombstone.
	if tombstone && r.Href != "" {
		if cs.tombstones == nil {
			cs.tombstones = map[string]tombstoneMeta{}
		}
		cs.tombstones[name] = tombstoneMeta{Href: r.Href, ETag: r.ETag}
	}

	if err := writeSidecar(s.root, cs); err != nil {
		// Revert the removal (restore the .ics + in-memory state) so a failed
		// sidecar write can't leave a lost tombstone that resurrects the item.
		s.revertMutation(calID, name, r, prevConf, hadConf, prevTomb, hadTomb)
		return fmt.Errorf("updating sidecar for %q: %w", calID, err)
	}
	return nil
}

// revertMutation restores a resource's .ics and in-memory state to their pre-write
// values after a sidecar write failed, so the .ics and the sidecar never diverge.
// prevRes is the resource before the write (nil if it was a create — then the
// .ics is removed); prevConf/prevTomb (with their had* flags) are the conflict and
// tombstone entries to put back.
func (s *Store) revertMutation(calID, name string, prevRes *Resource, prevConf conflictMeta, hadConf bool, prevTomb tombstoneMeta, hadTomb bool) {
	cs := s.cals[calID]
	if cs == nil {
		return
	}
	path := filepath.Join(s.root, calID, name)
	if prevRes != nil {
		cs.resources[name] = prevRes
		if data, err := prevRes.Object.Encode(); err == nil {
			_ = writeFileAtomic(path, data, filePerm)
		}
	} else {
		delete(cs.resources, name)
		_ = os.Remove(path)
	}
	if hadConf {
		cs.conflicts[name] = prevConf
	} else {
		delete(cs.conflicts, name)
	}
	if hadTomb {
		if cs.tombstones == nil {
			cs.tombstones = map[string]tombstoneMeta{}
		}
		cs.tombstones[name] = prevTomb
	} else {
		delete(cs.tombstones, name)
	}
}

// ResourceName returns the .ics file name for a new resource with the given
// UID, sanitized to be filesystem-safe. When rewriting a resource that already
// exists on disk or the server, reuse its existing Name instead so it maps back
// to the same file/server resource.
func ResourceName(uid string) string {
	return SafeName(uid) + icsExt
}

// SafeName maps an arbitrary string (a UID or a server path segment) to a safe
// file base name, replacing any character outside [A-Za-z0-9._-] with '_'.
// Distinct inputs can in principle collide after sanitizing; that is acceptable
// at personal-calendar scale, and the server identity is tracked separately via
// the resource Href.
func SafeName(s string) string {
	if s == "" {
		return "unnamed"
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// writeFileAtomic writes data to a temp file in the destination directory,
// fsyncs it, then renames it over path — so a reader (or a crash) never sees a
// half-written file. The directory is fsynced afterward so the rename is
// durable. This is how offline edits are committed safely to the cache.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false

	// Best-effort: fsync the directory so the rename survives a crash.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
