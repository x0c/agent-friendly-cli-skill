# Shared build script for the whole repository (six-platform cross-compilation
# + ldflags version injection).
# Each CLI's Makefile just declares three lines and includes this file:
#   COMMAND := mytool
#   VERSION_PKG := github.com/x0c/agent-friendly-cli-skill/cli-skeleton/internal/cmd
#   include ../build.mk
# Optional override: TEST_ENV (environment-variable prefix for go test, e.g. CI=true MYTOOL_NO_AUTO_UPDATE=1).
# build-all also generates dist/manifest.json (the basis for the self-update
# version guard and the pre-push check);
# make check-dist verifies that dist matches the current source and must pass
# before pushing.
COMMAND ?= $(error COMMAND is not declared)
VERSION_PKG ?= $(error VERSION_PKG is not declared)
TEST_ENV ?=
TARGETS := darwin-amd64 darwin-arm64 linux-amd64 linux-arm64 windows-amd64 windows-arm64
VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
BUILD_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X $(VERSION_PKG).version=$(VERSION) -X $(VERSION_PKG).buildCommit=$(BUILD_COMMIT) -X $(VERSION_PKG).buildTime=$(BUILD_TIME)

.PHONY: build test vet build-all check-dist clean

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(COMMAND) .

test:
	$(TEST_ENV) go test ./...

vet:
	go vet ./...

build-all:
	mkdir -p dist
	@for target in $(TARGETS); do \
		os=$${target%-*}; arch=$${target#*-}; ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(COMMAND)-$$target$$ext . || exit 1; \
	done
	@python3 ../scripts/gen-dist-manifest.py --command $(COMMAND) --version "$(VERSION)"

# Pre-push check: dist must match the current source (the .githooks/pre-push
# git hook calls the same script).
check-dist:
	@python3 ../scripts/check-dist.py --project $(notdir $(CURDIR))

clean:
	rm -f $(COMMAND)
