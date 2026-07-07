package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// sidecar is the on-disk JSON companion to a calendar directory. It caches
// server-owned metadata and per-resource sync state (ETags, hrefs, the sync
// token). It is derived data: if it is lost or corrupt the .ics files still
// stand as the source of truth, and a fresh sync rebuilds it.
type sidecar struct {
	DisplayName string                  `json:"display_name,omitempty"`
	Color       string                  `json:"color,omitempty"`
	SyncToken   string                  `json:"sync_token,omitempty"`
	Href        string                  `json:"href,omitempty"`
	Resources   map[string]resourceMeta `json:"resources,omitempty"`
	// Tombstones records resources deleted locally that still need to be deleted
	// on the server, keyed by their (now-gone) .ics file name. They are kept
	// until sync pushes the deletion, then cleared.
	Tombstones map[string]tombstoneMeta `json:"tombstones,omitempty"`
}

type resourceMeta struct {
	ETag  string `json:"etag,omitempty"`
	Href  string `json:"href,omitempty"`
	Dirty bool   `json:"dirty,omitempty"`
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
	data, err := os.ReadFile(filepath.Join(dir, sidecarName))
	if errors.Is(err, os.ErrNotExist) {
		return &sidecar{}, nil
	}
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
		DisplayName: cs.displayName,
		Color:       cs.color,
		SyncToken:   cs.syncToken,
		Href:        cs.href,
		Resources:   make(map[string]resourceMeta, len(cs.resources)),
	}
	for name, r := range cs.resources {
		sc.Resources[name] = resourceMeta{ETag: r.ETag, Href: r.Href, Dirty: r.Dirty}
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
