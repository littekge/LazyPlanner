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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if calID == "" || name == "" {
		return nil, errors.New("store: Put requires a calendar id and resource name")
	}
	if obj == nil || obj.Calendar == nil {
		return nil, errors.New("store: Put requires a decoded object")
	}

	data, err := obj.Encode()
	if err != nil {
		return nil, fmt.Errorf("encoding %s/%s: %w", calID, name, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cs := s.cals[calID]
	if cs == nil {
		if err := os.MkdirAll(filepath.Join(s.root, calID), dirPerm); err != nil {
			return nil, fmt.Errorf("creating calendar %q: %w", calID, err)
		}
		cs = &calState{id: calID, resources: map[string]*Resource{}}
		s.cals[calID] = cs
	}

	if err := writeFileAtomic(filepath.Join(s.root, calID, name), data, filePerm); err != nil {
		return nil, fmt.Errorf("writing %s/%s: %w", calID, name, err)
	}

	res := &Resource{Name: name, Object: obj, Dirty: true}
	if prev := cs.resources[name]; prev != nil {
		res.ETag = prev.ETag
		res.Href = prev.Href
	}
	cs.resources[name] = res

	if err := writeSidecar(s.root, cs); err != nil {
		return nil, fmt.Errorf("updating sidecar for %q: %w", calID, err)
	}
	return res, nil
}

// Delete removes calID/name from disk and the in-memory index. This is a local
// deletion; propagating it to the server is the sync layer's job.
func (s *Store) Delete(ctx context.Context, calID, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cs := s.cals[calID]
	if cs == nil {
		return fmt.Errorf("store: unknown calendar %q", calID)
	}
	if _, ok := cs.resources[name]; !ok {
		return fmt.Errorf("store: unknown resource %s/%s", calID, name)
	}

	if err := os.Remove(filepath.Join(s.root, calID, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("deleting %s/%s: %w", calID, name, err)
	}
	delete(cs.resources, name)

	if err := writeSidecar(s.root, cs); err != nil {
		return fmt.Errorf("updating sidecar for %q: %w", calID, err)
	}
	return nil
}

// ResourceName returns the .ics file name for a new resource with the given
// UID, sanitized to be filesystem-safe. When rewriting a resource that already
// exists on disk or the server, reuse its existing Name instead so it maps back
// to the same file/server resource.
func ResourceName(uid string) string {
	return sanitize(uid) + icsExt
}

// sanitize maps a UID to a safe file base name, replacing any character outside
// [A-Za-z0-9._-] with '_'. Distinct UIDs can in principle collide after
// sanitizing; that is acceptable at personal-calendar scale.
func sanitize(uid string) string {
	if uid == "" {
		return "unnamed"
	}
	var b strings.Builder
	b.Grow(len(uid))
	for _, r := range uid {
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
