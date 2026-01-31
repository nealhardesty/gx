# gx - CLI assistant for shell command generation
# Makefile for build, test, and development tasks

VERSION=$(shell grep 'const Version' internal/version/version.go | cut -d'"' -f2)
GOFLAGS=-ldflags="-s -w"

# Detect OS and set binary name accordingly
GOOS=$(shell go env GOOS)
ifeq ($(GOOS),windows)
	GX_BINARY=gx.exe
	GXX_BINARY=gxx.exe
else
	GX_BINARY=gx
	GXX_BINARY=gxx
endif

.PHONY: all build build-gx build-gxx test run clean lint fmt tidy help version install

## all: Build both binaries (default target)
all: build

## build: Compile both gx and gxx binaries
build: build-gx build-gxx

## build-gx: Compile the gx binary
build-gx:
	go build $(GOFLAGS) -o $(GX_BINARY) .

## build-gxx: Compile the gxx binary
build-gxx:
	go build $(GOFLAGS) -o $(GXX_BINARY) ./cmd/gxx

## test: Run all tests with race detection
test:
	go test -race -v ./...

## run: Build and run the application (use ARGS="your prompt" to pass arguments)
run: build-gx
	./$(GX_BINARY) $(ARGS)

## clean: Remove build artifacts
clean:
	rm -f $(GX_BINARY) $(GXX_BINARY)
	rm -f $(GX_BINARY).exe $(GXX_BINARY).exe

## lint: Run linters (go vet)
lint:
	go vet ./...

## fmt: Format code with gofmt and goimports
fmt:
	gofmt -s -w .
	@command -v goimports >/dev/null 2>&1 && goimports -w . || echo "goimports not installed, skipping"

## tidy: Run go mod tidy to clean up dependencies
tidy:
	go mod tidy

## version: Display current version
version:
	@echo "gx version $(VERSION)"

## install: Install both gx and gxx binaries to GOPATH/bin
install:
	go install $(GOFLAGS) .
	go install $(GOFLAGS) ./cmd/gxx

## help: Show this help message
help:
	@echo "gx - CLI assistant for shell command generation"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
