SHELL := /usr/bin/env bash
APP   := loadout
BIN   := bin/$(APP)
PKG   := github.com/AxeForging/loadout
DIST  := dist

# Version — can be overridden from environment
ifeq ($(origin VERSION), environment)
else
  VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
endif

BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILT_BY   := $(shell whoami)

LDFLAGS := -s -w \
  -X $(PKG)/internal/build.Version=$(VERSION) \
  -X $(PKG)/internal/build.Commit=$(GIT_COMMIT) \
  -X $(PKG)/internal/build.Date=$(BUILD_TIME) \
  -X $(PKG)/internal/build.BuiltBy=$(BUILT_BY)

GOOS_ARCH := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

.PHONY: all build build-all run test test-integration lint fmt format tidy clean ci version tag

all: build

build:
	@mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/$(APP)
	@echo "Built $(BIN) ($(VERSION))"

build-all:
	@mkdir -p $(DIST)
	@echo "Building for all platforms..."
	@for t in $(GOOS_ARCH); do \
		os=$${t%/*}; arch=$${t#*/}; \
		bin_path=$(DIST)/$(APP)-$${os}-$${arch}; \
		echo "  $$os/$$arch → $$bin_path"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" -o $$bin_path ./cmd/$(APP); \
	done
	@echo "Done. Binaries in $(DIST)/"

run: build
	@$(BIN) $(ARGS)

test:
	go test -race -count=1 ./...

test-integration:
	cd integration && go test -v -tags integration -count=1 ./... -timeout 120s

lint:
	golangci-lint run --timeout=5m

fmt:
	go fmt ./...

format: fmt
	@command -v gofumpt >/dev/null 2>&1 && gofumpt -w . || echo "gofumpt not installed, skipping"
	@command -v goimports >/dev/null 2>&1 && goimports -w . || echo "goimports not installed, skipping"

tidy:
	go mod tidy

clean:
	rm -rf bin $(DIST)

ci: tidy lint test build

version:
	@echo "Version:    $(VERSION)"
	@echo "Commit:     $(GIT_COMMIT)"
	@echo "Build time: $(BUILD_TIME)"
	@echo "Built by:   $(BUILT_BY)"

tag:
	@if [ "$(VERSION)" = "dev" ]; then \
		echo "Error: Set VERSION env var (e.g., VERSION=v0.1.0 make tag)"; \
		exit 1; \
	fi
	git tag -a $(VERSION) -m "Release $(VERSION)"
	@echo "Tag created. Push with: git push origin $(VERSION)"
