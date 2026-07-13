package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// maxLocalFileBytes bounds a single cache-file read (sidecar or .ics) so a corrupt
// or hostile file — including a symlink to an endless device — can't exhaust memory
// or hang. It mirrors the 64 MiB network-body cap; real files are far smaller.
const maxLocalFileBytes = 64 << 20

// sidecar is the on-disk JSON companion to a calendar directory. It caches
// server-owned metadata and per-resource sync state (ETags, hrefs, the sync
// token). It is derived data: if it is lost or corrupt the .ics files still
// stand as the source of truth, and a fresh sync rebuilds it.
type sidecar struct {
	DisplayName string                  `json:"display_name,omitempty"`
	Color       string                  `json:"color,omitempty"`
	SyncToken   string                  `json:"sync_token,omitempty"`
	CTag        string                  `json:"ctag,omitempty"`
	Href        string                  `json:"href,omitempty"`
	Resources   map[string]resourceMeta `json:"resources,omitempty"`
	// Calendar-level pending state for offline-first in-app management: a
	// locally-created calendar awaits MKCALENDAR on the next sync; one marked
	// for deletion awaits a server DELETE then local removal. Components is the
	// iCalendar component set (VEVENT/VTODO) to create.
	PendingCreate bool `json:"pending_create,omitempty"`
	PendingDelete bool `json:"pending_delete,omitempty"`
	// PendingName/PendingColor track a locally-edited display name / color awaiting
	// a PROPPATCH, independently so a pending name doesn't block a color pull (and
	// vice-versa). PendingProps is the legacy single flag, read for backward
	// compatibility and mapped onto both.
	PendingName  bool     `json:"pending_name,omitempty"`
	PendingColor bool     `json:"pending_color,omitempty"`
	PendingProps bool     `json:"pending_props,omitempty"`
	Components   []string `json:"components,omitempty"`
	// ReadOnly caches the server's read-only status (no write privilege) so the
	// UI knows not to allow writes even before the first sync of a session.
	ReadOnly bool `json:"read_only,omitempty"`
	// Tombstones records resources deleted locally that still need to be deleted
	// on the server, keyed by their (now-gone) .ics file name. They are kept
	// until sync pushes the deletion, then cleared.
	Tombstones map[string]tombstoneMeta `json:"tombstones,omitempty"`
}

type resourceMeta struct {
	ETag  string `json:"etag,omitempty"`
	Href  string `json:"href,omitempty"`
	Dirty bool   `json:"dirty,omitempty"`
	// Hash fingerprints the .ics bytes as of this sidecar write. On reload a
	// mismatch means the .ics was rewritten after the sidecar (a crash in the
	// window between the two atomic renames), so the resource is really an unsynced
	// local edit and must load Dirty — otherwise the edit looks clean and never
	// syncs. Empty on a legacy sidecar or an untracked resource (then not enforced).
	Hash string `json:"hash,omitempty"`
	// Conflict, when set, means the local resource and the server diverged (both
	// edited between syncs). The local .ics stays as the working copy; the
	// server's diverging version is stashed here losslessly until the user
	// resolves the conflict.
	Conflict *conflictMeta `json:"conflict,omitempty"`
}

// conflictMeta stashes the server's diverging version of a resource so nothing
// is lost while a conflict awaits resolution.
type conflictMeta struct {
	ServerETag string `json:"server_etag,omitempty"`
	ServerData string `json:"server_data,omitempty"` // raw iCalendar of the server's version
	// ServerDeleted marks a conflict where the server DELETED the resource while
	// it was edited locally (ServerData is then empty). It disambiguates that case
	// from a present-but-unparseable server version, which also stashes without a
	// typed model but must NOT be treated as a deletion (that would silently
	// discard the local edit on "keep server").
	ServerDeleted bool `json:"server_deleted,omitempty"`
}

// tombstoneMeta is the server identity of a locally-deleted resource, enough to
// issue a conditional DELETE (If-Match: ETag) on the next sync.
type tombstoneMeta struct {
	Href string `json:"href,omitempty"`
	ETag string `json:"etag,omitempty"`
}

// readSidecar loads a calendar's sidecar. A missing sidecar is normal (a vdir
// populated by another tool, or a first run) and yields an empty one.
func readSidecar(dir string) (*sidecar, error) {
	f, err := os.Open(filepath.Join(dir, sidecarName))
	if errors.Is(err, os.ErrNotExist) {
		return &sidecar{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxLocalFileBytes))
	if err != nil {
		return nil, err
	}
	var sc sidecar
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("parsing sidecar: %w", err)
	}
	return &sc, nil
}

// writeSidecar persists a calendar's current state to its sidecar file,
// atomically.
func writeSidecar(root string, cs *calState) error {
	sc := sidecar{
		DisplayName:   cs.displayName,
		Color:         cs.color,
		SyncToken:     cs.syncToken,
		CTag:          cs.ctag,
		Href:          cs.href,
		Resources:     make(map[string]resourceMeta, len(cs.resources)),
		PendingCreate: cs.pendingCreate,
		PendingDelete: cs.pendingDelete,
		PendingName:   cs.pendingName,
		PendingColor:  cs.pendingColor,
		Components:    cs.components,
		ReadOnly:      cs.readOnly,
	}
	for name, r := range cs.resources {
		m := resourceMeta{ETag: r.ETag, Href: r.Href, Dirty: r.Dirty, Hash: r.hash}
		if cm, ok := cs.conflicts[name]; ok {
			c := cm
			m.Conflict = &c
		}
		sc.Resources[name] = m
	}
	if len(cs.tombstones) > 0 {
		sc.Tombstones = make(map[string]tombstoneMeta, len(cs.tombstones))
		for name, tm := range cs.tombstones {
			sc.Tombstones[name] = tm
		}
	}
	data, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(root, cs.id, sidecarName), data, filePerm)
}
