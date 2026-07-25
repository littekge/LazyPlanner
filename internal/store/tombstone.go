package store

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/littekge/LazyPlanner/internal/model"
)

// Tombstone is a resource deleted locally that still needs to be deleted on the
// server. The sync layer pushes it as a conditional DELETE (If-Match: ETag) so a
// concurrent remote edit is not silently discarded.
type Tombstone struct {
	CalID string
	Name  string
	Href  string
	ETag  string
}

// Tombstones returns all pending server-side deletions across calendars, sorted
// for a deterministic push order.
func (s *Store) Tombstones() []Tombstone {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Tombstone
	for _, cs := range s.cals {
		for name, tm := range cs.tombstones {
			out = append(out, Tombstone{CalID: cs.id, Name: name, Href: tm.Href, ETag: tm.ETag})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CalID != out[j].CalID {
			return out[i].CalID < out[j].CalID
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// ResurrectTombstone writes the server's version of a resource whose local
// delete lost a delete-vs-server-change race (a conditional DELETE returned
// 412), but only if the tombstone is still exactly what the sync read as
// expected — i.e. nothing local touched this name since. Sync runs on a
// background goroutine while the UI keeps editing on the event loop, so
// between reading the tombstone and this write a concurrent local change
// (most notably an undo re-creating the resource via RestoreDirty, which
// clears the tombstone as part of its write) may have landed. Since the
// write is otherwise unconditional, applying it unguarded would silently
// clobber that re-create — the DELETE-conflict twin of the CommitPush
// resource-gone race PullRemote/PutIfUnchanged already guard for edits and
// pulls. When the tombstone changed underneath (cleared or now pointing at a
// different Href/ETag), the write is skipped (applied=false) so the caller
// can flag the server version as a conflict against the surviving local
// resource instead of overwriting it.
func (s *Store) ResurrectTombstone(ctx context.Context, calID, name string, obj *model.Parsed, etag, href string, expected Tombstone) (applied bool, err error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if calID == "" || name == "" {
		return false, errors.New("store: ResurrectTombstone requires a calendar id and resource name")
	}
	if obj == nil || obj.Calendar == nil {
		return false, errors.New("store: ResurrectTombstone requires a decoded object")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var cur tombstoneMeta
	var ok bool
	if cs := s.cals[calID]; cs != nil {
		cur, ok = cs.tombstones[name]
	}
	if !ok || cur.Href != expected.Href || cur.ETag != expected.ETag {
		return false, nil // a concurrent local change (e.g. an undo) landed; don't overwrite it
	}

	if _, err := s.writeResourceLocked(calID, name, func(*Resource) *Resource {
		return &Resource{Name: name, Object: obj, ETag: etag, Href: href, Dirty: false}
	}); err != nil {
		return false, err
	}
	return true, nil
}

// ClearTombstone drops a pending deletion after sync has pushed it to the
// server. It is a no-op if the calendar or tombstone is already gone.
func (s *Store) ClearTombstone(ctx context.Context, calID, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cs := s.cals[calID]
	if cs == nil {
		return nil
	}
	if _, ok := cs.tombstones[name]; !ok {
		return nil
	}
	delete(cs.tombstones, name)
	if err := writeSidecar(s.root, cs); err != nil {
		return fmt.Errorf("updating sidecar for %q: %w", calID, err)
	}
	return nil
}
