.PHONY: help setup build install compile test vet lint check

# help prints the list of available targets.
help:
	@echo "setup         Install dev tools"
	@echo "build         Build bl binary"
	@echo "install       Build and install to ~/.local/bin"
	@echo "compile       Type-check all packages"
	@echo "test          Run tests"
	@echo "vet           Run go vet"
	@echo "lint          Run golangci-lint"
	@echo "check         Vet, lint, and test"

# VERSION is the git description when available, else "dev". It is embedded in
# the binary so `bl version` reports a real build identity; a plain
# `go build ./...` (and the test suite) still reports "dev".
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/yantonov/backlogger/internal/cli.Version=$(VERSION)

# setup installs the development tools used by the check target. ## Install dev tools
setup:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# build compiles the bl binary with VERSION embedded into target/. CGO is off
# so the binary stays static even if a package later pulls in cgo-backed code. ## Build bl binary
build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o target/bl ./cmd/bl

# install builds bl and copies it to ~/.local/bin. ## Build and install to ~/.local/bin
install: build
	@mkdir -p "$$HOME/.local/bin"
	@cp target/bl "$$HOME/.local/bin/bl"
	@echo "target dir: $$HOME/.local/bin"
	@echo "bl installed successfully"

# compile type-checks every package without producing a binary. ## Type-check all packages
compile:
	go build ./...

# test runs the full test suite. ## Run tests
test:
	go test ./...

# vet runs the Go static analyzer. ## Run go vet
vet:
	go vet ./...

# lint runs golangci-lint. It is a hard failure when the tool is missing. ## Run golangci-lint
lint:
	golangci-lint run

# check is the single verification command: vet, lint, and test, in that order. ## Vet, lint, and test
check: vet lint test
	@echo "ALL CHECKS PASSED"
