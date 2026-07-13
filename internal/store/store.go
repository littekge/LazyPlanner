package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

const (
	calendarsDir = "calendars"
	sidecarName  = ".lazyplanner.json"
	icsExt       = ".ics"
	dirPerm      = 0o700
	filePerm     = 0o600
)

// Resource is an immutable snapshot of one cached .ics resource: its parsed
// contents plus the sync state tracked in the calendar sidecar. Resources are
// replaced wholesale on write, never mutated in place, so a snapshot handed to
// a reader stays valid even as the store changes underneath it.
type Resource struct {
	Name       string        // file name within the calendar dir, e.g. "abc.ics"
	ETag       string        // server ETag from the last sync; "" if never pushed
	Href       string        // server resource path; "" until first sync
	Dirty      bool          // written locally, not yet pushed to the server
	Conflicted bool          // local and server diverged; awaiting resolution
	Object     *model.Parsed // parsed events/todos + the raw calendar for re-encode
}

// Calendar is an immutable snapshot of a cached collection: server-owned
// metadata plus the resources it contains.
type Calendar struct {
	ID          string // collection id = directory name (filesystem-safe)
	DisplayName string // server-owned display name; falls back to ID
	Color       string
	SyncToken   string
	Href        string
	Resources   []*Resource
	// PendingCreate is true for a locally-created calendar not yet pushed to the
	// server; Components is the iCalendar component set to create it with.
	PendingCreate bool
	Components    []string
	// ReadOnly is true when the server grants no write privilege (e.g. the
	// generated birthday calendar); the UI blocks writes and sync is pull-only.
	ReadOnly bool
}

// LoadError records a resource (or sidecar) that could not be read or parsed
// during Open. The rest of the cache still loads; callers surface these through
// LoadErrors rather than failing outright — the local data must never be held
// hostage by one corrupt file.
type LoadError struct {
	Calendar string
	Name     string
	Err      error
}

func (e LoadError) Error() string {
	if e.Name == "" {
		return fmt.Sprintf("calendar %q: %v", e.Calendar, e.Err)
	}
	return fmt.Sprintf("%s/%s: %v", e.Calendar, e.Name, e.Err)
}

func (e LoadError) Unwrap() error { return e.Err }

// calState is the internal mutable state for one calendar, guarded by Store.mu.
// Its resources map follows a copy-on-write discipline: entries are replaced,
// never mutated, so snapshots handed out to readers stay valid.
type calState struct {
	id          string
	displayName string
	color       string
	syncToken   string
	ctag        string // last-synced collection CTag (getctag); "" forces a full sync
	href        string
	resources   map[string]*Resource
	tombstones  map[string]tombstoneMeta // resource name -> pending server-side deletion
	conflicts   map[string]conflictMeta  // resource name -> stashed server version awaiting resolution

	pendingCreate bool     // created locally, awaiting MKCALENDAR on the next sync
	pendingDelete bool     // marked for deletion, awaiting server DELETE then local removal
	pendingName   bool     // display name edited locally, awaiting PROPPATCH
	pendingColor  bool     // color edited locally, awaiting PROPPATCH
	components    []string // iCalendar component set to create (VEVENT/VTODO)
	readOnly      bool     // server grants no write privilege; the app never writes here
}

// Store is the vdir cache: a set of calendar directories under a data root,
// each holding raw .ics resources (the local source of truth) plus a JSON
// sidecar of sync state. It is safe for concurrent use — background sync may
// mutate it while the UI reads.
type Store struct {
	root       string
	mu         sync.RWMutex
	cals       map[string]*calState
	loadErrors []LoadError
}

// Open loads the vdir cache under dataDir (its "calendars" subdirectory),
// building the in-memory index. A missing cache directory is not an error: it
// yields an empty store for a first run. Individual unreadable or unparseable
// resources are skipped and recorded in LoadErrors.
func Open(ctx context.Context, dataDir string) (*Store, error) {
	root := filepath.Join(dataDir, calendarsDir)
	s := &Store{root: root, cals: map[string]*calState{}}

	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		// Degrade rather than abort: a cache root that is a regular file or is
		// unreadable must not lock the user out of the whole app. Record it as a
		// LoadError (mirroring the per-calendar ReadDir tolerance in loadCalendar)
		// and return an empty store — the UI surfaces the error, and a later sync
		// can repopulate. An empty store has no tombstones, so this never deletes
		// server data.
		s.loadErrors = append(s.loadErrors, LoadError{Err: fmt.Errorf("reading cache root %q: %w", root, err)})
		return s, nil
	}

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !entry.IsDir() {
			continue
		}
		cs, errs := s.loadCalendar(ctx, entry.Name())
		if cs != nil {
			s.cals[cs.id] = cs
		}
		s.loadErrors = append(s.loadErrors, errs...)
	}
	return s, nil
}

func (s *Store) loadCalendar(ctx context.Context, id string) (*calState, []LoadError) {
	dir := filepath.Join(s.root, id)
	cs := &calState{id: id, resources: map[string]*Resource{}}
	var errs []LoadError

	sc, err := readSidecar(dir)
	if err != nil {
		errs = append(errs, LoadError{Calendar: id, Name: sidecarName, Err: err})
		sc = &sidecar{}
	}
	cs.displayName = sc.DisplayName
	cs.color = sc.Color
	cs.syncToken = sc.SyncToken
	cs.ctag = sc.CTag
	cs.href = sc.Href
	cs.tombstones = sc.Tombstones
	cs.conflicts = map[string]conflictMeta{}
	cs.pendingCreate = sc.PendingCreate
	cs.pendingDelete = sc.PendingDelete
	// Split name/color pending flags; a legacy pending_props (both) maps to both.
	cs.pendingName = sc.PendingName || sc.PendingProps
	cs.pendingColor = sc.PendingColor || sc.PendingProps
	cs.components = sc.Components
	cs.readOnly = sc.ReadOnly

	entries, err := os.ReadDir(dir)
	if err != nil {
		errs = append(errs, LoadError{Calendar: id, Err: err})
		return cs, errs
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return cs, errs
		}
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != icsExt {
			continue
		}
		res, err := loadResource(dir, name, sc)
		if err != nil {
			errs = append(errs, LoadError{Calendar: id, Name: name, Err: err})
			continue
		}
		cs.resources[name] = res
		if meta := sc.Resources[name]; meta.Conflict != nil {
			cs.conflicts[name] = *meta.Conflict
		}
	}
	return cs, errs
}

// loadResource reads and parses one .ics file, merging in the sync state the
// sidecar recorded for it (empty state if the file is untracked — the .ics
// files, not the sidecar, are the source of truth for what exists).
func loadResource(dir, name string, sc *sidecar) (*Resource, error) {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil, err
	}
	obj, err := model.Decode(data, time.Local)
	if err != nil {
		return nil, err
	}
	meta := sc.Resources[name]
	return &Resource{
		Name:       name,
		ETag:       meta.ETag,
		Href:       meta.Href,
		Dirty:      meta.Dirty,
		Conflicted: meta.Conflict != nil,
		Object:     obj,
	}, nil
}

// Calendars returns a snapshot of the cached calendars, sorted by id.
func (s *Store) Calendars() []Calendar {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Calendar, 0, len(s.cals))
	for _, cs := range s.cals {
		if cs.pendingDelete {
			continue // hidden from the UI and sync-reconcile until removed
		}
		out = append(out, cs.snapshot())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Calendar returns a snapshot of one calendar by id.
func (s *Store) Calendar(id string) (Calendar, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cs, ok := s.cals[id]
	if !ok {
		return Calendar{}, false
	}
	return cs.snapshot(), true
}

func (cs *calState) snapshot() Calendar {
	name := cs.displayName
	if name == "" {
		name = cs.id
	}
	res := make([]*Resource, 0, len(cs.resources))
	for _, r := range cs.resources {
		res = append(res, r)
	}
	sort.Slice(res, func(i, j int) bool { return res[i].Name < res[j].Name })
	return Calendar{
		ID:            cs.id,
		DisplayName:   name,
		Color:         cs.color,
		SyncToken:     cs.syncToken,
		Href:          cs.href,
		Resources:     res,
		PendingCreate: cs.pendingCreate,
		Components:    append([]string(nil), cs.components...),
		ReadOnly:      cs.readOnly,
	}
}

// Todos returns every cached todo across all calendars. Callers build the
// subtask tree and apply visibility/sort rules on top of this.
func (s *Store) Todos() []*model.Todo { return s.TodosVisible(nil) }

// TodosVisible is Todos restricted to calendars not present in hidden (keyed by
// calendar id). A nil/empty set returns every todo. The UI passes its set of
// locally-hidden calendars so the calendar/agenda views can omit them.
func (s *Store) TodosVisible(hidden map[string]bool) []*model.Todo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*model.Todo
	for calID, cs := range s.cals {
		if hidden[calID] {
			continue
		}
		for _, r := range cs.resources {
			out = append(out, r.Object.Todos...)
		}
	}
	return out
}

// EventOccurrences expands every cached event into concrete instances
// overlapping [from, to), across all calendars, sorted by start. This is the
// date-range query backing the calendar views.
func (s *Store) EventOccurrences(from, to time.Time) ([]model.Occurrence, error) {
	return s.EventOccurrencesVisible(from, to, nil)
}

// EventOccurrencesVisible is EventOccurrences restricted to calendars not present
// in hidden (keyed by calendar id). A nil/empty set includes every calendar.
func (s *Store) EventOccurrencesVisible(from, to time.Time, hidden map[string]bool) ([]model.Occurrence, error) {
	s.mu.RLock()
	objs := make([]*model.Parsed, 0, len(s.cals))
	for calID, cs := range s.cals {
		if hidden[calID] {
			continue
		}
		for _, r := range cs.resources {
			objs = append(objs, r.Object)
		}
	}
	s.mu.RUnlock()

	var out []model.Occurrence
	for _, obj := range objs {
		// Skip an object whose expansion fails rather than blanking events from
		// every calendar (iron rule: one malformed resource must degrade
		// gracefully). The model already degrades a bad recurrence rule to its
		// base instance, so this is defense-in-depth for any future error source.
		occs, err := obj.EventOccurrences(from, to)
		if err != nil {
			continue
		}
		out = append(out, occs...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out, nil
}

// LoadErrors returns a copy of the problems encountered during Open.
func (s *Store) LoadErrors() []LoadError {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]LoadError(nil), s.loadErrors...)
}
