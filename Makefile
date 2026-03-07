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

.PHONY: all build build-gx build-gxx test run clean lint fmt tidy help version version-increment install push update-llm

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

## version-increment: Bump the patch version in internal/version/version.go
version-increment:
	@VERSION=$$(grep 'Version = ' internal/version/version.go | sed 's/.*"\(.*\)".*/\1/') && \
	MAJOR=$$(echo $$VERSION | cut -d. -f1) && \
	MINOR=$$(echo $$VERSION | cut -d. -f2) && \
	PATCH=$$(echo $$VERSION | cut -d. -f3) && \
	NEW_PATCH=$$((PATCH + 1)) && \
	NEW_VERSION="$$MAJOR.$$MINOR.$$NEW_PATCH" && \
	sed -i "s/Version = \"$$VERSION\"/Version = \"$$NEW_VERSION\"/" internal/version/version.go && \
	echo "Bumped $$VERSION → $$NEW_VERSION"

## push: Bump patch version, run checks, commit, push, and tag
push: fmt tidy build test
	@VERSION=$$(grep 'Version = ' internal/version/version.go | sed 's/.*"\(.*\)".*/\1/') && \
	MAJOR=$$(echo $$VERSION | cut -d. -f1) && \
	MINOR=$$(echo $$VERSION | cut -d. -f2) && \
	PATCH=$$(echo $$VERSION | cut -d. -f3) && \
	NEW_PATCH=$$((PATCH + 1)) && \
	NEW_VERSION="$$MAJOR.$$MINOR.$$NEW_PATCH" && \
	sed -i "s/Version = \"$$VERSION\"/Version = \"$$NEW_VERSION\"/" internal/version/version.go && \
	echo "Bumped $$VERSION → $$NEW_VERSION" && \
	git add -A && \
	git commit -m "release: v$$NEW_VERSION.  $$(gitsum)" && \
	git push && \
	git tag v$$NEW_VERSION && \
	git push --tags && \
	echo "Released v$$NEW_VERSION"

## update-llm: Pull latest easy-llm-wrapper directly from VCS (bypasses proxy cache)
update-llm:
	GOPROXY=direct go get github.com/nealhardesty/easy-llm-wrapper@latest
	go mod tidy

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
