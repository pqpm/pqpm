# PQPM Makefile
# Build the daemon and CLI binaries

BINARY_DAEMON = pqpmd
BINARY_CLI = pqpm
BUILD_DIR = bin
GO = go
GOFLAGS = -trimpath
LDFLAGS = -s -w

.PHONY: all build daemon cli clean install uninstall fmt vet test

all: build

build: daemon cli

daemon:
	@echo "Building daemon..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_DAEMON) ./cmd/daemon

cli:
	@echo "Building CLI..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_CLI) ./cmd/cli

clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)

test:
	$(GO) test -v -race ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

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
