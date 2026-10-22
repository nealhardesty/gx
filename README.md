# gx

![gx logo](gx.png)

A lightning-fast CLI assistant that converts natural language into executable shell commands using the Claude CLI, OpenRouter, or a local Ollama model via [easy-llm-wrapper](https://github.com/nealhardesty/easy-llm-wrapper).

**Zero fluff.** Returns raw shell code, not chatty explanations.

## Architecture

```
┌─────────────┐    ┌──────────────────┐    ┌──────────┐
│ User Prompt │───▶│ LLM              │───▶│ Stage to │
│  (+ stdin)  │    │ claude CLI,      │    │ ~/.gx    │
└─────────────┘    │ OpenRouter, or   │    └──────────┘
                   │ Ollama           │         │
                   └──────────────────┘         ▼
                                          ┌───────────┐
                                          │ Execute   │
                                          │ (-x / -y) │
                                          └───────────┘
```

**Generate → Stage → Execute** flow:
1. **Prompt** — User passes natural language to `gx`
2. **Inference** — Sent to the selected provider with a compact shell/platform-aware system prompt
3. **Stage** — Output saved to `~/.gx` for review
4. **Execute** — Run via `-x` (review first) or `-y` (YOLO mode)

Every run is a single, stateless request: no conversation history is kept. See [SPEC.md](SPEC.md) for the full specification.

## Installation

**Prerequisites:**
- [Go 1.21+](https://go.dev/)
- One LLM provider (see below)

**Pick a provider** — gx uses the first one available, in this order:

1. **Claude CLI** — if [`claude`](https://claude.com/claude-code) is on your `PATH`, it is used automatically. No further setup.
2. **OpenRouter (cloud):**
   ```bash
   export OPENROUTER_API_KEY=your_api_key_here
   ```
3. **Ollama (local):**
   ```bash
   # Install Ollama: https://ollama.com
   ollama pull gpt-oss:20b          # the default model; or pull any model and set GX_MODEL
   export OLLAMA_HOST=http://localhost:11434
   ```

> If `claude` is on your `PATH` it wins even when `OPENROUTER_API_KEY` or `OLLAMA_HOST` is set. See [Troubleshooting](#forcing-a-specific-provider) to override.

**Build from source:**
```bash
make build                       # builds ./gx and ./gxx
sudo mv gx gxx /usr/local/bin/   # Linux/macOS
```

**Or install directly:**
```bash
go install github.com/nealhardesty/gx@latest
go install github.com/nealhardesty/gx/cmd/gxx@latest

# Or from a checkout
make install
```

## Usage

```bash
# Generate a command
gx "find all large files over 100mb and sort by size"
# Output: find . -type f -size +100M -exec ls -lh {} + | sort -rh -k5

# Execute the staged command
gx -x

# Shortcut: gxx automatically includes -y flag (YOLO mode)
gxx "list docker containers"

# YOLO mode (generate and execute immediately)
gx -y "list docker containers"

# Read from stdin using '-' option
cat error.log | gx - "explain this error"
docker ps | gx - "create a kill command for these containers"
git diff | gx -  # Use stdin as entire prompt
```

> **Flags go before the prompt.** `gx "list files" -y` sends the text `list files -y` to the model and does *not* enable YOLO mode. Write `gx -y "list files"`.

## Options

| Flag | Description |
|------|-------------|
| `-` | Read additional input from stdin and append to prompt |
| `-x` | Execute command staged in `~/.gx` |
| `-y` | YOLO mode — execute immediately (no staging review) |
| `-v` | Verbose — include a short comment before each step |
| `-c` | Clear staged command |
| `-p` | Print the prompt (system + user message) that would be sent to the LLM; nothing is sent |
| `-D` | Debug mode — dump all activity to stderr, each line prefixed with `#`. Implies `-v`. |
| `--version` | Display version information |

When a command is executed (`-x` or `-y`), its exit code becomes gx's exit code, and gx's own banners go to stderr — stdout carries only the command's output.

### Stdin Support

Use `-` as a command-line option to read from stdin. The stdin content will be appended to your prompt:

```bash
# Append stdin to prompt
cat error.log | gx - "explain this error"

# Use stdin as entire prompt
docker ps | gx -

# Works with other flags (flags first)
git diff | gx -y - "create a commit message for these changes"
```

Piped content is untrusted input to the model. Prefer `gx -` and review over `gxx -` for content you didn't write.

## Shortcuts

| Command | Description |
|--------|-------------|
| `gx` | Standard command generation and execution |
| `gxx` | Shortcut that automatically includes `-y` flag — equivalent to `gx -y` (YOLO mode) |

## Storage

| File | Mode | Purpose |
|------|------|---------|
| `~/.gx` | 0600 | Latest generated command (staging area) |
| `~/.gxprompt` | 0600 | Last full prompt and response (debug log; path set by `GX_PROMPT_OUTPUT`) |

## Shell and Platform Aware

gx reads your shell from `$SHELL` (your login shell), or on Windows detects PowerShell vs CMD, and tailors the prompt to it: correct comment syntax (`#` vs `REM`), line continuation (`\`, PowerShell backtick, CMD `^`), and fish-specific syntax.

Platform is detected too — macOS, Linux, WSL2, and Windows. On macOS the model is told the userland is BSD (so you get `sed -i ''`, not GNU `sed -i`); on WSL2 it is told Windows drives live under `/mnt/<letter>`.

## Context Sent to the Model

Each request includes a short context block so commands fit your machine:

| Item | Details |
|------|---------|
| `cwd` | Current working directory |
| Unix/Linux/macOS | `HOME`, `USER` (or `LOGNAME`) |
| Windows | `USERPROFILE`, `USERNAME`, `TEMP` (or `TMP`) |
| Common | `GOPATH`, `VIRTUAL_ENV`, `DOCKER_HOST`, `KUBECONFIG`, `AWS_PROFILE`, `AWS_REGION`, `GCP_PROJECT` |
| `installed` | Which of a curated set of tools are on your `PATH` (e.g. `brew`, `apt`, `git`, `docker`, `kubectl`, `rg`, `fd`, `jq`, `gsed`, `python3`) — looked up only, never run |

Only these allow-listed variables are sent; API keys and anything secret-named are never included (and are redacted if ever added). Values are capped at 200 characters. Use `gx -p "..."` to see exactly what would be sent.

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `OPENROUTER_API_KEY` | OpenRouter API key — enables OpenRouter (used if no `claude` CLI) | — |
| `OLLAMA_HOST` | Ollama base URL — enables Ollama (used if neither of the above) | — |
| `GX_MODEL` | Model override (takes priority over `MODEL`) | provider default |
| `MODEL` | Model override | provider default |
| `GX_PROMPT_OUTPUT` | Path for the prompt log (`~/` is expanded) | `~/.gxprompt` |
| `LLM_DEBUG` | Set to `1` to trace provider selection inside easy-llm-wrapper | — |

**Provider priority:** `claude` CLI on `PATH` > `OPENROUTER_API_KEY` > `OLLAMA_HOST`

**Default models:** Claude CLI's own default · `anthropic/claude-3-haiku` (OpenRouter) · `gpt-oss:20b` (Ollama)

### Debugging

Use `-D` for a full diagnostic dump to stderr. Every line is prefixed with `#` (shell comment syntax). `-D` also implies `-v`:

```bash
gx -D "list files in current directory"
# provider: openrouter
# model:    anthropic/claude-3-haiku
# shell:    zsh
# platform: darwin/arm64
# prompt:
# list files in current directory
# sending request...
# response:
# # List all files, including hidden ones, with details
# ls -la
# List all files, including hidden ones, with details
ls -la
```

With the Claude CLI provider, a few extra wrapper lines prefixed `[llm debug]` also appear.

Use `-p` to print the full prompt (system instruction + user message) without sending it to the LLM:
```bash
gx -p "list files in current directory"
```

The last prompt and response are always written to `~/.gxprompt` (or `GX_PROMPT_OUTPUT`), readable only by you.

## Development

Everything runs through `make`; bare `make` lists the targets.

| Target | Description |
|--------|-------------|
| `make help` | List all targets (default) |
| `make build` | Build `./gx` and `./gxx` |
| `make install` | `go install` both binaries |
| `make run ARGS="..."` | Build and run `gx` with arguments |
| `make test` | Run all tests with the race detector |
| `make check` | gofmt check + `go vet` + tests (what CI should run) |
| `make lint` | `go vet` |
| `make fmt` | `gofmt -s` (+ `goimports` if installed) |
| `make tidy` | `go mod tidy` |
| `make version` | Print the current version |
| `make version-increment` | Bump the patch version |
| `make push` | fmt, tidy, build, test, bump patch, commit (message from `gitsum`), push, tag |
| `make update-llm` | Pull the latest easy-llm-wrapper, bypassing the module proxy cache |
| `make clean` | Remove built binaries |

Tests never touch your real `~/.gx` or call a real LLM: the CLI tests run against a fake Ollama server with an isolated `HOME` and `PATH`.

## Project Structure

```
gx/
├── main.go              # gx CLI entry point (thin wrapper)
├── version.go           # Re-exports internal/version (compat)
├── Makefile             # Build automation
├── SPEC.md              # Full specification
├── ROADMAP.md           # Planned and brainstormed work
├── go.mod / go.sum      # Dependencies
├── cmd/
│   └── gxx/
│       └── main.go      # gxx CLI entry point (thin wrapper with -y flag)
└── internal/
    ├── cli/             # Shared CLI logic (flags, stdin, execution) + tests
    ├── version/         # Semantic version constant
    ├── llm/             # Provider client, system prompt, context collection + tests
    └── history/         # ~/.gx staging file management + tests
```

## Technical Details

- **SDK:** [`github.com/nealhardesty/easy-llm-wrapper`](https://github.com/nealhardesty/easy-llm-wrapper) — the only dependency
- **Providers:** Claude CLI, OpenRouter (cloud), or Ollama (local) — auto-selected from environment
- **System Instruction:** Compact, shell- and platform-specific prompt that demands raw commands only. As a safety net, a reply wrapped in a Markdown code fence or backticks is unwrapped before it is printed or staged.

## Troubleshooting

### "no LLM provider configured"

Install the `claude` CLI, or set either `OPENROUTER_API_KEY` or `OLLAMA_HOST`:

```bash
# For Ollama (local)
export OLLAMA_HOST=http://localhost:11434

# For OpenRouter (cloud)
export OPENROUTER_API_KEY=your_key_here
```

### "failed to generate response"

Ensure the configured provider is reachable:
- **Claude CLI:** `claude -p hello` should answer
- **Ollama:** `ollama list` — confirm Ollama is running, `OLLAMA_HOST` is correct, and the model (default `gpt-oss:20b`) is pulled
- **OpenRouter:** confirm `OPENROUTER_API_KEY` is valid at [openrouter.ai](https://openrouter.ai)

Run `LLM_DEBUG=1 gx -D "..."` to see which provider was picked and why.

### Forcing a specific provider

The `claude` CLI is chosen whenever it is on `PATH`. To use OpenRouter or Ollama instead, run gx with a `PATH` that excludes it, e.g. a shell alias:

```bash
alias gxo='PATH=/usr/local/bin:/usr/bin:/bin gx'
```

## License

See [LICENSE](LICENSE).
