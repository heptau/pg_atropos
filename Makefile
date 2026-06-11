.PHONY: help build install test lint clean all release check-gh

VERSION := $(shell cat VERSION 2>/dev/null || echo "0.0.0")
BIN_DIR := binaries
PKG := pg_atropos
FORMULA_NAME := pg-atropos
TAP_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))../homebrew-tap

PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64

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
	@echo "  make release         Create a new release (version prompt, build, gh release, homebrew)"
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
		[ "$$GOOS" = "windows" ] && output="$$output.exe"; \
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
	rm -rf $(BIN_DIR) dist
	rm -f coverage.out

check-gh:
	@command -v gh >/dev/null 2>&1 || { echo "Error: GitHub CLI (gh) is required. Install: brew install gh"; exit 1; }

release: test check-gh
	@current="$(VERSION)"; \
	echo "Current version: $$current"; \
	read -p "New version (leave empty to keep $$current): " v; \
	new=$${v:-$$current}; \
	if [ "$$new" != "$$current" ]; then \
		echo "$$new" > VERSION; \
		sed -i '' 's/var version = ".*"/var version = "'"$$new"'"/' main.go; \
		git add VERSION main.go; \
		git commit -m "Release v$$new"; \
		git tag "v$$new"; \
		git push origin main --tags; \
	fi; \
	$(MAKE) build-all; \
	rm -rf dist; \
	mkdir -p dist; \
	for platform in $(PLATFORMS); do \
		goos=$${platform%/*}; \
		goarch=$${platform#*/}; \
		binary="$(BIN_DIR)/$(PKG)-$$new-$$goos-$$goarch"; \
		[ "$$goos" = "windows" ] && binary="$$binary.exe"; \
		archive="dist/$(PKG)-$$new-$$goos-$$goarch.tar.gz"; \
		tmpdir=$$(mktemp -d); \
		cp "$$binary" "$$tmpdir/$(PKG)"; \
		COPYFILE_DISABLE=1 tar czf "$$archive" -C "$$tmpdir" "$(PKG)"; \
		rm -rf "$$tmpdir"; \
		shasum -a 256 "$$archive" | awk '{print $$1}' > "$$archive.sha256"; \
		echo "  Created $$archive"; \
	done; \
	echo "Creating GitHub release v$$new..."; \
	gh release create "v$$new" dist/$(PKG)-$$new-*.tar.gz dist/$(PKG)-$$new-*.sha256 --title "v$$new" --generate-notes; \
	echo "Generating Homebrew formula..."; \
	sha_darwin_arm64=$$(cat dist/$(PKG)-$$new-darwin-arm64.tar.gz.sha256); \
	sha_darwin_amd64=$$(cat dist/$(PKG)-$$new-darwin-amd64.tar.gz.sha256); \
	sha_linux_arm64=$$(cat dist/$(PKG)-$$new-linux-arm64.tar.gz.sha256); \
	sha_linux_amd64=$$(cat dist/$(PKG)-$$new-linux-amd64.tar.gz.sha256); \
	formula="$(TAP_DIR)/Formula/$(FORMULA_NAME).rb"; \
	{ \
		echo "class PgAtropos < Formula"; \
		echo "  desc \"PostgreSQL custom-format dump splitter for GIT\""; \
		echo "  homepage \"https://github.com/heptau/pg_atropos\""; \
		echo "  version \"$$new\""; \
		echo "  license \"MIT\""; \
		echo ""; \
		echo "  on_macos do"; \
		echo "    if Hardware::CPU.arm?"; \
		echo "      url \"https://github.com/heptau/pg_atropos/releases/download/v$$new/$(PKG)-$$new-darwin-arm64.tar.gz\""; \
		echo "      sha256 \"$$sha_darwin_arm64\""; \
		echo "    else"; \
		echo "      url \"https://github.com/heptau/pg_atropos/releases/download/v$$new/$(PKG)-$$new-darwin-amd64.tar.gz\""; \
		echo "      sha256 \"$$sha_darwin_amd64\""; \
		echo "    end"; \
		echo "  end"; \
		echo ""; \
		echo "  on_linux do"; \
		echo "    if Hardware::CPU.arm?"; \
		echo "      url \"https://github.com/heptau/pg_atropos/releases/download/v$$new/$(PKG)-$$new-linux-arm64.tar.gz\""; \
		echo "      sha256 \"$$sha_linux_arm64\""; \
		echo "    else"; \
		echo "      url \"https://github.com/heptau/pg_atropos/releases/download/v$$new/$(PKG)-$$new-linux-amd64.tar.gz\""; \
		echo "      sha256 \"$$sha_linux_amd64\""; \
		echo "    end"; \
		echo "  end"; \
		echo ""; \
		echo "  def install"; \
		echo "    bin.install \"$(PKG)\""; \
		echo "  end"; \
		echo ""; \
		echo "  test do"; \
		echo "    assert_match \"version\", shell_output(\"#{bin}/$(PKG) --help\")"; \
		echo "  end"; \
		echo "end"; \
	} > "$$formula"; \
	echo "Formula written to $$formula"; \
	cd "$(TAP_DIR)" && git pull --rebase origin main && git add "Formula/$(FORMULA_NAME).rb" && git commit -m "Brew formula update for $(FORMULA_NAME) version v$$new" && git push origin main; \
	echo "=========================================================="; \
	echo "Release v$$new complete!"; \
	echo "=========================================================="
