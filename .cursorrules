# Golang Cursor Rules

## Think Hard
**CRITICAL: Think deeply before taking action**
- STOP and THINK before making any changes or writing any code
- Ask clarifying questions if requirements are unclear or ambiguous
- Do not assume - if something is not explicitly stated, ASK
- Consider multiple approaches and their trade-offs before implementing
- If you're uncertain about the best path forward, discuss options with the user
- Never proceed with incomplete understanding - clarity first, code second
- Validate your understanding by summarizing the task back before implementation

## Workflow and Planning
**CRITICAL: Always read the current state of the project before planning any changes**
- Before making any modifications, ALWAYS read and understand the current state of relevant files
- Use codebase search and file reading tools to gather context about existing code
- Review related files, dependencies, and project structure before implementing changes
- Understand the full context of what you're modifying, including:
  - Existing implementations and patterns
  - Dependencies and imports
  - Related packages and functions
  - Current architecture and design decisions
- Think deeply about the implications of changes:
  - Consider edge cases and potential side effects
  - Analyze how changes will interact with existing code
  - Evaluate alternative approaches before committing to a solution
  - Consider long-term maintainability and scalability
  - Think about backward compatibility if applicable
- Never make assumptions about the codebase - always verify by reading the actual code
- Plan changes thoroughly before implementation, considering the full impact

## Language and Version
- Use Go 1.21+ syntax and features
- Leverage generics where appropriate (Go 1.18+)
- Use the standard library whenever possible before reaching for third-party packages
- Follow the official Go style guide and conventions

## Code Style
- Use `gofmt` / `goimports` for formatting (tabs for indentation)
- Follow Effective Go guidelines
- Use descriptive variable and function names
- Use `camelCase` for unexported identifiers
- Use `PascalCase` for exported identifiers
- Use `UPPER_CASE` with underscores for constants only when truly constant and package-level
- Keep functions short and focused on a single responsibility
- Prefer early returns to reduce nesting

## Package Organization
- Use meaningful package names (short, lowercase, no underscores)
- Avoid package name collisions with standard library
- Group related functionality into cohesive packages
- Use `internal/` for packages not meant for external use
- Keep `main` packages minimal - delegate to library packages

## Imports
- Group imports: standard library, third-party, local packages
- Use `goimports` to manage import organization automatically
- Avoid dot imports except in tests where appropriate
- Prefer explicit imports over blank identifier imports unless necessary for side effects

## Error Handling
- Always handle errors explicitly - never ignore with `_`
- Use `errors.New()` or `fmt.Errorf()` for error creation
- Wrap errors with context using `fmt.Errorf("context: %w", err)`
- Use custom error types when additional context is needed
- Check errors immediately after the call that may return them
- Use `errors.Is()` and `errors.As()` for error comparison
- Consider using sentinel errors for expected error conditions

## Documentation
- Write doc comments for all exported functions, types, and packages
- Start doc comments with the name of the element being documented
- Use complete sentences in documentation
- Include examples in `_test.go` files using `Example` functions
- Add inline comments for complex logic
- Document non-obvious behavior and edge cases

## Testing
- Write unit tests for all new functionality
- Use the standard `testing` package
- Name test files with `_test.go` suffix
- Use table-driven tests for comprehensive coverage
- Use `t.Helper()` in test helper functions
- Use `testify` or similar for assertions if it improves readability
- Aim for high test coverage on critical paths
- Use `go test -race` to detect race conditions

## Dependencies
- Use Go modules (`go.mod` and `go.sum`) for dependency management
- Pin dependency versions for reproducibility
- Minimize external dependencies - prefer the standard library
- Vet dependencies for maintenance status and security
- Run `go mod tidy` to clean up unused dependencies
- Document why each significant dependency is needed
- Installs via 'go install github.com/.../...@latest` should always be available (and always documented)

## Makefile
**CRITICAL: Every project MUST have a Makefile**
- Always include a `Makefile` in the project root to control build, test, run, and utility functions
- Standard targets that should be present:
  - `make build` - Compile the project
  - `make test` - Run all tests (include `-race` flag)
  - `make run` - Build and run the application
  - `make clean` - Remove build artifacts
  - `make lint` - Run linters (`golangci-lint`, `go vet`, etc.)
  - `make fmt` - Format code with `gofmt`/`goimports`
  - `make tidy` - Run `go mod tidy`
- Add project-specific utility targets as needed (e.g., `make docker`, `make migrate`, `make generate`)
- Include a `make help` target that lists all available targets with descriptions
- Use `.PHONY` declarations for targets that don't produce files
- Keep the Makefile well-organized and commented
- Makefile should be the primary interface for common development tasks

## Versioning
**CRITICAL: Every project MUST have a `version.go` file**
- Always include a `version.go` file in the project root or main package to track the current version
- Use Semantic Versioning (SemVer) format: `MAJOR.MINOR.PATCH` (e.g., `1.2.3`)
  - **MAJOR**: Incremented for incompatible API changes
  - **MINOR**: Incremented for new functionality in a backward-compatible manner
  - **PATCH**: Incremented for backward-compatible bug fixes
- The `version.go` file should export a `Version` constant or variable accessible to the application
- Example structure:
  ```go
  package main
  
  // Version is the current semantic version of the application
  const Version = "0.1.0"
  ```
- Version should be displayed via `--version` or `-v` flag in CLI applications
- Makefile must include version management targets:
  - `make version` - Display current version
  - `make version-increment` - Increment patch version (or prompt for major/minor/patch)
  - `make release` - Create a release (tag, build, changelog update)
- Keep version in sync with git tags when releasing
- Update CHANGELOG.md when version changes

## Code Quality
**CRITICAL: DRY (Don't Repeat Yourself) is an absolute requirement**
- Never duplicate code - extract shared logic into reusable packages, functions, or types
- When creating multiple entry points (e.g., cmd/gx and cmd/gxx), extract all shared logic into internal packages
- Main packages should be thin wrappers that delegate to shared library code
- If you find yourself copying code between files, stop and refactor to a shared location
- Use interfaces for abstraction and testability
- Keep interfaces small and focused (Go proverb: "The bigger the interface, the weaker the abstraction")
- Use structs for data containers
- Use `context.Context` for cancellation and request-scoped values
- Avoid global state - use dependency injection
- Use `defer` for cleanup operations
- Prefer composition over inheritance

## Concurrency
- Use goroutines and channels appropriately
- Always ensure goroutines can exit (avoid goroutine leaks)
- Use `sync.WaitGroup` to wait for goroutine completion
- Use `sync.Mutex` or `sync.RWMutex` for shared state protection
- Prefer channels for communication, mutexes for state
- Use `context.Context` for cancellation propagation
- Run tests with `-race` flag to detect data races

## Performance
- Profile before optimizing (`pprof`, benchmarks)
- Avoid premature optimization
- Use benchmarks (`func BenchmarkXxx`) to measure performance
- Consider memory allocations in hot paths
- Use `sync.Pool` for frequently allocated objects
- Prefer stack allocation over heap when possible

## Security
- Never hardcode secrets or API keys
- Use environment variables or secure configuration management
- Validate and sanitize all user inputs
- Use parameterized queries for database operations
- Be cautious with `unsafe` package - avoid unless absolutely necessary
- Use `crypto/rand` for cryptographic randomness, not `math/rand`

## README.md
- README.md file must be kept up to date with the base design at all times

## Git and Version Control
- Write clear, descriptive commit messages
- Keep commits focused and atomic
- Use meaningful branch names
- **MANDATORY**: Update `CHANGELOG.md` before every commit/checkin
- Never commit changes without updating `CHANGELOG.md` first

## CHANGELOG.md Documentation Requirement
**CRITICAL: CHANGELOG.md must ALWAYS be updated before checkin and for ALL changes**
- The `CHANGELOG.md` file MUST be updated for EVERY change made to the codebase, whether by humans or AI agents
- Update `CHANGELOG.md` BEFORE committing changes or creating pull requests
- Document all changes including:
  - New features and functionality
  - Bug fixes and patches
  - Configuration changes
  - Dependency updates
  - Documentation updates
  - Refactoring and code improvements
  - Breaking changes
- Use clear, descriptive entries that explain what changed and why
- Group changes by date or version as appropriate
- This is a mandatory step that must not be skipped

## README Documentation Requirement
**CRITICAL: All changes must be documented in README.md**
- After making any significant changes to the codebase, update the README.md file
- Document new features, bug fixes, configuration changes, and dependency updates
- Include a "Changelog" or "Updates" section in the README.md
- Update installation instructions if dependencies change
- Update usage examples if functionality changes
- Keep the README.md current and comprehensive

## Project-Specific Guidelines
- This is a personal project: prioritize working solutions over perfect code
- However, maintain code quality and readability
- Document architectural decisions in the README.md
- Keep the codebase organized and maintainable
