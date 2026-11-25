GO ?= $(shell command -v go 2>/dev/null || printf '%s' "$(HOME)/.local/share/go/bin/go")
CPA_BINARY ?= $(CURDIR)/dist/cpa/cli-proxy-api
SOAK_SECONDS ?= 120
.PHONY: test build integration-test soak-test package cpa version clean

test:
	$(GO) test -race -count=1 ./...
	$(GO) vet ./...
	@test -z "$$($(GO) fmt ./...)" || (echo 'Run gofmt and rerun'; exit 1)
	python3 -m unittest discover -s tests -p 'test_*.py'

build:
	mkdir -p dist
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 GOAMD64=v1 CGO_CFLAGS='-O2 -march=x86-64 -mtune=generic' $(GO) build -trimpath -buildvcs=false -buildmode=c-shared -o dist/quota-first-router.so .

integration-test: build
	python3 tests/host_integration.py --cpa $(CPA_BINARY) --library dist/quota-first-router.so

soak-test: build
	SOAK_SECONDS=$(SOAK_SECONDS) python3 tests/host_integration.py --cpa $(CPA_BINARY) --library dist/quota-first-router.so

package: build
	python3 scripts/release.py package

cpa:
	python3 scripts/release.py cpa

version:
	@python3 scripts/release.py version

clean:
	rm -rf dist
