# LazyPlanner build helpers. Plain `go build` still works everywhere; these
# targets add the full check gate and the stripped cross-builds shipped on each
# GitHub Release.
#
# All cross-builds are CGO-free pure-Go static binaries (embedded tzdata, pure-Go
# tcell/CalDAV stack), so every target cross-compiles from any machine with no
# toolchain beyond Go. -ldflags="-s -w" drops the symbol table and DWARF to
# roughly halve the binary; -trimpath keeps build paths out of it.

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

# Every release binary is named lazyplanner_<os>_<arch> (Windows gets .exe), so a
# downloaded asset says on its face what it runs on.
.PHONY: all build check test vet staticcheck fmt run clean \
        release checksums cross \
        build-linux-amd64 build-linux-arm64 build-linux-armv7 build-linux-armv6 \
        build-windows-amd64 build-darwin-amd64 build-darwin-arm64

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

# dist/ is an order-only prerequisite of every cross-build: `go build -o` won't
# create the directory itself, and an order-only dep avoids spurious rebuilds.
$(DIST):
	mkdir -p $(DIST)

## release: build every distributable target into dist/, then write checksums
release: build-linux-amd64 build-linux-arm64 build-linux-armv7 build-linux-armv6 \
         build-windows-amd64 build-darwin-amd64 build-darwin-arm64
	cd $(DIST) && sha256sum $(BIN)_* > sha256sums.txt

## checksums: (re)generate sha256sums.txt over the binaries already in dist/
checksums:
	cd $(DIST) && sha256sum $(BIN)_* > sha256sums.txt

## cross: the Raspberry Pi ARM builds — the cheap ARM regression check run in CI
cross: build-linux-arm64 build-linux-armv7 build-linux-armv6

## build-linux-amd64: 64-bit x86 Linux (primary desktop/server)
build-linux-amd64: | $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -o $(DIST)/$(BIN)_linux_amd64 $(PKG)

## build-linux-arm64: 64-bit Raspberry Pi OS (Pi 3/4/5, Zero 2 W on 64-bit)
build-linux-arm64: | $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(GOFLAGS) -o $(DIST)/$(BIN)_linux_arm64 $(PKG)

## build-linux-armv7: 32-bit Raspberry Pi OS (Pi 2/3/4, Zero 2 W)
build-linux-armv7: | $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build $(GOFLAGS) -o $(DIST)/$(BIN)_linux_armv7 $(PKG)

## build-linux-armv6: Pi 1 / Pi Zero / Zero W
build-linux-armv6: | $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=6 go build $(GOFLAGS) -o $(DIST)/$(BIN)_linux_armv6 $(PKG)

## build-windows-amd64: 64-bit Windows (secondary target)
build-windows-amd64: | $(DIST)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(GOFLAGS) -o $(DIST)/$(BIN)_windows_amd64.exe $(PKG)

## build-darwin-amd64: Intel macOS
build-darwin-amd64: | $(DIST)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(GOFLAGS) -o $(DIST)/$(BIN)_darwin_amd64 $(PKG)

## build-darwin-arm64: Apple Silicon macOS
build-darwin-arm64: | $(DIST)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(GOFLAGS) -o $(DIST)/$(BIN)_darwin_arm64 $(PKG)

## clean: remove build output
clean:
	rm -f $(BIN)
	rm -rf $(DIST)
