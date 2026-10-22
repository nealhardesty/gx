# gx - CLI assistant for shell command generation
# Makefile for build, test, and development tasks

VERSION=$(shell grep 'const Version' internal/version/version.go | cut -d'"' -f2)
# Not named GOFLAGS: make exports overridden environment variables, and go
# cannot parse GOFLAGS=-ldflags="-s -w" (breaks every target when CI sets GOFLAGS).
LDFLAGS := -ldflags="-s -w"
# The race detector needs cgo and a C compiler; clear it with `make test RACE=`.
RACE ?= -race

# Detect OS and set binary name accordingly
GOOS=$(shell go env GOOS)
ifeq ($(GOOS),windows)
	GX_BINARY=gx.exe
	GXX_BINARY=gxx.exe
else
	GX_BINARY=gx
	GXX_BINARY=gxx
endif

.PHONY: all build build-gx build-gxx test check run clean lint fmt tidy help version version-increment install push update-llm

# Running bare `make` lists the targets.
.DEFAULT_GOAL := help

## all: Build both binaries
all: build

## build: Compile both gx and gxx binaries
build: build-gx build-gxx

## build-gx: Compile the gx binary
build-gx:
	go build $(LDFLAGS) -o $(GX_BINARY) .

## build-gxx: Compile the gxx binary
build-gxx:
	go build $(LDFLAGS) -o $(GXX_BINARY) ./cmd/gxx

## test: Run all tests (race detector on; RACE= to disable)
test:
	go test $(RACE) -v ./...

## run: Build and run the application (use ARGS="your prompt" to pass arguments)
run: build-gx
	./$(GX_BINARY) $(ARGS)

## clean: Remove build artifacts
clean:
	rm -f $(GX_BINARY) $(GXX_BINARY)
	rm -f $(GX_BINARY).exe $(GXX_BINARY).exe

## check: Verify formatting, vet, and run tests (what CI should run)
check: lint test
	@UNFORMATTED=$$(gofmt -s -l .) && if [ -n "$$UNFORMATTED" ]; then \
		echo "gofmt needed:"; echo "$$UNFORMATTED"; exit 1; fi

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

# Shell snippet shared by version-increment and push. push must not call
# $(MAKE): make runs any recipe line containing $(MAKE) even under `make -n`,
# which would turn a dry run of push into a real commit.
BUMP_VERSION = VERSION=$$(grep 'Version = ' internal/version/version.go | sed 's/.*"\(.*\)".*/\1/') && \
	MAJOR=$$(echo $$VERSION | cut -d. -f1) && \
	MINOR=$$(echo $$VERSION | cut -d. -f2) && \
	PATCH=$$(echo $$VERSION | cut -d. -f3) && \
	NEW_VERSION="$$MAJOR.$$MINOR.$$((PATCH + 1))" && \
	sed -i.bak "s/Version = \"$$VERSION\"/Version = \"$$NEW_VERSION\"/" internal/version/version.go && \
	rm -f internal/version/version.go.bak && \
	echo "Bumped $$VERSION → $$NEW_VERSION"

## version-increment: Bump the patch version in internal/version/version.go
version-increment:
	@$(BUMP_VERSION)

## push: Run checks, commit (message from gitsum), bump patch version, push, and tag
push: fmt tidy build test
	@command -v gitsum >/dev/null 2>&1 || { echo "push: gitsum not found on PATH"; exit 1; }
	@git add -A && \
	MSG=$$(gitsum) && [ -n "$$MSG" ] || { echo "push: gitsum produced no commit message; nothing committed"; exit 1; }; \
	$(BUMP_VERSION) && \
	git add internal/version/version.go && \
	git commit -m "release: v$$NEW_VERSION.  $$MSG" && \
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
	go install $(LDFLAGS) .
	go install $(LDFLAGS) ./cmd/gxx

## help: Show this help message
help:
	@echo "gx - CLI assistant for shell command generation"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
