# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Changed
- **2026-01-31**: Rewrote `README.md` — compressed redundant content, added ASCII architecture diagram, organized sections for clarity (Installation, Usage, Options, Storage, Tools, Configuration, Technical Details)

### Added
- **2025-01-31**: Added Versioning section to `.cursorrules` requiring `version.go` with semantic versioning, and Makefile targets (`make version`, `make version-increment`, `make release`)
- **2025-01-31**: Added Makefile section to `.cursorrules` requiring every project to have a Makefile with standard targets (build, test, run, clean, lint, fmt, tidy, help)

### Changed
- **2025-01-31**: Rewrote `.cursorrules` from Python3 to Golang development guidelines
  - Added new "Think Hard" section emphasizing careful consideration and asking clarifying questions before implementation
  - Updated all language-specific guidelines for Go (code style, error handling, testing, concurrency, etc.)
  - Retained README.md and CHANGELOG.md documentation requirements
  - Added Go-specific sections for package organization, concurrency, and Go module management
