# PQPM Makefile
# Build the daemon and CLI binaries

BINARY_DAEMON = pqpmd
BINARY_CLI = pqpm
BINARY_UI = pqpm-ui
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

.PHONY: all build daemon cli ui clean install install-ui uninstall fmt vet test release e2e

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

ui:
	@echo "Building UI addon ($(VERSION) @ $(COMMIT))..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BUILD_DIR)/$(BINARY_UI) ./addons/ui/cmd/pqpm-ui

clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR) dist/

test:
	$(GO) test -v -race ./...

e2e:
	@chmod +x test/e2e/*.sh
	./test/e2e/run.sh

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
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o dist/$(BINARY_UI)-linux-amd64 ./addons/ui/cmd/pqpm-ui

	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o dist/$(BINARY_DAEMON)-linux-arm64 ./cmd/daemon
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o dist/$(BINARY_CLI)-linux-arm64 ./cmd/cli
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o dist/$(BINARY_UI)-linux-arm64 ./addons/ui/cmd/pqpm-ui

	@echo "Packaging archives..."
	cd dist && tar czf pqpm-$(VERSION)-linux-amd64.tar.gz $(BINARY_DAEMON)-linux-amd64 $(BINARY_CLI)-linux-amd64
	cd dist && tar czf pqpm-$(VERSION)-linux-arm64.tar.gz $(BINARY_DAEMON)-linux-arm64 $(BINARY_CLI)-linux-arm64
	# UI addon tarballs (opt-in install via install-ui.sh)
	mkdir -p dist/ui-amd64 dist/ui-arm64
	cp dist/$(BINARY_UI)-linux-amd64 dist/ui-amd64/pqpm-ui
	cp addons/ui/systemd/pqpm-ui.service dist/ui-amd64/
	cp -a addons/ui/webmin dist/ui-amd64/webmin
	cp dist/$(BINARY_UI)-linux-arm64 dist/ui-arm64/pqpm-ui
	cp addons/ui/systemd/pqpm-ui.service dist/ui-arm64/
	cp -a addons/ui/webmin dist/ui-arm64/webmin
	cd dist/ui-amd64 && tar czf ../pqpm-ui-$(VERSION)-linux-amd64.tar.gz pqpm-ui pqpm-ui.service webmin
	cd dist/ui-arm64 && tar czf ../pqpm-ui-$(VERSION)-linux-arm64.tar.gz pqpm-ui pqpm-ui.service webmin
	rm -rf dist/ui-amd64 dist/ui-arm64
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
	install -d -m 0755 /var/lib/pqpm

install-ui: ui
	@echo "Installing UI addon (requires core pqpm)..."
	install -d /usr/local/bin
	install -m 0755 $(BUILD_DIR)/$(BINARY_UI) /usr/local/bin/$(BINARY_UI)
	install -m 0644 addons/ui/systemd/pqpm-ui.service /etc/systemd/system/pqpm-ui.service
	@command -v systemctl >/dev/null 2>&1 && systemctl daemon-reload || true
	@echo "Enable with: systemctl enable --now pqpm-ui"

uninstall:
	@echo "Removing binaries..."
	rm -f /usr/local/bin/$(BINARY_DAEMON)
	rm -f /usr/local/bin/$(BINARY_CLI)
	rm -f /usr/local/bin/$(BINARY_UI)
	@echo "Note: /var/run/pqpm, /var/log/pqpm, and /var/lib/pqpm were not removed. Remove manually if desired."
