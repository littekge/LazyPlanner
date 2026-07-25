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

	// Salvage bookkeeping for a sidecar that failed a strict decode. Unexported,
	// so it never round-trips to disk: it describes this load, not the file.
	//
	//   salvaged        - the file existed but did not parse cleanly, so every
	//                     field NOT recovered below is UNKNOWN, not empty.
	//   unparseable     - not even a JSON object; nothing at all was recovered.
	//   intactResources - names whose resourceMeta decoded in full; any other
	//                     resource's sync state is unknown and loads dirty.
	salvaged        bool
	unparseable     bool
	intactResources map[string]bool
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
		// A sidecar that fails to parse must never be read as "this calendar has no
		// sync state": empty state means every unsynced local edit looks clean (the
		// next pull silently overwrites it) and every pending deletion vanishes (the
		// item resurrects on the server). So: salvage every field still intact, keep
		// the original bytes for recovery, and report the failure so the UI shows it.
		// What could not be salvaged is UNKNOWN — the caller resolves unknown in the
		// data-preserving direction (assume unsynced), never as empty.
		salvaged := salvageSidecar(data)
		salvaged.Tombstones = dropUnsafeTombstoneNames(salvaged.Tombstones)
		err = fmt.Errorf("parsing sidecar: %w", err)
		if qerr := quarantineSidecar(dir, data); qerr != nil {
			return salvaged, fmt.Errorf("%w (original bytes NOT preserved: %v)", err, qerr)
		}
		return salvaged, fmt.Errorf("%w (original bytes kept as %s)", err, sidecarName+corruptSuffix)
	}
	sc.Tombstones = dropUnsafeTombstoneNames(sc.Tombstones)
	return &sc, nil
}

// corruptSuffix names the quarantine copy of a sidecar that failed to parse.
// The salvaged state is written back over the sidecar itself on the next store
// write, so without this copy the unrecovered metadata would be gone for good;
// with it, the exact original bytes stay on disk for hand-recovery.
const corruptSuffix = ".corrupt"

// quarantineSidecar preserves the raw bytes of a sidecar that failed to parse
// next to it, so nothing the salvage pass could not recover is destroyed by the
// rewrite that follows.
func quarantineSidecar(dir string, data []byte) error {
	return writeFileAtomic(filepath.Join(dir, sidecarName+corruptSuffix), data, filePerm)
}

// salvageSidecar rebuilds as much of a sidecar as is still readable after a
// strict decode failed, field by field: one bad value (a wrong JSON type, a
// truncated entry) then costs only that field instead of the calendar's entire
// sync state. The returned sidecar is always marked salvaged, so the caller can
// tell recovered state from state that is genuinely absent.
func salvageSidecar(data []byte) *sidecar {
	sc := &sidecar{salvaged: true}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		sc.unparseable = true
		return sc
	}
	// A field that won't decode keeps its zero value; "salvaged" tells the caller
	// that a zero value here may mean unknown rather than absent.
	get := func(key string, dst any) {
		if raw, ok := fields[key]; ok {
			_ = json.Unmarshal(raw, dst)
		}
	}
	get("display_name", &sc.DisplayName)
	get("color", &sc.Color)
	get("sync_token", &sc.SyncToken)
	get("ctag", &sc.CTag)
	get("href", &sc.Href)
	get("pending_create", &sc.PendingCreate)
	get("pending_delete", &sc.PendingDelete)
	get("pending_name", &sc.PendingName)
	get("pending_color", &sc.PendingColor)
	get("pending_props", &sc.PendingProps)
	get("components", &sc.Components)
	get("read_only", &sc.ReadOnly)
	sc.Resources, sc.intactResources = salvageResources(fields["resources"])
	sc.Tombstones = salvageTombstones(fields["tombstones"])
	return sc
}

// salvageResources decodes the resource map entry by entry, returning the
// recovered metadata plus the set of entries that decoded in full. An entry
// missing from that set has unknown sync state and must load dirty.
func salvageResources(raw json.RawMessage) (map[string]resourceMeta, map[string]bool) {
	entries, ok := jsonObject(raw)
	if !ok {
		return nil, nil
	}
	out := make(map[string]resourceMeta, len(entries))
	intact := make(map[string]bool, len(entries))
	for name, rawMeta := range entries {
		var m resourceMeta
		if err := json.Unmarshal(rawMeta, &m); err == nil {
			out[name] = m
			intact[name] = true
			continue
		}
		out[name] = salvageResourceMeta(rawMeta)
	}
	return out, intact
}

// salvageResourceMeta recovers the readable fields of one resource entry. ETag
// and Href matter most: with them a later push is a conditional PUT that the
// server can reject, instead of a blind create that duplicates the resource.
func salvageResourceMeta(raw json.RawMessage) resourceMeta {
	var m resourceMeta
	fields, ok := jsonObject(raw)
	if !ok {
		return m
	}
	get := func(key string, dst any) {
		if f, ok := fields[key]; ok {
			_ = json.Unmarshal(f, dst)
		}
	}
	get("etag", &m.ETag)
	get("href", &m.Href)
	get("dirty", &m.Dirty)
	get("hash", &m.Hash)
	var c conflictMeta
	if f, ok := fields["conflict"]; ok && json.Unmarshal(f, &c) == nil {
		m.Conflict = &c
	}
	return m
}

// salvageTombstones decodes pending deletions entry by entry: losing one must
// not lose the rest, since a dropped tombstone resurrects a deleted item.
func salvageTombstones(raw json.RawMessage) map[string]tombstoneMeta {
	entries, ok := jsonObject(raw)
	if !ok {
		return nil
	}
	out := make(map[string]tombstoneMeta, len(entries))
	for name, rawMeta := range entries {
		var tm tombstoneMeta
		if err := json.Unmarshal(rawMeta, &tm); err == nil {
			out[name] = tm
			continue
		}
		fields, ok := jsonObject(rawMeta)
		if !ok {
			continue
		}
		if f, ok := fields["href"]; ok {
			_ = json.Unmarshal(f, &tm.Href)
		}
		if f, ok := fields["etag"]; ok {
			_ = json.Unmarshal(f, &tm.ETag)
		}
		// Kept even if the href did not survive: such a tombstone can't be pushed
		// and sync reports it as a skip, which is a visible failure — dropping it
		// instead silently undoes the user's deletion.
		out[name] = tm
	}
	return out
}

func jsonObject(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, false
	}
	return out, true
}

// dropUnsafeTombstoneNames removes tombstone entries whose key is not a single
// safe path element. Unlike the resource map — whose keys are real directory
// entries read back off disk — tombstone keys come straight out of the sidecar's
// JSON and are later joined onto the cache root (the 412 delete-vs-server-change
// resurrect writes through filepath.Join(root, calID, name)), so a corrupt or
// hostile sidecar could otherwise make sync write outside the data dir. The bad
// entry is dropped rather than the whole load failing: the sidecar is derived
// data, and its remaining entries are still needed to push real deletions.
func dropUnsafeTombstoneNames(in map[string]tombstoneMeta) map[string]tombstoneMeta {
	for name := range in {
		if !validPathElement(name) {
			delete(in, name)
		}
	}
	return in
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
