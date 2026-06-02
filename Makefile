.PHONY: help build install test lint clean all

VERSION := $(shell cat VERSION 2>/dev/null || echo "0.0.0")
BIN_DIR := binaries
PKG := pg_atropos

PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64

help:
	@echo "pg_atropos v$(VERSION) - PostgreSQL dump file splitter"
	@echo ""
	@echo "Usage:"
	@echo "  make build           Build binary for current platform"
	@echo "  make install         Build and install to \$$GOPATH/bin"
	@echo "  make all             Build binaries for all platforms"
	@echo "  make test            Run tests"
	@echo "  make coverage        Run tests with coverage report"
	@echo "  make lint            Run linters (golangci-lint)"
	@echo "  make clean           Remove built artifacts"
	@echo "  make help            Show this help"

build:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -ldflags "-X main.version=$(VERSION)" -o $(BIN_DIR)/$(PKG)-$(VERSION)

install: build
	@mkdir -p "$(shell go env GOPATH)/bin"
	cp $(BIN_DIR)/$(PKG)-$(VERSION) "$(shell go env GOPATH)/bin/$(PKG)"
	@echo "Installed $(PKG) v$(VERSION) to $(shell go env GOPATH)/bin/$(PKG)"

all: test build-all

build-all:
	@mkdir -p $(BIN_DIR)
	@for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*}; \
		GOARCH=$${platform#*/}; \
		output="$(BIN_DIR)/$(PKG)-$(VERSION)-$$GOOS-$$GOARCH"; \
		echo "Building for $$GOOS/$$GOARCH..."; \
		CGO_ENABLED=0 GOOS=$$GOOS GOARCH=$$GOARCH go build -ldflags "-X main.version=$(VERSION)" -o "$$output"; \
	done

test:
	go test -v ./...

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	rm coverage.out

LINT := $(shell command -v golangci-lint 2>/dev/null || echo "$(shell go env GOPATH)/bin/golangci-lint")

lint:
	@if ! test -x "$(LINT)" > /dev/null 2>&1; then \
		echo "Install golangci-lint: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi
	"$(LINT)" run ./...

clean:
	rm -rf $(BIN_DIR)
	rm -f coverage.out
