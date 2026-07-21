package state

// GlobalFileName is the cross-account state file's name, stored at the data-dir
// root rather than inside any one account's directory.
const GlobalFileName = "global.json"

// Global is app-remembered state that spans accounts. It lives at the data-dir
// root so it survives (and is shared across) account switches, unlike State,
// which is per-account. Zero values mean "no preference", so a missing or corrupt
// file is harmless.
type Global struct {
	// ActiveAccountID is the id (config.AccountID) of the account that was active
	// at the last quit or switch. Empty means no preference — the caller resolves
	// to the first configured account.
	ActiveAccountID string `json:"active_account_id,omitempty"`
}

// LoadGlobal reads the global state file at path. Any problem (missing file, bad
// JSON) yields a zero Global rather than an error — the remembered active account
// must never block startup.
func LoadGlobal(path string) Global {
	var g Global
	if !readJSONFile(path, &g) {
		return Global{}
	}
	return g
}

// SaveGlobal writes the global state file at path (0600), creating the directory
// if needed, via a temp file and rename so a crash never leaves it half-written.
func SaveGlobal(path string, g Global) error {
	return writeJSONFile(path, g)
}
