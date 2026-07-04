// Package config loads and validates the LazyPlanner TOML configuration file.
// It reads configuration once at startup and never writes it back; app-managed
// state belongs in a state file under the data directory instead. See main.md.
package config
