# gx - CLI assistant for shell command generation
# Makefile for build, test, and development tasks

VERSION=$(shell grep 'const Version' version.go | cut -d'"' -f2)
GOFLAGS=-ldflags="-s -w"

# Detect OS and set binary name accordingly
GOOS=$(shell go env GOOS)
ifeq ($(GOOS),windows)
	BINARY_NAME=gx.exe
else
	BINARY_NAME=gx
endif

.PHONY: all build test run clean lint fmt tidy help version install

## all: Build the binary (default target)
all: build

## build: Compile the project
build:
	go build $(GOFLAGS) -o $(BINARY_NAME) .

## test: Run all tests with race detection
test:
	go test -race -v ./...

## run: Build and run the application (use ARGS="your prompt" to pass arguments)
run: build
	$(BINARY_NAME) $(ARGS)

## clean: Remove build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME).exe

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

## install: Install the binary to GOPATH/bin
install:
	go install $(GOFLAGS) .

## help: Show this help message
help:
	@echo "gx - CLI assistant for shell command generation"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
