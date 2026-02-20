.PHONY: all build install uninstall clean help test install-watchdog enable-watchdog disable-watchdog watchdog-status watchdog-run-once test-watchdog-mock watchdog-kick

# Build variables
BINARY_NAME=picoclaw
BUILD_DIR=build
CMD_DIR=cmd/$(BINARY_NAME)
MAIN_GO=$(CMD_DIR)/main.go

# Version
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT=$(shell git rev-parse --short=8 HEAD 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date +%FT%T%z)
GO_VERSION=$(shell $(GO) version | awk '{print $$3}')
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.gitCommit=$(GIT_COMMIT) -X main.buildTime=$(BUILD_TIME) -X main.goVersion=$(GO_VERSION)"

# Go variables
GO?=go
GOFLAGS?=-v

# Installation
INSTALL_PREFIX?=$(HOME)/.local
INSTALL_BIN_DIR=$(INSTALL_PREFIX)/bin
INSTALL_MAN_DIR=$(INSTALL_PREFIX)/share/man/man1

# Workspace and Skills
PICOCLAW_HOME?=$(HOME)/.picoclaw
WORKSPACE_DIR?=$(PICOCLAW_HOME)/workspace
WORKSPACE_SKILLS_DIR=$(WORKSPACE_DIR)/skills
BUILTIN_SKILLS_DIR=$(CURDIR)/skills
SYSTEMD_USER_DIR=$(HOME)/.config/systemd/user
PICOCLAW_SHARE_DIR=$(INSTALL_PREFIX)/share/picoclaw
WATCHDOG_SCRIPT_SRC=$(CURDIR)/scripts/ops_watchdog.sh
WATCHDOG_SCRIPT_DST=$(PICOCLAW_SHARE_DIR)/scripts/ops_watchdog.sh
WATCHDOG_KICK_SCRIPT_SRC=$(CURDIR)/scripts/ops_watchdog_kick.sh
WATCHDOG_KICK_SCRIPT_DST=$(PICOCLAW_SHARE_DIR)/scripts/ops_watchdog_kick.sh
WATCHDOG_SERVICE_SRC=$(CURDIR)/systemd/user/picoclaw-watchdog.service
WATCHDOG_TIMER_SRC=$(CURDIR)/systemd/user/picoclaw-watchdog.timer

# OS detection
UNAME_S:=$(shell uname -s)
UNAME_M:=$(shell uname -m)

# Platform-specific settings
ifeq ($(UNAME_S),Linux)
	PLATFORM=linux
	ifeq ($(UNAME_M),x86_64)
		ARCH=amd64
	else ifeq ($(UNAME_M),aarch64)
		ARCH=arm64
	else ifeq ($(UNAME_M),riscv64)
		ARCH=riscv64
	else
		ARCH=$(UNAME_M)
	endif
else ifeq ($(UNAME_S),Darwin)
	PLATFORM=darwin
	ifeq ($(UNAME_M),x86_64)
		ARCH=amd64
	else ifeq ($(UNAME_M),arm64)
		ARCH=arm64
	else
		ARCH=$(UNAME_M)
	endif
else
	PLATFORM=$(UNAME_S)
	ARCH=$(UNAME_M)
endif

BINARY_PATH=$(BUILD_DIR)/$(BINARY_NAME)-$(PLATFORM)-$(ARCH)

# Default target
all: build

## generate: Run generate
generate:
	@echo "Run generate..."
	@rm -r ./$(CMD_DIR)/workspace 2>/dev/null || true
	@$(GO) generate ./...
	@echo "Run generate complete"

## build: Build the picoclaw binary for current platform
build: generate
	@echo "Building $(BINARY_NAME) for $(PLATFORM)/$(ARCH)..."
	@mkdir -p $(BUILD_DIR)
	@$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY_PATH) ./$(CMD_DIR)
	@echo "Build complete: $(BINARY_PATH)"
	@ln -sf $(BINARY_NAME)-$(PLATFORM)-$(ARCH) $(BUILD_DIR)/$(BINARY_NAME)

## build-all: Build picoclaw for all platforms
build-all: generate
	@echo "Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./$(CMD_DIR)
	GOOS=linux GOARCH=arm64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./$(CMD_DIR)
	GOOS=linux GOARCH=riscv64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-riscv64 ./$(CMD_DIR)
	GOOS=darwin GOARCH=arm64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./$(CMD_DIR)
	GOOS=windows GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./$(CMD_DIR)
	@echo "All builds complete"

## install: Install picoclaw to system and copy builtin skills
install: build
	@echo "Installing $(BINARY_NAME)..."
	@mkdir -p $(INSTALL_BIN_DIR)
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_BIN_DIR)/$(BINARY_NAME)
	@chmod +x $(INSTALL_BIN_DIR)/$(BINARY_NAME)
	@echo "Installed binary to $(INSTALL_BIN_DIR)/$(BINARY_NAME)"
	@echo "Installation complete!"
	@echo "Tip: run 'make install-watchdog enable-watchdog' to enable ops watchdog."

## install-watchdog: Install watchdog script and systemd --user units
install-watchdog:
	@echo "Installing watchdog script and systemd units..."
	@mkdir -p $(PICOCLAW_SHARE_DIR)/scripts
	@mkdir -p $(SYSTEMD_USER_DIR)
	@cp $(WATCHDOG_SCRIPT_SRC) $(WATCHDOG_SCRIPT_DST)
	@chmod +x $(WATCHDOG_SCRIPT_DST)
	@cp $(WATCHDOG_KICK_SCRIPT_SRC) $(WATCHDOG_KICK_SCRIPT_DST)
	@chmod +x $(WATCHDOG_KICK_SCRIPT_DST)
	@sed 's#%h/.local/share/picoclaw/scripts/ops_watchdog.sh#$(WATCHDOG_SCRIPT_DST)#g' $(WATCHDOG_SERVICE_SRC) > $(SYSTEMD_USER_DIR)/picoclaw-watchdog.service
	@cp $(WATCHDOG_TIMER_SRC) $(SYSTEMD_USER_DIR)/picoclaw-watchdog.timer
	@systemctl --user daemon-reload
	@echo "Installed: $(WATCHDOG_SCRIPT_DST)"
	@echo "Installed: $(SYSTEMD_USER_DIR)/picoclaw-watchdog.service"
	@echo "Installed: $(SYSTEMD_USER_DIR)/picoclaw-watchdog.timer"

## enable-watchdog: Enable and start watchdog timer
enable-watchdog:
	@systemctl --user daemon-reload
	@systemctl --user enable --now picoclaw-watchdog.timer
	@echo "watchdog timer enabled."

## disable-watchdog: Disable and stop watchdog timer
disable-watchdog:
	@systemctl --user disable --now picoclaw-watchdog.timer || true
	@echo "watchdog timer disabled."

## watchdog-status: Show watchdog timer/service status
watchdog-status:
	@systemctl --user status picoclaw-watchdog.timer --no-pager || true
	@systemctl --user status picoclaw-watchdog.service --no-pager || true

## watchdog-run-once: Run watchdog script one time
watchdog-run-once:
	@bash "$(WATCHDOG_SCRIPT_DST)" once

## test-watchdog-mock: Run mock-based watchdog regression tests
test-watchdog-mock:
	@bash scripts/tests/watchdog_mock_test.sh

## watchdog-kick: Queue manual watchdog action (ACTION=restart_gateway|recover_funnel|check_ollama SOURCE=line TOKEN=...)
watchdog-kick:
	@bash "$(WATCHDOG_KICK_SCRIPT_DST)" "$(ACTION)" "$(SOURCE)" "$(TOKEN)"

## uninstall: Remove picoclaw from system
uninstall:
	@echo "Uninstalling $(BINARY_NAME)..."
	@rm -f $(INSTALL_BIN_DIR)/$(BINARY_NAME)
	@echo "Removed binary from $(INSTALL_BIN_DIR)/$(BINARY_NAME)"
	@echo "Note: Only the executable file has been deleted."
	@echo "If you need to delete all configurations (config.json, workspace, etc.), run 'make uninstall-all'"

## uninstall-all: Remove picoclaw and all data
uninstall-all:
	@echo "Removing workspace and skills..."
	@rm -rf $(PICOCLAW_HOME)
	@echo "Removed workspace: $(PICOCLAW_HOME)"
	@echo "Complete uninstallation done!"

## clean: Remove build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@echo "Clean complete"

## vet: Run go vet for static analysis
vet:
	@$(GO) vet ./...

## fmt: Format Go code
test:
	@$(GO) test ./...

## fmt: Format Go code
fmt:
	@$(GO) fmt ./...

## deps: Download dependencies
deps:
	@$(GO) mod download
	@$(GO) mod verify

## update-deps: Update dependencies
update-deps:
	@$(GO) get -u ./...
	@$(GO) mod tidy

## check: Run vet, fmt, and verify dependencies
check: deps fmt vet test

## run: Build and run picoclaw
run: build
	@$(BUILD_DIR)/$(BINARY_NAME) $(ARGS)

## help: Show this help message
help:
	@echo "picoclaw Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
	@echo ""
	@echo "Examples:"
	@echo "  make build              # Build for current platform"
	@echo "  make install            # Install to ~/.local/bin"
	@echo "  make uninstall          # Remove from /usr/local/bin"
	@echo "  make install-skills     # Install skills to workspace"
	@echo ""
	@echo "Environment Variables:"
	@echo "  INSTALL_PREFIX          # Installation prefix (default: ~/.local)"
	@echo "  WORKSPACE_DIR           # Workspace directory (default: ~/.picoclaw/workspace)"
	@echo "  VERSION                 # Version string (default: git describe)"
	@echo ""
	@echo "Current Configuration:"
	@echo "  Platform: $(PLATFORM)/$(ARCH)"
	@echo "  Binary: $(BINARY_PATH)"
	@echo "  Install Prefix: $(INSTALL_PREFIX)"
	@echo "  Workspace: $(WORKSPACE_DIR)"
