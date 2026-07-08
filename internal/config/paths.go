package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// appDir is the per-user subdirectory LazyPlanner owns under the OS config and
// data locations.
const appDir = "lazyplanner"

// ConfigDir returns the directory holding LazyPlanner's config file:
// ~/.config/lazyplanner on Linux ($XDG_CONFIG_HOME honored), %APPDATA%\lazyplanner
// on Windows. The directory is not created.
func ConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolving config dir: %w", err)
	}
	return filepath.Join(base, appDir), nil
}

// ConfigPath returns the full path to the config file (ConfigDir/config.toml).
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configName), nil
}

// DataDir returns the directory holding LazyPlanner's calendar cache:
// ~/.local/share/lazyplanner on Linux ($XDG_DATA_HOME honored),
// %LOCALAPPDATA%\lazyplanner on Windows. This is durable data — it can hold
// offline edits not yet synced — so it lives under data paths, never a cache
// directory. The directory is not created.
func DataDir() (string, error) {
	base, err := dataHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appDir), nil
}

func dataHome() (string, error) {
	switch runtime.GOOS {
	case "windows":
		if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
			return dir, nil
		}
		// Fall back to the roaming config location if LOCALAPPDATA is unset.
		return os.UserConfigDir()
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolving data dir: %w", err)
		}
		return filepath.Join(home, "Library", "Application Support"), nil
	default: // Linux and other XDG-style systems.
		if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
			return dir, nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolving data dir: %w", err)
		}
		return filepath.Join(home, ".local", "share"), nil
	}
}
