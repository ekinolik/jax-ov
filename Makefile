# Makefile for jax-ov project

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get

# Build parameters
BINARY_DIR=bin
LINUX_BINARY_DIR=$(BINARY_DIR)/linux/jax-ov
GOOS_LINUX=linux
GOARCH=amd64

# Commands to build
COMMANDS=monitor reconstruct analyze log-analyze extract log-extract top-contracts logger server

# Default target - build for current OS
.PHONY: all
all: $(COMMANDS)

# Build all commands for Linux
.PHONY: linux
linux: $(addprefix linux-,$(COMMANDS))

# Build all commands for Linux in a single directory
.PHONY: linux-all
linux-all:
	@echo "Building all commands for Linux..."
	@mkdir -p $(LINUX_BINARY_DIR)
	@for cmd in $(COMMANDS); do \
		echo "Building $$cmd for Linux..."; \
		GOOS=$(GOOS_LINUX) GOARCH=$(GOARCH) $(GOBUILD) -o $(LINUX_BINARY_DIR)/$$cmd ./cmd/$$cmd || exit 1; \
	done
	@echo "All Linux binaries built in $(LINUX_BINARY_DIR)/"

# Individual command targets (local build)
monitor:
	@echo "Building monitor..."
	$(GOBUILD) -o monitor ./cmd/monitor

reconstruct:
	@echo "Building reconstruct..."
	$(GOBUILD) -o reconstruct ./cmd/reconstruct

analyze:
	@echo "Building analyze..."
	$(GOBUILD) -o analyze ./cmd/analyze

log-analyze:
	@echo "Building log-analyze..."
	$(GOBUILD) -o log-analyze ./cmd/log-analyze

extract:
	@echo "Building extract..."
	$(GOBUILD) -o extract ./cmd/extract

log-extract:
	@echo "Building log-extract..."
	$(GOBUILD) -o log-extract ./cmd/log-extract

top-contracts:
	@echo "Building top-contracts..."
	$(GOBUILD) -o top-contracts ./cmd/top-contracts

logger:
	@echo "Building logger..."
	$(GOBUILD) -o logger ./cmd/logger

server:
	@echo "Building server..."
	$(GOBUILD) -o server ./cmd/server

# Linux-specific builds
linux-monitor:
	@echo "Building monitor for Linux..."
	@mkdir -p $(LINUX_BINARY_DIR)
	GOOS=$(GOOS_LINUX) GOARCH=$(GOARCH) $(GOBUILD) -o $(LINUX_BINARY_DIR)/monitor ./cmd/monitor

linux-reconstruct:
	@echo "Building reconstruct for Linux..."
	@mkdir -p $(LINUX_BINARY_DIR)
	GOOS=$(GOOS_LINUX) GOARCH=$(GOARCH) $(GOBUILD) -o $(LINUX_BINARY_DIR)/reconstruct ./cmd/reconstruct

linux-analyze:
	@echo "Building analyze for Linux..."
	@mkdir -p $(LINUX_BINARY_DIR)
	GOOS=$(GOOS_LINUX) GOARCH=$(GOARCH) $(GOBUILD) -o $(LINUX_BINARY_DIR)/analyze ./cmd/analyze

linux-log-analyze:
	@echo "Building log-analyze for Linux..."
	@mkdir -p $(LINUX_BINARY_DIR)
	GOOS=$(GOOS_LINUX) GOARCH=$(GOARCH) $(GOBUILD) -o $(LINUX_BINARY_DIR)/log-analyze ./cmd/log-analyze

linux-extract:
	@echo "Building extract for Linux..."
	@mkdir -p $(LINUX_BINARY_DIR)
	GOOS=$(GOOS_LINUX) GOARCH=$(GOARCH) $(GOBUILD) -o $(LINUX_BINARY_DIR)/extract ./cmd/extract

linux-log-extract:
	@echo "Building log-extract for Linux..."
	@mkdir -p $(LINUX_BINARY_DIR)
	GOOS=$(GOOS_LINUX) GOARCH=$(GOARCH) $(GOBUILD) -o $(LINUX_BINARY_DIR)/log-extract ./cmd/log-extract

linux-top-contracts:
	@echo "Building top-contracts for Linux..."
	@mkdir -p $(LINUX_BINARY_DIR)
	GOOS=$(GOOS_LINUX) GOARCH=$(GOARCH) $(GOBUILD) -o $(LINUX_BINARY_DIR)/top-contracts ./cmd/top-contracts

linux-logger:
	@echo "Building logger for Linux..."
	@mkdir -p $(LINUX_BINARY_DIR)
	GOOS=$(GOOS_LINUX) GOARCH=$(GOARCH) $(GOBUILD) -o $(LINUX_BINARY_DIR)/logger ./cmd/logger

linux-server:
	@echo "Building server for Linux..."
	@mkdir -p $(LINUX_BINARY_DIR)
	GOOS=$(GOOS_LINUX) GOARCH=$(GOARCH) $(GOBUILD) -o $(LINUX_BINARY_DIR)/server ./cmd/server

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	$(GOCLEAN)
	@rm -f monitor reconstruct analyze log-analyze extract log-extract top-contracts logger server
	@rm -rf $(BINARY_DIR)
	@echo "Clean complete"

# Clean only Linux binaries
.PHONY: clean-linux
clean-linux:
	@echo "Cleaning Linux binaries..."
	@rm -rf $(LINUX_BINARY_DIR)
	@echo "Linux binaries cleaned"

# Test
.PHONY: test
test:
	$(GOTEST) -v ./...

# Get dependencies
.PHONY: deps
deps:
	$(GOGET) -d -v
	$(GOCMD) mod tidy

# Help
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  all              - Build all commands for current OS (default)"
	@echo "  linux            - Build all commands for Linux (individual targets)"
	@echo "  linux-all        - Build all commands for Linux in bin/linux/jax-ov/ directory"
	@echo "  <command>        - Build specific command (e.g., 'make monitor')"
	@echo "  linux-<command>  - Build specific command for Linux (e.g., 'make linux-monitor')"
	@echo "  clean            - Remove all build artifacts"
	@echo "  clean-linux      - Remove only Linux binaries"
	@echo "  test             - Run tests"
	@echo "  deps             - Download and tidy dependencies"
	@echo "  help             - Show this help message"
	@echo ""
	@echo "Available commands:"
	@echo "  $(COMMANDS)"

