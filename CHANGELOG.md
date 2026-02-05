# Changelog

All notable changes to this project will be documented in this file.

## [0.1.0] - 2026-01-31

### Added
- **Initial Release**: Full implementation of gx CLI assistant
- `main.go` — Entry point with CLI flag parsing (-x, -y, -v, -c, -n, --version)
- `version.go` — Semantic versioning (v0.1.0)
- `go.mod` — Go module with Vertex AI SDK dependency
- `Makefile` — Build automation (build, test, run, clean, lint, fmt, tidy, help, version, install)
- `internal/gemini/client.go` — Vertex AI Gemini client with tool integration, shell/platform detection, system instruction generation
- `internal/history/history.go` — JSON-based history management for ~/.gxhistory and command staging to ~/.gx
- `internal/tools/registry.go` — Tool registration and function call dispatch for LLM
- `internal/tools/files.go` — File system tools (pwd, ls, ls -R, stat, cat)
- `internal/tools/process.go` — Process tools (ps, uptime) with cross-platform support

### Features
- Natural language to shell command conversion using Google Vertex AI Gemini
- Context-aware follow-up prompts via conversation history
- Command staging to `~/.gx` with execute via `-x` flag
- YOLO mode (`-y`) for immediate execution
- Verbose mode (`-v`) for detailed command comments
- LLM tools for file system and process context gathering
- Cross-platform support (Windows PowerShell, bash, zsh, macOS, Linux, WSL2)
- Shell-aware output formatting (correct comment syntax per shell)

## [Unreleased]

### Added
- **2026-02-05**: Added stdin input support with `-` command-line option — when `-` is passed as a standalone argument, gx will read from stdin and append it to the prompt before sending. This enables piping file contents or command output directly into prompts (e.g., `cat error.log | gx - "explain this error"` or `docker ps | gx -`)
- **2026-01-31**: Added `gx.png` logo to README.md — incorporated project logo at the top of the documentation
- **2026-01-31**: Added `gxx` command shortcut — automatically includes `-y` flag (YOLO mode) for immediate generation and execution. Both `gx` and `gxx` binaries are now built and installed together.
- **2026-01-31**: Refactored CLI logic into `internal/cli` package — extracted all shared logic from `main.go` to eliminate code duplication between `gx` and `gxx` commands. Main packages are now thin wrappers that delegate to shared library code.
- **2026-01-31**: Moved version constant to `internal/version` package — enables both `gx` and `gxx` to share the same version without duplication.
- **2026-01-31**: Updated Makefile — now builds both `gx` and `gxx` binaries, and `make install` installs both commands. `go install ./...` will also install both binaries.

### Changed
- **2026-01-31**: Updated `.cursorrules` — added DRY (Don't Repeat Yourself) as a critical requirement in the Code Quality section, emphasizing that code duplication is never acceptable and shared logic must be extracted to reusable packages.

### Fixed
- **2026-01-31**: Fixed shell detection in `internal/gemini/client.go` — PowerShell is now correctly detected when running in PowerShell by checking `PSModulePath` before `ComSpec` (which is often set even in PowerShell sessions)
- **2026-01-31**: Enhanced system instruction in `internal/gemini/client.go` — Added explicit warning at the top of instructions to NEVER use REM comments for PowerShell (REM is only for CMD), ensuring the LLM uses `#` for PowerShell comments
- **2026-01-31**: Fixed exit code propagation in `main.go` — When executing with `-x` or `-y` flags, the program now returns the same exit code as the subprocess, ensuring proper error handling in scripts and pipelines. Stdout and stderr are properly streamed to the parent process.

### Added
- **2026-01-31**: Added prompt debugging feature in `internal/gemini/client.go` — When `GX_PROMPT_OUTPUT` environment variable is set (defaults to `~/.gxprompt`), all prompts sent to the LLM are logged to the specified file, including system instructions, history context, user prompts, tool calls, and responses, separated by `---` between turns
- **2026-01-31**: Added `-p` flag in `main.go` — Print the prompt that would be sent to the LLM without actually sending it, useful for debugging and understanding what instructions are being sent

### Changed
- **2026-01-31**: Updated default Gemini model from `gemini-1.5-flash` to `gemini-2.5-flash-lite` in `internal/gemini/client.go`, `main.go` usage text, and `README.md` documentation
- **2026-01-31**: Updated `Makefile` — build target now automatically detects OS and creates `gx.exe` on Windows and `gx` on Linux/Mac/etc
- **2026-01-31**: Updated CLI help output (`main.go`) — added GCP Setup section showing required gcloud commands
- **2026-01-31**: Updated `README.md` — added clear GCP project setup instructions with `gcloud config set project` command, added note about common "no project ID specified" error
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
