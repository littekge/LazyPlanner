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
	CalID         string
	Name          string
	ServerETag    string
	ServerData    []byte
	ServerDeleted bool // the server deleted the resource (accept-server = drop the local copy)
}

// MarkConflict records that calID/name conflicts with the server, stashing the
// server's diverging version losslessly. The local resource is left in place
// (and flagged Conflicted) so the user's edit is preserved; sync skips a
// conflicted resource until it is resolved. Re-marking updates the stash.
func (s *Store) MarkConflict(ctx context.Context, calID, name string, serverData []byte, serverETag string, serverDeleted bool) error {
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
	cs.conflicts[name] = conflictMeta{ServerETag: serverETag, ServerData: string(serverData), ServerDeleted: serverDeleted}

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
				CalID:         cs.id,
				Name:          name,
				ServerETag:    cm.ServerETag,
				ServerData:    []byte(cm.ServerData),
				ServerDeleted: cm.ServerDeleted,
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
// PUT overwrites the server with the local version. When the conflict is a
// server *deletion* (no server copy remains), it instead clears the Href so the
// next sync re-creates the item on the server via the create path — keeping the
// local edit and resurrecting it, rather than re-raising the same conflict every
// sync. The local .ics is unchanged.
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
	if cm.ServerDeleted {
		// The server has no copy left to conditionally overwrite, so keep-local
		// means re-create it. Clear the Href so the next reconcile takes the
		// create path (Href=="" && Dirty) and pushes it as a new resource;
		// otherwise it lands in the !onServer && Dirty branch and re-raises the
		// identical server-deleted conflict every sync, never converging and
		// never resurrecting the item on the server.
		nr.Href = ""
	}
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

	// The server DELETED the resource while it was edited locally. "Keep server"
	// = accept the deletion: drop the local copy (no tombstone — it's already gone
	// on the server) and clear the conflict. This is keyed on the explicit
	// ServerDeleted flag, NOT on empty ServerData: a present-but-unparseable
	// server version also lacks a typed model, and treating it as a deletion would
	// silently discard the local edit.
	if cm.ServerDeleted {
		return s.Forget(ctx, calID, name)
	}
	if cm.ServerData == "" {
		// Not a deletion, yet nothing was stashed — the server version couldn't be
		// captured. Refuse rather than dropping the local edit; the conflict stays
		// for the user to resolve (e.g. keep-local).
		return fmt.Errorf("store: server version of %s/%s is unavailable; cannot keep server", calID, name)
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
