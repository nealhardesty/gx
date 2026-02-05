# gx

![gx logo](gx.png)

A lightning-fast CLI assistant that converts natural language into executable shell commands using Google Gemini (Vertex AI).

**Zero fluff.** Returns raw shell code, not chatty explanations.

## Architecture

```
┌─────────────┐    ┌──────────────┐    ┌─────────────┐    ┌──────────┐
│ User Prompt │───▶│ Load Context │───▶│ Vertex AI   │───▶│ Stage to │
│             │    │ ~/.gxhistory │    │ Gemini LLM  │    │ ~/.gx    │
└─────────────┘    └──────────────┘    └─────────────┘    └──────────┘
                                              │                 │
                                              ▼                 ▼
                                       ┌─────────────┐   ┌───────────┐
                                       │ Tools       │   │ Execute   │
                                       │ (files API) │   │ (-x / -y) │
                                       └─────────────┘   └───────────┘
```

**Generate → Cache → Execute** flow:
1. **Prompt** — User passes natural language to `gx`
2. **Context** — Loads last 2-3 turns from `~/.gxhistory` for follow-up awareness
3. **Inference** — Sent to Vertex AI with strict system instruction (shell-type aware)
4. **Stage** — Output saved to `~/.gx` for review
5. **Execute** — Run via `-x` (review first) or `-y` (YOLO mode)

## Installation

**Prerequisites:**
- [Go 1.21+](https://go.dev/)
- Google Cloud Project with Vertex AI API enabled
- [Google Cloud CLI (`gcloud`)](https://cloud.google.com/sdk/docs/install) installed and configured

**GCP Setup:**
```bash
# Authenticate with Google Cloud
gcloud auth application-default login

# Set your default GCP project (REQUIRED)
gcloud config set project YOUR_PROJECT_ID
```

> **Note:** If you see `no project ID specified and failed to get default`, run the `gcloud config set project` command above with your GCP project ID.

**Build from source:**
```bash
# Build both gx and gxx
make build

# Or manually
go build -o gx .
go build -o gxx ./cmd/gxx
sudo mv gx gxx /usr/local/bin/   # Linux/macOS

# Or on Windows (PowerShell)
go build -o gx.exe .
go build -o gxx.exe ./cmd/gxx
```

**Or install directly:**
```bash
# Installs both gx and gxx
go install github.com/nealhardesty/gx@latest
go install github.com/nealhardesty/gx/cmd/gxx@latest

# Or use make install (builds and installs both)
make install

# Then configure gcloud (if not already done)
gcloud auth application-default login
gcloud config set project YOUR_PROJECT_ID
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
| `-n` | Disable tools (no file system access for LLM) |
| `-p` | Print the prompt that would be sent to the LLM (don't send it) |
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

The `gxx` command is a convenience shortcut that automatically generates and executes commands immediately (YOLO mode) without needing to pass the `-y` flag. Both `gx` and `gxx` are built and installed together.

## Storage

| File | Purpose |
|------|---------|
| `~/.gx` | Latest generated command (staging area) |
| `~/.gxhistory` | JSON log of recent prompt/response pairs |

## Tools

The LLM has access to **readonly** tools for context gathering:

| Tool | Description |
|------|-------------|
| `pwd` | Current working directory |
| `ls` | List directory contents |
| `ls -R` | Recursive directory listing |
| `stat` | File/directory metadata |
| `cat` | Read file contents (max 100KB) |
| `ps` | Running processes |
| `uptime` | System uptime |

Disable all tools with `-n` flag.

## Shell Aware

gx is aware of the shell that is running as the parent, be it 'sh', 'bash', 'zsh', 'powershell'

Obviously, the context of the current platform (mac, linux, wsl2, powershell/windows cmd) and the operating system (ubuntu, fedora, windows, windows/wsl2) should be provided in context to the prompt.

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `GX_MODEL` | Gemini model to use | `gemini-2.5-flash-lite` |
| `GX_HISTORY` | Max history entries | `10` |
| `GX_PROMPT_OUTPUT` | Path to write prompt logs for debugging | `~/.gxprompt` |

### Debugging

Use the `-p` flag to see exactly what prompt is being sent to the LLM:
```bash
gx -p "list files in current directory"
```

This will print the full prompt including system instructions, history context, and your input without actually sending it to the LLM.

Prompt logs are automatically written to the file specified by `GX_PROMPT_OUTPUT` (default: `~/.gxprompt`) for every request, showing the complete conversation flow including tool calls and responses.

## Project Structure

```
gx/
├── main.go              # gx CLI entry point (thin wrapper)
├── version.go           # Semantic version constant (re-exports internal/version)
├── Makefile             # Build automation
├── go.mod / go.sum      # Dependencies
├── cmd/
│   └── gxx/
│       └── main.go      # gxx CLI entry point (thin wrapper with -x flag)
└── internal/
    ├── cli/
    │   └── cli.go       # Shared CLI logic (used by both gx and gxx)
    ├── version/
    │   └── version.go   # Semantic version constant
    ├── gemini/
    │   └── client.go     # Vertex AI client, system prompts
    ├── history/
    │   └── history.go   # ~/.gxhistory management
    └── tools/
        ├── registry.go  # Tool registration & dispatch
        ├── files.go     # File system tools
        └── process.go   # Process tools (ps, uptime)
```

## Technical Details

- **SDK:** `cloud.google.com/go/vertexai/genai`
- **Model:** `gemini-2.5-flash-lite` (optimized for speed/latency)
- **System Instruction:** Shell-type aware prompt that returns raw commands only — no markdown, no backticks, no explanations. Comments use shell-appropriate syntax.
- **Context:** OS, platform, and shell type automatically detected and passed to the LLM

## Troubleshooting

### "no project ID specified and failed to get default"

This error means gcloud doesn't have a default project configured. Fix it by running:

```bash
gcloud config set project YOUR_PROJECT_ID
```

To find your project ID, run `gcloud projects list` or check the [Google Cloud Console](https://console.cloud.google.com/).

### "failed to create Gemini client"

Ensure you have:
1. Authenticated: `gcloud auth application-default login`
2. Enabled Vertex AI API in your GCP project
3. Set your project: `gcloud config set project YOUR_PROJECT_ID`

## License

See [LICENSE](LICENSE).
