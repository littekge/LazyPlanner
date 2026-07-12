# LazyPlanner build helpers. Plain `go build` still works everywhere; these
# targets add the full check gate and stripped Raspberry Pi cross-builds.
#
# Pi builds are pure-Go static binaries (no cgo), so they cross-compile from any
# machine with no toolchain beyond Go. -ldflags="-s -w" drops the symbol table
# and DWARF to roughly halve the binary; -trimpath keeps paths out of it.

BIN     := lazyplanner
PKG     := ./cmd/lazyplanner
DIST    := dist
LDFLAGS := -s -w
GOFLAGS := -trimpath -ldflags "$(LDFLAGS)"

.PHONY: all build check test vet staticcheck fmt run clean \
        cross pi-arm64 pi-armv7 pi-armv6

## build: native binary for this machine
build:
	go build -o $(BIN) $(PKG)

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
