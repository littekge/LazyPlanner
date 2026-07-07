package store

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// Conflict is a resource whose local and server versions diverged (both edited
// between syncs). The local .ics remains the working copy; ServerData holds the
// server's diverging version so nothing is lost until the user resolves it.
type Conflict struct {
	CalID      string
	Name       string
	ServerETag string
	ServerData []byte
}

// MarkConflict records that calID/name conflicts with the server, stashing the
// server's diverging version losslessly. The local resource is left in place
// (and flagged Conflicted) so the user's edit is preserved; sync skips a
// conflicted resource until it is resolved. Re-marking updates the stash.
func (s *Store) MarkConflict(ctx context.Context, calID, name string, serverData []byte, serverETag string) error {
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

	if cs.conflicts == nil {
		cs.conflicts = map[string]conflictMeta{}
	}
	cs.conflicts[name] = conflictMeta{ServerETag: serverETag, ServerData: string(serverData)}

	// Replace the resource with a conflicted copy (copy-on-write: never mutate a
	// snapshot a reader may hold).
	nr := *r
	nr.Conflicted = true
	cs.resources[name] = &nr

	if err := writeSidecar(s.root, cs); err != nil {
		return fmt.Errorf("updating sidecar for %q: %w", calID, err)
	}
	return nil
}

// Conflicts returns every unresolved conflict across calendars, sorted. The UI
// uses the count for the sync-status indicator; interactive resolution arrives
// with command mode (build step 10).
func (s *Store) Conflicts() []Conflict {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Conflict
	for _, cs := range s.cals {
		for name, cm := range cs.conflicts {
			out = append(out, Conflict{
				CalID:      cs.id,
				Name:       name,
				ServerETag: cm.ServerETag,
				ServerData: []byte(cm.ServerData),
			})
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

// ResolveKeepLocal resolves a conflict in favor of the local copy: it clears the
// conflict and adopts the server's current ETag so the next sync's conditional
// PUT overwrites the server with the local version. The local .ics is unchanged.
func (s *Store) ResolveKeepLocal(ctx context.Context, calID, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cs := s.cals[calID]
	if cs == nil {
		return fmt.Errorf("store: unknown calendar %q", calID)
	}
	cm, ok := cs.conflicts[name]
	r := cs.resources[name]
	if !ok || r == nil {
		return fmt.Errorf("store: no conflict for %s/%s", calID, name)
	}
	nr := *r
	nr.ETag = cm.ServerETag // so the next push's If-Match matches the server
	nr.Dirty = true
	nr.Conflicted = false
	cs.resources[name] = &nr
	delete(cs.conflicts, name)
	if err := writeSidecar(s.root, cs); err != nil {
		return fmt.Errorf("updating sidecar for %q: %w", calID, err)
	}
	return nil
}

// ResolveKeepServer resolves a conflict in favor of the server: it overwrites the
// local copy with the stashed server version (written clean, with the server's
// ETag) and clears the conflict. The next sync sees the local copy already
// matching the server — a no-op.
func (s *Store) ResolveKeepServer(ctx context.Context, calID, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.RLock()
	cs := s.cals[calID]
	var (
		cm   conflictMeta
		href string
		ok   bool
	)
	if cs != nil {
		cm, ok = cs.conflicts[name]
		if r := cs.resources[name]; r != nil {
			href = r.Href
		}
	}
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("store: no conflict for %s/%s", calID, name)
	}

	parsed, err := model.Decode([]byte(cm.ServerData), time.Local)
	if err != nil {
		return fmt.Errorf("store: decoding server version of %s/%s: %w", calID, name, err)
	}
	// PutRemote writes clean and (via writeResource) clears the conflict stash.
	if _, err := s.PutRemote(ctx, calID, name, parsed, cm.ServerETag, href); err != nil {
		return err
	}
	return nil
}
