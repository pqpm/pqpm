# PQPM Makefile
# Build the daemon and CLI binaries

BINARY_DAEMON = pqpmd
BINARY_CLI = pqpm
BUILD_DIR = bin
GO = go
GOFLAGS = -trimpath

VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

LDFLAGS = -s -w \
	-X github.com/pqpm/pqpm/internal/version.Version=$(VERSION) \
	-X github.com/pqpm/pqpm/internal/version.Commit=$(COMMIT) \
	-X github.com/pqpm/pqpm/internal/version.Date=$(DATE)

.PHONY: all build daemon cli clean install uninstall fmt vet test release

all: build

build: daemon cli

daemon:
	@echo "Building daemon ($(VERSION) @ $(COMMIT))..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BUILD_DIR)/$(BINARY_DAEMON) ./cmd/daemon

cli:
	@echo "Building CLI ($(VERSION) @ $(COMMIT))..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BUILD_DIR)/$(BINARY_CLI) ./cmd/cli

clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR) dist/

test:
	$(GO) test -v -race ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

# Cross-compile for release (Linux amd64 + arm64)
release: clean
	@echo "Building release binaries..."
	@mkdir -p dist

	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o dist/$(BINARY_DAEMON)-linux-amd64 ./cmd/daemon
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o dist/$(BINARY_CLI)-linux-amd64 ./cmd/cli

	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o dist/$(BINARY_DAEMON)-linux-arm64 ./cmd/daemon
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o dist/$(BINARY_CLI)-linux-arm64 ./cmd/cli

	@echo "Packaging archives..."
	cd dist && tar czf pqpm-$(VERSION)-linux-amd64.tar.gz $(BINARY_DAEMON)-linux-amd64 $(BINARY_CLI)-linux-amd64
	cd dist && tar czf pqpm-$(VERSION)-linux-arm64.tar.gz $(BINARY_DAEMON)-linux-arm64 $(BINARY_CLI)-linux-arm64
	cd dist && sha256sum *.tar.gz > checksums.txt

	@echo "Release artifacts in dist/"

install: build
	@echo "Installing binaries..."
	install -d /usr/local/bin
	install -m 0755 $(BUILD_DIR)/$(BINARY_DAEMON) /usr/local/bin/$(BINARY_DAEMON)
	install -m 0755 $(BUILD_DIR)/$(BINARY_CLI) /usr/local/bin/$(BINARY_CLI)
	@echo "Creating runtime directories..."
	install -d -m 0755 /var/run/pqpm
	install -d -m 0755 /var/log/pqpm
	install -d -m 0755 /var/log/pqpm/users

uninstall:
	@echo "Removing binaries..."
	rm -f /usr/local/bin/$(BINARY_DAEMON)
	rm -f /usr/local/bin/$(BINARY_CLI)
	@echo "Note: /var/run/pqpm and /var/log/pqpm were not removed. Remove manually if desired."
