# Preserved reproductions for OPEN findings

A `.go.txt` file here is a **failing** reproduction for a finding that is recorded as an open residual
rather than fixed. The `.txt` suffix keeps it out of the build: dropping it into the package path named
at the top of the file and renaming it to `.go` reproduces the defect.

Once a finding is fixed, its repro becomes a real regression test in the package it belongs to and the
copy here is deleted — this directory only ever holds what is still open.

| File | Finding | Recorded in |
|---|---|---|
| `pass24-M4-dstgap-timed-anchor_test.go.txt` | A timed recurring todo bakes a DST gap into its anchor, permanently. Drop into `internal/model/`. | `docs/audit/passes/PASS-24.md` (M4), `COVERAGE.md` |
