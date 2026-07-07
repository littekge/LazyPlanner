// Package state persists small app-remembered UI preferences (like chosen pane
// sizes) in a JSON file under the data directory. This is deliberately separate
// from config (which the user hand-edits and the app never writes) and from the
// vdir cache (calendar data): it is the app's own scratch state, safe to lose.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// FileName is the state file's name within the data directory.
const FileName = "state.json"

// State is the persisted UI state. Zero values mean "use the app default", so a
// missing or partial file is harmless.
type State struct {
	// LeftWidth is the remembered width of the left overview column, in columns.
	LeftWidth int `json:"left_width,omitempty"`
}

// Load reads the state file at path. Any problem (missing file, bad JSON) yields
// a zero State rather than an error — remembered preferences must never block
// startup.
func Load(path string) State {
	var s State
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}
	}
	return s
}

// Save writes the state file at path (0600), creating the directory if needed.
// It writes to a temp file and renames so a crash never leaves a half-written
// state file.
func Save(path string, s State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
