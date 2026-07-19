# LazyPlanner build helpers. Plain `go build` still works everywhere; these
# targets add the full check gate and stripped Raspberry Pi cross-builds.
#
# Pi builds are pure-Go static binaries (no cgo), so they cross-compile from any
# machine with no toolchain beyond Go. -ldflags="-s -w" drops the symbol table
# and DWARF to roughly halve the binary; -trimpath keeps paths out of it.

BIN     := lazyplanner
PKG     := ./cmd/lazyplanner
DIST    := dist

# VERSION is derived from the git tag — GitHub Releases + tags are the source of
# truth, so nothing is hand-maintained in the source. `git describe` yields the
# tag (e.g. v1.0.0), or tag-N-gHASH between tags, or the short hash before any
# tag; -dirty flags an uncommitted tree. Falls back to "dev" without git.
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
VERSION_LDFLAGS := -X main.appVersion=$(VERSION)

LDFLAGS := -s -w $(VERSION_LDFLAGS)
GOFLAGS := -trimpath -ldflags "$(LDFLAGS)"

.PHONY: all build check test vet staticcheck fmt run clean \
        cross pi-arm64 pi-armv7 pi-armv6

## build: native binary for this machine (version injected from the git tag)
build:
	go build -ldflags "$(VERSION_LDFLAGS)" -o $(BIN) $(PKG)

## check: the full gate (test + vet + staticcheck) — run before committing
check: test vet staticcheck

test:
	go test ./...

vet:
	go vet ./...

staticcheck:
	staticcheck ./...

## fmt: gofmt the tree in place
fmt:
	gofmt -w .

## run: build and launch the TUI
run: build
	./$(BIN)

## cross: all Raspberry Pi targets, stripped, into dist/
cross: pi-arm64 pi-armv7 pi-armv6

## pi-arm64: 64-bit Raspberry Pi OS (Pi 3/4/5, Zero 2 W on 64-bit)
pi-arm64:
	GOOS=linux GOARCH=arm64 go build $(GOFLAGS) -o $(DIST)/$(BIN)-linux-arm64 $(PKG)

## pi-armv7: 32-bit Raspberry Pi OS (Pi 2/3/4, Zero 2 W)
pi-armv7:
	GOOS=linux GOARCH=arm GOARM=7 go build $(GOFLAGS) -o $(DIST)/$(BIN)-linux-armv7 $(PKG)

## pi-armv6: Pi 1 / Pi Zero / Zero W
pi-armv6:
	GOOS=linux GOARCH=arm GOARM=6 go build $(GOFLAGS) -o $(DIST)/$(BIN)-linux-armv6 $(PKG)

## clean: remove build output
clean:
	rm -f $(BIN)
	rm -rf $(DIST)
