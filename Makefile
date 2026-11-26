GO ?= $(shell command -v go 2>/dev/null || printf '%s' "$(HOME)/.local/share/go/bin/go")
CPA_BINARY ?= $(CURDIR)/dist/cpa/cli-proxy-api
SOAK_SECONDS ?= 120
TARGET_OS ?= linux
TARGET_ARCH ?= amd64
LIB_EXT = $(if $(filter windows,$(TARGET_OS)),dll,$(if $(filter darwin,$(TARGET_OS)),dylib,so))
LIBRARY ?= dist/quota-first-router.$(LIB_EXT)
BUILD_FLAGS ?= $(if $(filter amd64,$(TARGET_ARCH)),-O2 -march=x86-64 -mtune=generic,-O2)
RELEASE_DIR ?= dist/release
.PHONY: test build integration-test soak-test package cpa matrix version clean

test:
	$(GO) test -race -count=1 ./...
	$(GO) vet ./...
	@test -z "$$($(GO) fmt ./...)" || (echo 'Run gofmt and rerun'; exit 1)
	python3 -m unittest discover -s tests -p 'test_*.py'

build:
	mkdir -p $(dir $(LIBRARY))
	CGO_ENABLED=1 GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) GOAMD64=v1 GOARM64=v8.0 CGO_CFLAGS='$(BUILD_FLAGS)' $(GO) build -trimpath -buildvcs=false -buildmode=c-shared -o $(LIBRARY) .

integration-test: build
	python3 tests/host_integration.py --cpa $(CPA_BINARY) --library dist/quota-first-router.so

soak-test: build
	SOAK_SECONDS=$(SOAK_SECONDS) python3 tests/host_integration.py --cpa $(CPA_BINARY) --library dist/quota-first-router.so

package: build
	python3 scripts/release.py package --goos $(TARGET_OS) --goarch $(TARGET_ARCH) --library $(LIBRARY) --output $(RELEASE_DIR)

cpa:
	python3 scripts/release.py cpa --goos $(TARGET_OS) --goarch $(TARGET_ARCH)

matrix:
	python3 scripts/release.py matrix --output $(RELEASE_DIR)

version:
	@python3 scripts/release.py version

clean:
	rm -rf dist
