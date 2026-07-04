// Package sync is the sync engine: it diffs the local store against the server
// via ETags, applies changes in both directions, and handles conflicts without
// ever silently overwriting either side.
package sync
