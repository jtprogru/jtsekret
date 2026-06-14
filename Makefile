.PHONY: build test lint clean release install help

.DEFAULT_GOAL := help

BINARY_DIR=dist
BINARY_NAME=jtsekret
OUTPUT_NAME=$(BINARY_DIR)/$(BINARY_NAME)
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GO=go
GOFLAGS=-v
LDFLAGS=-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)

build:
	$(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(OUTPUT_NAME) .

build-linux:
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(OUTPUT_NAME)-linux-amd64 .

build-darwin:
	GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(OUTPUT_NAME)-darwin-arm64 .

test:
	$(GO) test $(GOFLAGS) -race -covermode=atomic -coverprofile=coverage.out ./...

test-unit:
	$(GO) test $(GOFLAGS) -v -race ./...

test-integration:
	$(GO) test $(GOFLAGS) -v -tags=integration ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .
	goimports -w .

vet:
	$(GO) vet ./...

clean:
	rm -rf $(BINARY_DIR)/*
	rm -f coverage.out

install:
	$(GO) install -ldflags="$(LDFLAGS)"

release:
	goreleaser release --clean

release-dry:
	goreleaser release --clean --skip-publish --skip-validate

tidy:
	$(GO) mod tidy
	$(GO) mod verify

deps:
	$(GO) mod download

help:
	@echo "Available targets:"
	@echo "  build            - Build binary for current platform"
	@echo "  build-linux      - Build for Linux amd64"
	@echo "  build-darwin     - Build for Darwin arm64"
	@echo "  test             - Run all tests with race detector"
	@echo "  test-unit        - Run unit tests"
	@echo "  test-integration - Run integration tests (requires credentials)"
	@echo "  lint             - Run linter"
	@echo "  fmt              - Format code"
	@echo "  vet              - Run go vet"
	@echo "  clean            - Remove built binaries"
	@echo "  install          - Install binary to \$$GOPATH/bin"
	@echo "  release          - Create release with goreleaser"
	@echo "  release-dry      - Dry run release"
	@echo "  tidy             - Tidy go modules"
	@echo "  deps             - Download dependencies"
