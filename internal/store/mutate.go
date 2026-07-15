package store

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
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

// PutIfUnchanged writes obj to calID/name only if the current cached resource is
// still expectedPrev (pointer identity) — the write guard PullRemote applies to a
// pull, but for a local edit. A read-modify-write built from a snapshot (grab's
// Locate→edit→Put) can otherwise clobber a background sync pull that landed in the
// window: the write's build(prev) reads the freshly-pulled resource and adopts
// its ETag while persisting the stale-derived content, so the next push's ETag
// CAS matches the server and overwrites the remote edit. When the resource
// changed underneath, the write is skipped (applied=false) so the caller can
// abort rather than clobber. A nil expectedPrev is an unconditional Put.
func (s *Store) PutIfUnchanged(ctx context.Context, calID, name string, obj *model.Parsed, expectedPrev *Resource) (applied bool, err error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if calID == "" || name == "" {
		return false, errors.New("store: write requires a calendar id and resource name")
	}
	if obj == nil || obj.Calendar == nil {
		return false, errors.New("store: write requires a decoded object")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if expectedPrev != nil {
		var cur *Resource
		if cs := s.cals[calID]; cs != nil {
			cur = cs.resources[name]
		}
		if cur != expectedPrev {
			return false, nil // a concurrent change (e.g. a sync pull) landed
		}
	}
	if _, err := s.writeResourceLocked(calID, name, func(prev *Resource) *Resource {
		res := &Resource{Name: name, Object: obj, Dirty: true}
		if prev != nil {
			res.ETag = prev.ETag
			res.Href = prev.Href
		}
		return res
	}); err != nil {
		return false, err
	}
	return true, nil
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
// the current resource can choose which content to keep under the lock. The .ics
// and sidecar are two separate atomic renames; a crash in the window between them
// is caught on reload by the sidecar's per-resource content hash (a mismatch
// reloads the resource Dirty), so a stranded edit is never silently seen as synced.
func (s *Store) writeResourceLocked(calID, name string, build func(prev *Resource) *Resource) (*Resource, error) {
	cs, err := s.ensureCalendar(calID)
	if err != nil {
		return nil, err
	}
	res, revert, err := s.stageResourceLocked(cs, calID, name, build)
	if err != nil {
		return nil, err
	}
	if err := writeSidecar(s.root, cs); err != nil {
		// The sidecar didn't persist — revert the .ics and in-memory state so the
		// two on-disk files can't diverge (a stale sidecar could otherwise strand
		// this edit or resurrect a deleted item after a restart).
		if rerr := revert(); rerr != nil {
			return nil, fmt.Errorf("updating sidecar for %q failed and the on-disk revert also failed — cache may be inconsistent until the next successful sync: %w",
				calID, errors.Join(err, rerr))
		}
		return nil, fmt.Errorf("updating sidecar for %q: %w", calID, err)
	}
	return res, nil
}

// stageResourceLocked encodes and atomically writes a resource's .ics and applies
// the in-memory change, but does NOT persist the sidecar — the caller does that.
// It returns a revert closure that undoes the in-memory change (used when a
// following sidecar write fails). Splitting the sidecar write out lets a bulk
// pull (PullRemoteBatch) write many .ics files under one sidecar write instead of
// re-serializing the whole calendar per resource. The caller must hold s.mu.
func (s *Store) stageResourceLocked(cs *calState, calID, name string, build func(prev *Resource) *Resource) (*Resource, func() error, error) {
	prevRes := cs.resources[name]
	prevConf, hadConf := cs.conflicts[name]
	prevTomb, hadTomb := cs.tombstones[name]

	res := build(prevRes)
	if res == nil || res.Object == nil || res.Object.Calendar == nil {
		return nil, nil, errors.New("store: write requires a decoded object")
	}
	data, err := res.Object.Encode()
	if err != nil {
		return nil, nil, fmt.Errorf("encoding %s/%s: %w", calID, name, err)
	}
	// Record the fingerprint of exactly the bytes we write, so a reload can detect
	// an .ics that was rewritten after the sidecar (a crash between the renames).
	res.hash = contentHash(data)

	if err := writeFileAtomic(filepath.Join(s.root, calID, name), data, filePerm); err != nil {
		return nil, nil, fmt.Errorf("writing %s/%s: %w", calID, name, err)
	}

	cs.resources[name] = res
	// Writing a resource cancels any pending deletion of the same name — this is
	// how undo (Restore) resurrects a just-deleted resource — and supersedes any
	// stashed conflict (a deliberate write resolves it).
	delete(cs.tombstones, name)
	delete(cs.conflicts, name)

	revert := func() error { return s.revertMutation(calID, name, prevRes, prevConf, hadConf, prevTomb, hadTomb) }
	return res, revert, nil
}

// ensureCalendar returns the calendar state for calID, creating the directory
// and index entry on first use. Callers must hold s.mu.
func (s *Store) ensureCalendar(calID string) (*calState, error) {
	if !validCalendarID(calID) {
		return nil, fmt.Errorf("store: unsafe calendar id %q", calID)
	}
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
// its sync state (ETag/Href/Dirty). Used by the multi-write rollback paths
// (a failed split/detach/grab), where the server was never touched so the clean
// snapshot is still accurate. For the session undo stack, use RestoreDirty.
func (s *Store) Restore(ctx context.Context, calID, name string, res *Resource) (*Resource, error) {
	if res == nil || res.Object == nil {
		return nil, errors.New("store: restore requires a resource snapshot")
	}
	return s.writeResource(ctx, calID, name, res.Object, func(*Resource) *Resource {
		return &Resource{Name: name, ETag: res.ETag, Href: res.Href, Dirty: res.Dirty, Object: res.Object}
	})
}

// RestoreDirty writes a prior snapshot back like Restore but marks it Dirty
// (keeping the snapshot's Href/ETag as the sync baseline), so the next sync treats
// the resurrection/revert as a pending local change. This is the session-undo
// path: undoing an edit or delete that ALREADY synced must not replay clean — a
// clean, stale snapshot is pulled back over on the next reconcile (an undone edit
// whose ETag is older than the server's) or Forgotten as a server-absent orphan
// (an undone delete whose tombstone already pushed), silently losing the undo.
// Marking it Dirty makes sync push the revert or raise a keep-both conflict
// instead of dropping it — consistent with the never-silently-overwrite model.
func (s *Store) RestoreDirty(ctx context.Context, calID, name string, res *Resource) (*Resource, error) {
	if res == nil || res.Object == nil {
		return nil, errors.New("store: restore requires a resource snapshot")
	}
	return s.writeResource(ctx, calID, name, res.Object, func(*Resource) *Resource {
		return &Resource{Name: name, ETag: res.ETag, Href: res.Href, Dirty: true, Object: res.Object}
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
		if rerr := s.revertMutation(calID, name, r, prevConf, hadConf, prevTomb, hadTomb); rerr != nil {
			return fmt.Errorf("updating sidecar for %q failed and the on-disk revert also failed — cache may be inconsistent until the next successful sync: %w",
				calID, errors.Join(err, rerr))
		}
		return fmt.Errorf("updating sidecar for %q: %w", calID, err)
	}
	return nil
}

// revertMutation restores a resource's .ics and in-memory state to their pre-write
// values after a sidecar write failed, so the .ics and the sidecar never diverge.
// prevRes is the resource before the write (nil if it was a create — then the
// .ics is removed); prevConf/prevTomb (with their had* flags) are the conflict and
// tombstone entries to put back. It returns a non-nil error when the on-disk
// restore itself fails (a failing disk — e.g. ENOSPC — that also broke the sidecar
// write): the caller must surface that rather than swallow it, because the on-disk
// .ics may then hold the failed edit while the sidecar still describes the prior
// state. The in-memory restore always succeeds.
func (s *Store) revertMutation(calID, name string, prevRes *Resource, prevConf conflictMeta, hadConf bool, prevTomb tombstoneMeta, hadTomb bool) error {
	cs := s.cals[calID]
	if cs == nil {
		return nil
	}
	path := filepath.Join(s.root, calID, name)
	var revertErr error
	if prevRes != nil {
		cs.resources[name] = prevRes
		if data, err := prevRes.Object.Encode(); err != nil {
			revertErr = fmt.Errorf("re-encoding prior %s/%s: %w", calID, name, err)
		} else if err := writeFileAtomic(path, data, filePerm); err != nil {
			revertErr = fmt.Errorf("restoring %s/%s: %w", calID, name, err)
		}
	} else {
		delete(cs.resources, name)
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			revertErr = fmt.Errorf("removing staged %s/%s: %w", calID, name, err)
		}
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
	return revertErr
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
	out := b.String()
	// "." and ".." are the two dot-only names the filesystem treats specially, so
	// a name that sanitizes to either would be a path-traversal segment (e.g. a
	// calendar id of ".." joined to the cache root escapes it). Neutralize them —
	// legitimate names never sanitize to a bare "." or "..".
	if out == "." || out == ".." {
		return "unnamed"
	}
	// Cap the length so an over-long UID/href (from another client) still yields a
	// writable file name under the filesystem's per-name limit; a deterministic hash
	// suffix keeps distinct long inputs distinct and stable across runs. The ".ics"
	// suffix a resource name later appends still fits under the common 255 limit.
	if len(out) > maxSafeNameLen {
		h := fnv.New64a()
		_, _ = h.Write([]byte(s))
		out = out[:maxSafeNameLen] + "-" + fmt.Sprintf("%016x", h.Sum64())
	}
	return out
}

// maxSafeNameLen caps the sanitized-name prefix; with the 17-char hash suffix and
// a later ".ics" it stays well under the common 255-byte filesystem NAME_MAX.
const maxSafeNameLen = 200

// validCalendarID reports whether id is safe to join onto the cache root as a
// calendar directory: non-empty, not a dot-traversal segment, and free of any
// path separator or NUL. SafeName already produces such ids; this is a
// defense-in-depth guard on the mutation paths (above all the RemoveAll one), so
// a raw id can never escape the calendars root regardless of how it was derived.
func validCalendarID(id string) bool {
	if id == "" || id == "." || id == ".." {
		return false
	}
	return !strings.ContainsAny(id, "/\\\x00")
}

// contentHash is a fast non-cryptographic fingerprint of a resource's on-disk
// bytes, recorded in the sidecar so a reload can detect an .ics rewritten after
// the sidecar (a crash between the two atomic renames) and reload it dirty.
func contentHash(data []byte) string {
	h := fnv.New64a()
	_, _ = h.Write(data)
	return fmt.Sprintf("%016x", h.Sum64())
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
