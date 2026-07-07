package store

import (
	"context"
	"fmt"
	"sort"
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
