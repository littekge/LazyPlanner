package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// defaultConfigTemplate is the fully-commented config.toml written on first run.
// Every option is listed at its default (commented out), so the only edit needed
// to get a working config is filling in the [server] connection. The app never
// rewrites this file after generating it — it is the user's to hand-edit.
const defaultConfigTemplate = `# LazyPlanner configuration.
#
# The only required section is [server]. Every other option is shown below at
# its default value (commented out) — uncomment and change what you want.
# LazyPlanner reads this file once at startup and never writes it.

[server]
# CalDAV base URL. For NextCloud this is typically:
#   https://your-nextcloud.example.com/remote.php/dav
url = ""
username = ""

# Authentication uses a NextCloud *app password* (Settings -> Security ->
# Devices & sessions), never your account password. Provide it one of two ways:
#
#   1. Inline (keep this file chmod 600 — it holds a secret):
# password = "your-app-password"
#
#   2. A command whose stdout is the password (preferred — keeps the secret out
#      of the file). Example with Vaultwarden/Bitwarden:
# password_command = "bw get password lazyplanner"

# [appearance]
# first_day_of_week = "monday"   # "monday" or "sunday"
# default_view = "month"         # "month", "week", or "day"
# time_format = "12h"            # "12h" (2:30pm) or "24h" (14:30)
# date_format = "us"             # "us" (07/04/2026) or "iso" (2026-07-04)

# [behavior]
# sync_interval_minutes = 15     # periodic background sync; 0 disables it
`

// GenerateDefault writes the commented default config.toml into ConfigDir if no
// config file exists yet, creating the directory. It returns the path written
// and true, or ("", false) if a config already exists (it never overwrites a
// user's file). The file is written 0600 since it may come to hold a password.
func GenerateDefault() (path string, written bool, err error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", false, err
	}
	path = filepath.Join(dir, configName)
	if _, err := os.Stat(path); err == nil {
		return "", false, nil // already exists; never overwrite
	} else if !os.IsNotExist(err) {
		return "", false, fmt.Errorf("stat config %q: %w", path, err)
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", false, fmt.Errorf("creating config dir %q: %w", dir, err)
	}
	if err := os.WriteFile(path, []byte(defaultConfigTemplate), 0o600); err != nil {
		return "", false, fmt.Errorf("writing default config %q: %w", path, err)
	}
	return path, true, nil
}
