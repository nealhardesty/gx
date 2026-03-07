# gx

![gx logo](gx.png)

A lightning-fast CLI assistant that converts natural language into executable shell commands using local or cloud LLMs via [easy-llm-wrapper](https://github.com/nealhardesty/easy-llm-wrapper).

**Zero fluff.** Returns raw shell code, not chatty explanations.

## Architecture

```
┌─────────────┐    ┌──────────────┐    ┌─────────────┐    ┌──────────┐
│ User Prompt │───▶│ Load Context │───▶│ LLM         │───▶│ Stage to │
│             │    │ ~/.gxhistory │    │ (Ollama or  │    │ ~/.gx    │
└─────────────┘    └──────────────┘    │  OpenRouter)│    └──────────┘
                                       └─────────────┘         │
                                                                ▼
                                                          ┌───────────┐
                                                          │ Execute   │
                                                          │ (-x / -y) │
                                                          └───────────┘
```

**Generate → Cache → Execute** flow:
1. **Prompt** — User passes natural language to `gx`
2. **Context** — Loads last 2-3 turns from `~/.gxhistory` for follow-up awareness
3. **Inference** — Sent to Ollama or OpenRouter with a shell/platform/environment-aware system prompt
4. **Stage** — Output saved to `~/.gx` for review
5. **Execute** — Run via `-x` (review first) or `-y` (YOLO mode)

## Installation

**Prerequisites:**
- [Go 1.21+](https://go.dev/)
- Ollama running locally **or** an OpenRouter API key

**Ollama setup (local):**
```bash
# Install Ollama: https://ollama.com
ollama pull llama3.2
export OLLAMA_HOST=http://localhost:11434
```

**OpenRouter setup (cloud):**
```bash
export OPENROUTER_API_KEY=your_api_key_here
```

**Build from source:**
```bash
# Build both gx and gxx
make build

# Or manually
go build -o gx .
go build -o gxx ./cmd/gxx
sudo mv gx gxx /usr/local/bin/   # Linux/macOS
```

**Or install directly:**
```bash
go install github.com/nealhardesty/gx@latest
go install github.com/nealhardesty/gx/cmd/gxx@latest

# Or use make install (builds and installs both)
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

# Refine with context awareness
gx "actually, only look in /var/log"

# YOLO mode (generate and execute immediately)
gx -y "list docker containers"

# Read from stdin using '-' option
cat error.log | gx - "explain this error"
docker ps | gx - "create a kill command for these containers"
git diff | gx -  # Use stdin as entire prompt
```

## Options

| Flag | Description |
|------|-------------|
| `-` | Read additional input from stdin and append to prompt |
| `-x` | Execute command staged in `~/.gx` |
| `-y` | YOLO mode — execute immediately (no staging review) |
| `-v` | Verbose — include detailed comments in output |
| `-c` | Clear history and staged commands |
| `-p` | Print the prompt that would be sent to the LLM (don't send it) |
| `-D` | Debug mode — dump all activity to stderr, each line prefixed with `#`. Implies `-v`. |
| `--version` | Display version information |

### Stdin Support

Use `-` as a command-line option to read from stdin. The stdin content will be appended to your prompt:

```bash
# Append stdin to prompt
cat error.log | gx - "explain this error"

# Use stdin as entire prompt
docker ps | gx -

# Works with other flags
git diff | gx -y - "create a commit message for these changes"
```

## Shortcuts

| Command | Description |
|--------|-------------|
| `gx` | Standard command generation and execution |
| `gxx` | Shortcut that automatically includes `-y` flag — equivalent to `gx -y` (YOLO mode) |

## Storage

| File | Purpose |
|------|---------|\
| `~/.gx` | Latest generated command (staging area) |
| `~/.gxhistory` | JSON log of recent prompt/response pairs |

## Shell Aware

gx automatically detects the shell running as its parent (`sh`, `bash`, `zsh`, `powershell`, `cmd`) and tailors output accordingly — correct comment syntax, line continuation style, and idioms per shell.

Platform and OS are also detected: macOS, Linux, WSL2, and Windows (PowerShell/CMD) are all handled.

## Environment Context

On every request, relevant environment variables are automatically collected and injected into the LLM system prompt so it can generate accurate, context-aware commands:

| Category | Variables Collected |
|----------|-------------------|
| Unix/Linux/macOS | `HOME`, `USER`/`LOGNAME`, `SHELL`, `PWD` |
| Windows | `USERPROFILE`, `USERNAME`, `ComSpec`, `PSModulePath`, `TEMP`/`TMP` |
| Common | `PATH`, `GOPATH`, `GOROOT`, `DOCKER_HOST`, `KUBECONFIG`, `AWS_PROFILE`, `AWS_REGION`, `GCP_PROJECT` |
| gx config | `GX_MODEL`, `GX_HISTORY`, `GX_PROMPT_OUTPUT` |

Sensitive variable names (containing `KEY`, `TOKEN`, `SECRET`, `PASSWORD`, `AUTH`, `CREDENTIAL`) are automatically redacted. Long values like `PATH` are truncated.

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `OPENROUTER_API_KEY` | OpenRouter API key — enables OpenRouter provider (takes priority over Ollama) | — |
| `OLLAMA_HOST` | Ollama base URL — enables Ollama provider | — |
| `GX_MODEL` | Model override (takes priority over `MODEL`) | provider default |
| `MODEL` | Model override | provider default |
| `GX_HISTORY` | Max history entries | `10` |
| `GX_PROMPT_OUTPUT` | Path to write prompt logs for debugging | `~/.gxprompt` |

**Provider priority:** `OPENROUTER_API_KEY` > `OLLAMA_HOST`

**Default models:** `llama3.2` (Ollama) · `anthropic/claude-3-haiku` (OpenRouter)

### Debugging

Use `-D` for a full diagnostic dump to stderr. Every line is prefixed with `#` (shell comment syntax). `-D` also implies `-v`:

```bash
gx -D "list files in current directory"
# provider: openrouter
# model:    anthropic/claude-3-haiku
# shell:    zsh
# platform: wsl2/amd64
# history: 0 entries
# prompt:
# list files in current directory
# sending request...
# response:
# ls -la
ls -la
```

Use `-p` to print the full prompt (system instruction + history + user message) without sending it to the LLM:
```bash
gx -p "list files in current directory"
```

Prompt logs are automatically written to `~/.gxprompt` (or `GX_PROMPT_OUTPUT`) for every request.

## Project Structure

```
gx/
├── main.go              # gx CLI entry point (thin wrapper)
├── version.go           # Semantic version constant (re-exports internal/version)
├── Makefile             # Build automation
├── go.mod / go.sum      # Dependencies
├── cmd/
│   └── gxx/
│       └── main.go      # gxx CLI entry point (thin wrapper with -y flag)
└── internal/
    ├── cli/
    │   └── cli.go       # Shared CLI logic (used by both gx and gxx)
    ├── version/
    │   └── version.go   # Semantic version constant
    ├── llm/
    │   └── client.go    # LLM client (easy-llm-wrapper + system prompt logic)
    └── history/
        └── history.go   # ~/.gxhistory management
```

## Technical Details

- **SDK:** [`github.com/nealhardesty/easy-llm-wrapper`](https://github.com/nealhardesty/easy-llm-wrapper)
- **Providers:** Ollama (local) or OpenRouter (cloud) — auto-selected from environment
- **System Instruction:** Shell-type aware prompt that returns raw commands only — no markdown, no backticks, no explanations. Comments use shell-appropriate syntax.
- **Context:** OS, platform, shell type, and environment variables are automatically passed to the LLM.

## Troubleshooting

### "no LLM provider configured"

Set either `OPENROUTER_API_KEY` or `OLLAMA_HOST`:

```bash
# For Ollama (local)
export OLLAMA_HOST=http://localhost:11434

# For OpenRouter (cloud)
export OPENROUTER_API_KEY=your_key_here
```

### "failed to create LLM client"

Ensure the configured provider is reachable:
- **Ollama:** `ollama list` — confirm Ollama is running and `OLLAMA_HOST` is correct
- **OpenRouter:** confirm `OPENROUTER_API_KEY` is valid at [openrouter.ai](https://openrouter.ai)

## License

See [LICENSE](LICENSE).
