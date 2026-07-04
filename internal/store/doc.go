// Package store is the vdir cache: it reads and writes per-resource .ics files
// on disk, maintains the JSON sync-state sidecar, and builds the in-memory
// index used for date-range and todo queries. It is the only package that
// reads or writes the cache directory.
package store
