# gx

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
- `gcloud auth application-default login`

```bash
go build -o gx main.go
sudo mv gx /usr/local/bin/
```

- or - 

```bash
go install github.com/nealhardesty/gx@latest
gcloud auth application-default login
```

## Usage

```bash
# Generate a command
gx "find all large files over 100mb and sort by size"
# Output: find . -type f -size +100M -exec ls -lh {} + | sort -rh -k5

# Execute the staged command
gx -x

# Refine with context awareness
gx "actually, only look in /var/log"

# YOLO mode (generate and execute immediately)
gx -y "list docker containers"
```

## Options

| Flag | Description |
|------|-------------|
| `-x` | Execute command staged in `~/.gx` |
| `-y` | YOLO mode — execute immediately (no staging review) |
| `-v` | Verbose — include detailed comments in output |
| `-c` | Clear history and staged commands |
| `-n` | Disable tools (no file system access for LLM) |

## Storage

| File | Purpose |
|------|---------|
| `~/.gx` | Latest generated command (staging area) |
| `~/.gxhistory` | JSON log of recent prompt/response pairs |

## Tools

The LLM has access to a **readonly** `files` tool for context gathering:
- `pwd` / `ls` / `ls -R` / `stat` / `cat`

The LLM also has access to a detailed `ps` equivalent to see running processes.

The LLM also should have access to `uptime` command

Disable all tools with `-n` flag.

## Shell Aware

gx is aware of the shell that is running as the parent, be it 'sh', 'bash', 'zsh', 'powershell'

Obviously, the context of the current platform (mac, linux, wsl2, powershell/windows cmd) and the operating system (ubuntu, fedora, windows, windows/wsl2) should be provided in context to the prompt.

## Configuration

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| `GX_MODEL` | Gemini model to use | `gemini-1.5-flash` |
| `GX_HISTORY` | Number of history commands to keep by default in ~/.gxhistory | 10 |

## Technical Details

- **SDK:** `cloud.google.com/go/vertexai/genai`
- **Model:** `gemini-1.5-flash` (optimized for speed/latency)
- **System Instruction:** Shell-type aware prompt that returns raw commands only — no markdown, no backticks, no explanations. Comments use shell-appropriate syntax.

Note, the prompt will need context information always passed in (OS, platform, etc)

## License

See [LICENSE](LICENSE).
