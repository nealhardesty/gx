# gx — Specification

> **Status:** describes gx **v1.2.4** (`internal/version/version.go`) built against
> `github.com/nealhardesty/easy-llm-wrapper` **v0.1.11**. This file is the
> authority on *how gx works*. User-facing setup lives in `README.md`; work that
> does not exist yet lives in `ROADMAP.md`. If code and this document disagree,
> one of them is a bug — fix whichever is wrong, in the same change.

---

## 1. Purpose and philosophy

gx turns a natural-language request into a **directly executable shell command**
for the user's current shell and platform, prints it, and keeps it so it can be
run on demand.

Design tenets (these drive every decision below):

1. **Raw output, zero fluff.** stdout receives the command and nothing else, so
   `gx ... | pbcopy`, `$(gx ...)` and redirection work. Diagnostics go to stderr.
2. **Generate → Stage → Execute.** Generation never runs anything unless the user
   opts in (`-y` / `gxx`). The last command is always staged in `~/.gx` for review.
3. **Stateless prompts.** Each invocation is a single-turn request. There is no
   conversation history (removed in v1.2.4) and no tool calling or command output
   collected as context (removed in v1.2.3). The only context is shell, platform,
   cwd, a fixed allow-list of environment variables, and which well-known tools
   exist on `PATH` (a lookup, never an execution).
4. **Small prompts.** Every token in the system prompt must earn its place; it is
   sent on every request and smaller local models follow short prompts better.
5. **Provider-agnostic.** All LLM transport is delegated to `easy-llm-wrapper`;
   gx owns only the prompt, the context, and the CLI.
6. **One dependency.** The only non-stdlib module is `easy-llm-wrapper`.

---

## 2. Binaries

| Binary | Entry point | Behavior |
|--------|-------------|----------|
| `gx`  | `main.go`         | `cli.Run(cli.Options{Version})` |
| `gxx` | `cmd/gxx/main.go` | `cli.Run(cli.Options{ForceYolo: true, Version})` — identical to `gx -y` |

Both are thin wrappers; all behavior lives in `internal/cli`. `ForceYolo` only
changes the *default* of `-y`, so `gxx -y=false "..."` behaves like plain `gx`.
The process exit code is whatever `cli.Run` returns (see §6).

---

## 3. Architecture

```
            ┌───────────────────────── internal/cli ─────────────────────────┐
argv/stdin ─▶ flag parse ─▶ mode dispatch ─┬─ --version → print, exit 0       │
            │                              ├─ -c → history.Clear()            │
            │                              ├─ -x → history.Get → execute      │
            │                              ├─ -p → llm.BuildPrompt → stdout   │
            │                              └─ generate:                       │
            │                                   llm.Generate ─▶ clean ─▶ stdout│
            │                                   history.StageCommand (~/.gx)  │
            │                                   if -y → execute               │
            └──────────────────────────────────────────────────────────────────┘
                       │                                    │
          internal/llm │ system prompt, shell/platform      │ internal/history
                       │ detection, env collection,         │ ~/.gx staging file
                       │ ~/.gxprompt log                    │
                       ▼
            easy-llm-wrapper (provider auto-detect + transport)
               ├─ claude   → exec `claude -p ...`
               ├─ openrouter → HTTPS https://openrouter.ai/api/v1
               └─ ollama   → HTTP  $OLLAMA_HOST/api/chat
```

### 3.1 Source layout

| Path | Package | Responsibility |
|------|---------|----------------|
| `main.go` | `main` | `gx` entry point |
| `version.go` | `main` | Re-exports `version.Version` as `main.Version` (backward compat only; nothing reads it) |
| `cmd/gxx/main.go` | `main` | `gxx` entry point (`ForceYolo`) |
| `internal/cli/cli.go` | `cli` | Flags, usage text, mode dispatch, prompt assembly, command execution. `Run` wraps `run(opts, args, stdin, stdout, stderr)` for testability |
| `internal/llm/client.go` | `llm` | Wraps easy-llm-wrapper; builds system prompt; detects shell/platform; collects context; cleans responses; writes prompt log |
| `internal/history/history.go` | `history` | Reads/writes/removes the `~/.gx` staging file (name is historical — it no longer holds history) |
| `internal/*/*_test.go` | — | Unit and end-to-end tests (§10.4) |
| `internal/version/version.go` | `version` | `const Version` — single source of truth for the version string |
| `Makefile` | — | All automation (§10) |
| `gx.png` | — | Logo used by README (~2 MB) |

---

## 4. Command-line interface

```
gx [flags] [prompt words...] [-]
```

### 4.1 Flags

Parsed with a dedicated `flag.FlagSet` in `ContinueOnError` mode (single or
double dash both work: `-version` ≡ `--version`).

| Flag | Default | Effect |
|------|---------|--------|
| `-x` | false | Execute the command staged in `~/.gx`; no LLM call |
| `-y` | false (`true` in `gxx`) | After generating, execute immediately |
| `-v` | false | Ask the LLM to include explanatory comments |
| `-c` | false | Delete `~/.gx` and print `Staged command cleared.` |
| `-p` | false | Print the full prompt (SYSTEM + USER) to stdout instead of sending it |
| `-D` | false | Debug trace to stderr (§8). Forces `-v` on |
| `-version` | false | Print `gx version <X.Y.Z>` and exit 0 |
| `-h`, `-help` | — | Print usage to stderr (Go `flag` exits **0** for `-h`) |

### 4.2 Mode precedence

Evaluated top-down in `cli.Run`; the first match wins and the rest are ignored:

1. `-D` ⇒ set `-v` (not a mode, a modifier)
2. `-version`
3. `-c` (so `gx -c -x` clears and does **not** execute)
4. `-x` (prompt args are ignored)
5. Build prompt (§4.3); if empty ⇒ usage on stderr, exit 1
6. Create the LLM client (provider resolution, §5.1) — fails with exit 1 if none
7. `-p`
8. Generate, clean (§5.2), print, stage, then execute if `-y`

### 4.3 Prompt assembly

- All positional args except the literal `-` are joined with single spaces.
- If any positional arg is exactly `-`, stdin is read **to EOF**, `TrimSpace`d, and:
  - prompt empty ⇒ prompt = stdin
  - stdin empty ⇒ prompt = args (no dangling separator)
  - otherwise ⇒ prompt = `args + "\n\n---\n\n" + stdin`
- The position of `-` among the args does not matter; stdin is always appended
  after the text. Multiple `-` read stdin once.
- No quoting is required: `gx find big files` ≡ `gx "find big files"`. Quote
  anyway when the prompt contains shell metacharacters (`*`, `?`, `|`, `>`, `$`).

### 4.4 Hard-won parsing facts

- **Flags must come before the prompt.** Go's `flag` stops at the first
  non-flag argument. `gx "list files" -y` sends the prompt `list files -y` and
  does *not* enable YOLO. Covered by `TestFlagsAfterPromptAreText`; the usage
  text says so.
- **A bare `-` also stops flag parsing.** `gx - -v` puts `-v` into the prompt.
  Write `gx -v - "..."`.
- `-` with no stdin attached (a TTY) blocks until the user types EOF (Ctrl-D /
  Ctrl-Z).

---

## 5. LLM layer (`internal/llm`)

### 5.1 Provider and model resolution

`llm.NewClient` calls `elw.ConfigFromEnv()` (easy-llm-wrapper v0.1.11), then
applies gx's own override. Provider resolution is **first match wins**:

| # | Condition | Provider | Default model | Transport |
|---|-----------|----------|---------------|-----------|
| 1 | `claude` binary found on `PATH` | `claude` | *(none — Claude CLI's own default)* | `exec claude -p <prompt> --append-system-prompt <system> [--model M] --output-format text` |
| 2 | `OPENROUTER_API_KEY` non-empty | `openrouter` | `anthropic/claude-3-haiku` | HTTPS to `https://openrouter.ai/api/v1` |
| 3 | `OLLAMA_HOST` non-empty | `ollama` | `gpt-oss:20b` | HTTP `POST $OLLAMA_HOST/api/chat`; `http://` prepended if no scheme |
| — | none | error | — | `no LLM provider configured: install claude CLI, or set OPENROUTER_API_KEY or OLLAMA_HOST` |

Model: `GX_MODEL` > `MODEL` > provider default. `MODEL` is read by the wrapper;
`GX_MODEL` is applied by gx afterwards and therefore wins for every provider,
including `claude` (passed as `--model`).

> ⚠️ **The Claude CLI silently wins.** If `claude` is on `PATH`, OpenRouter and
> Ollama are never used, even when their variables are set. To force another
> provider, run gx with a `PATH` that excludes `claude` (README → Troubleshooting).

`NewClient` is called for **both** generation and `-p`, so `-p` also fails with
the "no provider" error when nothing is configured, even though it never sends.

### 5.2 Request shape

Exactly one request per invocation:

```go
elw.Request{
    System:   buildSystemInstruction(),
    Messages: []elw.Message{{Role: user, Parts: [TextPart(prompt)]}},
}
```

- Non-streaming (`Complete`), no temperature/max-token settings, no tools.
- `context.Background()` — **no timeout**; a hung provider hangs gx (Ctrl-C to abort).
- The response goes through `cleanResponse`: trimmed; if the whole reply is a
  Markdown fence (opening ```` ```lang ```` line … closing ```` ``` ```` line) the
  fence lines are dropped; if it is a single line wrapped in exactly one pair of
  backticks they are dropped. Backticks used *inside* a command (`` echo `date` ``)
  are left alone. Nothing else is validated — the cleaned text is what gets
  printed, staged, logged and executed.

### 5.3 System prompt

Built fresh on every call by `buildSystemInstruction()`. Template (variables in `{}`,
optional lines marked):

```
You turn a request into a ready-to-run {shell} command on {platform}.

Reply with the command text only: no prose, no Markdown, no code fences. Your reply is pasted into {shell} and executed exactly as written.
- {commentRule}
- Split long commands with {continuation}; put independent steps on separate lines.
- Prefer tools listed as installed. Do not add destructive options (force, recursive delete, overwrite) the request does not ask for.
- If no command can do it, reply only with a {comment} comment saying why.
- {shell note}            ← only if the shell has one
- {platform note}         ← only if the platform has one

Context:                  ← only if any line was collected (§5.6)
- cwd: /path
- HOME: ...
- installed: brew, git, rg, jq, ...
```

**Shell profiles** (`profileFor`):

| Shell | `comment` | `continuation` | Note |
|-------|-----------|----------------|------|
| `powershell`, `pwsh` | `#` | trailing backtick | Use PowerShell cmdlets and syntax. Comments start with #; never use REM. |
| `cmd` | `REM` | trailing caret `^` | Use cmd.exe syntax and built-ins; chain steps with &&. |
| `fish` | `#` | trailing backslash | fish syntax: `set VAR value`, begin/end blocks, no heredocs. |
| anything else | `#` | trailing backslash | — |

**Platform notes** (`platformNote`): `darwin/*` → BSD userland, use BSD flags
(`sed -i ''`, `stat -f`, `date -v`); `wsl2/*` → Windows drives at `/mnt/<letter>`;
others → none.

**Comment rule:** `No comments.` by default; with `-v`/`-D`:
`Put a short {comment} comment before each step explaining what it does.`

Design notes (hard-won):
- The v1.2.4 prompt was ~1.4 KB with a duplicated "copy-paste ready" rule, a
  redundant `Operating System` line and an unconditional PowerShell warning
  block. The current one is ~0.85 KB on a typical macOS machine and carries more
  useful context.
- "Your reply is pasted into {shell} and executed exactly as written" replaces
  the shouted, repeated rule: models comply better with the *consequence* than
  with emphasis.
- The BSD note fixes the most common macOS failure (GNU `sed -i` / `stat -c`).
  Verified live: "replace foo with bar in place" → `sed -i '' 's/foo/bar/g' ./*.txt`.
- The PowerShell REM warning exists because models emitted `REM` in PowerShell
  before it was added (2026-01-31); it now lives in the PowerShell profile only.
- The "no destructive options" rule is a soft guard, not a security boundary (§9).

### 5.4 Shell detection (`detectShell`)

First match wins:

1. `$SHELL` set ⇒ its basename, minus any `.exe` (`/bin/zsh` → `zsh`, `/usr/local/bin/fish` → `fish`)
2. `PSModulePath` set ⇒ `powershell`
3. `ComSpec` contains `cmd.exe` ⇒ `cmd`
4. `GOOS == windows` ⇒ `powershell`
5. ⇒ `bash`

Facts:
- `$SHELL` is the **login** shell, not the parent process. Running `bash`
  inside a zsh login session still reports `zsh` (ROADMAP: parent-process
  detection).
- `PSModulePath` is checked before `ComSpec` because Windows sets `ComSpec`
  even inside PowerShell. Under Git Bash/MSYS on Windows, `$SHELL` is set and wins.
- `pwsh` is only reported if `$SHELL` ends in `pwsh`; the PSModulePath branch
  always reports `powershell`.

### 5.5 Platform detection (`detectPlatform`)

`{GOOS}/{GOARCH}` (e.g. `darwin/arm64`), except on Linux where `uname -r` is run
and a kernel string containing `microsoft` or `wsl` yields `wsl2/{GOARCH}`. WSL1
kernels also match and are reported as `wsl2`.

### 5.6 Context block (`collectEnvironment`)

Lines are emitted in this order, each only when available:

| Line | Source | Notes |
|------|--------|-------|
| `cwd` | `os.Getwd()` | Works on Windows, where `$PWD` does not exist |
| Non-Windows | `HOME`, `USER` (else `LOGNAME`) | |
| Windows | `USERPROFILE`, `USERNAME`, `TEMP` (else `TMP`) | |
| Common | `GOPATH`, `VIRTUAL_ENV`, `DOCKER_HOST`, `KUBECONFIG`, `AWS_PROFILE`, `AWS_REGION`, `GCP_PROJECT` | Table-driven (`unixEnv`, `windowsEnv`, `commonEnv`) |
| `installed` | `exec.LookPath` over `probeTools` | Comma list, in list order |

`probeTools`: `brew apt dnf pacman apk winget choco git docker podman kubectl rg
fd jq yq fzf gsed gawk python3 node go`. `LookPath` only stats `PATH` entries —
nothing is executed — so the probe costs well under a millisecond-per-tool and
tells the model which package manager and modern tools it may use. It replaces
the old 300-char truncated `PATH` dump, which was large and mostly noise.
Deliberately **not** sent any more: `PATH`, `SHELL` (the shell is named in the
header), `GOROOT`, `ComSpec`, `PSModulePath`, `GX_MODEL`, `GX_PROMPT_OUTPUT`.

Every value is capped at 200 **runes** (`truncate`, UTF-8 safe) with a
` (truncated)` suffix, and passed through `redact`: if the variable's *name*
contains `KEY`, `TOKEN`, `SECRET`, `PASSWORD`, `AUTH` or `CREDENTIAL`, the value
becomes `[REDACTED]`. The allow-list is the primary guarantee (no secret-named
variable is on it, `OPENROUTER_API_KEY` is never sent); `redact` is the backstop
for anyone extending the lists. `TestCollectEnvironment` asserts that keys and
tokens never appear.

Data sent to the provider therefore includes cwd, home, username and cloud
profile names — relevant when using a cloud provider.

### 5.7 Prompt log (`~/.gxprompt`)

Written after every `Generate` call — success **and** failure. Not written by `-p`.

- Path (`promptLogPath`): `$GX_PROMPT_OUTPUT` if set (a leading `~/` is
  expanded), else `~/.gxprompt`.
- Overwritten each run (not appended). Mode **0600**: the file holds the prompt,
  any piped stdin, and the context block. `os.WriteFile` does not change the mode
  of an existing file, so gx also `chmod`s it — logs left at 0644 by v1.2.4 and
  earlier are tightened on the next run. Write errors are ignored.
- Format: the shared `formatTranscript` (identical to `-p`, plus a response
  section and trailing newline):

```
SYSTEM:
<system prompt>

---

USER:
<prompt>

---

RESPONSE:
<cleaned response>   ← omitted when the request failed
```

### 5.8 `-p` output format

`formatTranscript(system, prompt, "")` — the same layout without `RESPONSE`.

---

## 6. Staging and execution

### 6.1 Staging file (`internal/history`)

| Property | Value |
|----------|-------|
| Path | `$HOME/.gx` (`os.UserHomeDir()`; not configurable) |
| Mode | `0600` |
| Content | Exact trimmed LLM response, no trailing newline |
| Written | After every successful generation, *before* any `-y` execution |
| Failure to write | `Warning: failed to stage command: ...` on stderr; exit code unaffected |
| `-c` | Removes the file; a missing file is not an error |
| `-x` with no file | `Error: no staged command found (run gx with a prompt first)`, exit 1 |

The staged content is whatever the model returned — including comment-only
"can't do that" answers. It is the user's job to review before `-x`.

### 6.2 Execution (`executeCommand`)

| OS | Interpreter |
|----|-------------|
| Windows, `PSModulePath` set | `powershell -Command <cmd>` |
| Windows, otherwise | `cmd /C <cmd>` |
| Everything else | `$SHELL -c <cmd>` (fallback `/bin/sh`) |

- stdin/stdout/stderr are inherited, so interactive commands (`sudo`, `less`,
  `read`) work. **Exception:** with `-` the stdin pipe was already consumed, so
  the executed command sees EOF on stdin.
- The child runs in gx's working directory and environment. Directory changes
  (`cd`) and `export`s in the command do not persist to the calling shell.
- Since the generated command targets `detectShell()` and execution uses
  `$SHELL`, the two agree on Unix. On Windows without PowerShell, generation may
  target `cmd` and execution uses `cmd` — consistent.
- Banners go to **stderr** in both modes (`-y`: `\n--- Executing ---`; `-x`:
  `Executing: <cmd>` and `---`), so stdout carries only the command's own output
  and `gx -x | grep ...` works. (Before this change `-x` printed its banner to
  stdout.)

### 6.3 Exit codes

| Situation | Exit |
|-----------|------|
| Success without execution | 0 |
| `-version`, `-c` success, `-p` | 0 |
| `-h` | 0 (Go `flag` default) |
| Unknown flag | 2 (Go `flag` default) |
| Empty prompt, provider/config error, LLM error, stdin read error | 1 |
| `-x` / `-y` executed | **child's exit code**, propagated |
| Child could not be started | 1 |

This makes `gxx "..." && next-step` behave as the user expects.

---

## 7. Files and environment reference

### 7.1 Files

| Path | Written by | Mode | Contents |
|------|-----------|------|----------|
| `~/.gx` | every generation | 0600 | Last generated command |
| `~/.gxprompt` (or `$GX_PROMPT_OUTPUT`) | every LLM request | 0600 | Last SYSTEM/USER/RESPONSE |

Legacy (no longer read or written, safe to delete): `~/.gxhistory` (removed
v1.2.4).

### 7.2 Environment variables

| Variable | Read by | Purpose |
|----------|---------|---------|
| `OPENROUTER_API_KEY` | wrapper | Enables OpenRouter (priority 2) |
| `OLLAMA_HOST` | wrapper | Enables Ollama (priority 3) |
| `MODEL` | wrapper | Model override |
| `GX_MODEL` | gx | Model override, beats `MODEL` |
| `GX_PROMPT_OUTPUT` | gx | Prompt-log path |
| `LLM_DEBUG=1` | wrapper | Wrapper's provider-selection trace on stderr, prefixed `[llm debug]` |
| `PATH` | wrapper | Presence of `claude` selects the Claude CLI provider |
| `SHELL`, `PSModulePath`, `ComSpec` | gx | Shell detection and execution interpreter |
| (allow-list in §5.6) | gx | Sent to the model as context |
| `PATH` | gx | Searched for `probeTools` (§5.6) |

---

## 8. Debugging

`-D` emits to stderr, every line prefixed `# ` so a combined stream remains
valid shell:

```
# provider: openrouter
# model:    anthropic/claude-3-haiku
# shell:    zsh
# platform: darwin/arm64
# prompt:
# <prompt lines>
# sending request...
# response:
# <response lines>
```

Caveats:
- `-D` also sets the wrapper's `Debug`. Only the `claude` provider honours it,
  and its lines are prefixed `[llm debug]`, not `#` — so with the Claude CLI
  provider the "every line starts with #" promise does not hold.
- Provider-selection tracing happens inside `ConfigFromEnv()` before gx can set
  `Debug`; use `LLM_DEBUG=1` to see it.
- For the provider `claude` the `model:` line is empty unless `GX_MODEL`/`MODEL` is set.

Other tools: `-p` (see exactly what would be sent) and `~/.gxprompt` (see exactly
what *was* sent and received).

---

## 9. Security model

- gx executes LLM output **only** on `-x`, `-y`, or `gxx`. Treat `gxx` as
  "run arbitrary model output as me" — no confirmation, no sandbox, no
  allow/deny list.
- Prompt injection: content piped via `-` is untrusted and is placed in the user
  message verbatim. `cat untrusted.txt | gxx -` can be steered into running
  anything. Prefer `gx -` + review + `gx -x` for untrusted input.
- Data egress to the provider: the prompt, any stdin, and the context block
  (§5.6: cwd, `HOME`, `USER`, cloud profile names, installed-tool names). No API
  keys or secret-named variables.
- `~/.gxprompt` and `~/.gx` are both owner-only (0600).
- The system prompt asks the model not to add destructive options the user did
  not request. That reduces accidents; it does not stop a hostile prompt.
- The Claude CLI provider passes the full prompt and system prompt as process
  arguments, visible to other local users via `ps` for the call's duration.

---

## 10. Build, release, automation

Toolchain: Go **1.21+** (`go.mod`), CGO not required. Builds use
`-ldflags="-s -w"` (stripped). On Windows binaries get `.exe`.

### 10.1 Make targets

| Target | Does |
|--------|------|
| `help` *(default)* | List `## target: description` lines; bare `make` runs this |
| `all` | `build` |
| `build` | `build-gx` + `build-gxx` → `./gx`, `./gxx` |
| `build-gx` / `build-gxx` | One binary each |
| `install` | `go install` both into `$GOBIN`/`$GOPATH/bin` |
| `run ARGS="..."` | Build `gx` and run with `ARGS` |
| `test` | `go test -race -v ./...` |
| `check` | `lint` + `test` + fail if `gofmt -s -l` reports files — the target CI should call |
| `lint` | `go vet ./...` |
| `fmt` | `gofmt -s -w .` + `goimports` if installed |
| `tidy` | `go mod tidy` |
| `version` | Print current version |
| `version-increment` | Bump patch in `internal/version/version.go` |
| `push` | `fmt tidy build test version-increment` → `git add -A` → commit `release: vX.Y.Z.  $(gitsum)` → push → tag `vX.Y.Z` → push tags |
| `update-llm` | `GOPROXY=direct go get easy-llm-wrapper@latest` + tidy (bypasses proxy cache for fresh tags) |
| `clean` | Remove built binaries |

### 10.2 Versioning and release

- SemVer, single source: `internal/version/version.go`. The `make` targets find
  it with `grep 'Version = '` / `sed`, so keep the line shape
  `const Version = "X.Y.Z"`.
- Every release bumps **patch**; minor/major bumps are manual edits.
- Releases are git tags `vX.Y.Z` on `master`; `go install ...@latest` resolves
  from those tags. No binary artifacts are published.
- `make push` depends on `gitsum` (an external AI commit-message tool, expected
  on `PATH`) and stages **everything** with `git add -A`.

### 10.3 Automation facts

- The version bump uses `sed -i.bak … && rm *.bak`, which works with both GNU
  and BSD (macOS) sed. Plain `sed -i "…"` — used up to v1.2.4 — fails on macOS
  with `invalid command code v`.
- `push` reuses `version-increment` rather than duplicating the bump logic.
- There is no CI yet; when it is added it must call `make check` (AGENTS.md).

### 10.4 Tests

`make test` / `make check`. No test needs network access, an API key, or the
real `~/.gx`.

| Package | Covers |
|---------|--------|
| `internal/history` | Stage/get/overwrite/clear, missing-file error, 0600 mode — under a temp `HOME` |
| `internal/llm` | `cleanResponse` table; `detectShell` precedence; prompt content per shell, verbose, platform notes; context collection with stubbed `lookPath` (no secrets, no `PATH`); `redact`; rune-safe `truncate`; `-p` format; `promptLogPath` expansion; log is 0600 even over a pre-existing 0644 file |
| `internal/cli` | End-to-end through `run()` against an `httptest` **fake Ollama** (`/api/chat`) with isolated `HOME`, `PATH=/usr/bin:/bin` (no `claude`), `SHELL=/bin/sh`: version, help/bad-flag exit codes, empty prompt, no-provider error, generate → fence-stripped stdout → staged → 0600 log, request body shape, stdin append, `-p` sends nothing, `gxx` executes and propagates exit code 3, trailing flags become prompt text, `-x` (stdout carries only command output), `-c` beats `-x`, `buildPrompt` table |

The CLI tests skip on Windows (they drive a POSIX shell). Tests use `t.Setenv`,
so they are not parallel.

---

## 11. Dependency: easy-llm-wrapper

`github.com/nealhardesty/easy-llm-wrapper` (same author; source at
`~/dev/easy-llm-wrapper`). gx uses only: `ConfigFromEnv`, `NewClientWithConfig`,
`Client.Complete`, `Client.Provider`, `Client.Model`, `Request`, `Message`,
`RoleUser`, `TextPart`.

Upgrading: `make update-llm`, then re-verify §5.1 (provider order, defaults,
error text) against the new `config.go` — gx documents those behaviors but does
not own them. Defaults have already drifted once (Ollama default changed from
`llama3.2` to `gpt-oss:20b`, and the README went stale until 2026-10-22).

---

## 12. History (why things are the way they are)

| Version / date | Change | Consequence today |
|----------------|--------|-------------------|
| 0.1.0 (2026-01-31) | Initial release on Google Vertex AI Gemini, with LLM tools (pwd/ls/stat/cat/ps/uptime) and JSON history in `~/.gxhistory` | — |
| 2026-01-31 | `gxx` added; logic moved to `internal/cli`; version to `internal/version` | Root `version.go` kept for compat |
| 2026-01-31 | PSModulePath-before-ComSpec; PowerShell REM warning | §5.4, §5.3 |
| 2026-02-05 | `-` stdin support | §4.3 |
| 2026-03-06 | Vertex replaced with easy-llm-wrapper (Ollama/OpenRouter); `-D` added | Single dependency; no GCP |
| v1.2.3 | Tools package and `-n` removed; provider config delegated to `elw.ConfigFromEnv()` | Claude CLI provider arrived via the wrapper |
| v1.2.4 | Conversation history removed; `-c` now clears staging only | Stateless prompts; `history` package name is vestigial |
| unreleased (2026-10-22) | SPEC/ROADMAP added; README drift fixed; compact shell/platform-aware prompt; installed-tool probe replaces `PATH`; response fence stripping; prompt log 0600; `-x` banner to stderr; `make` defaults to `help`; portable sed; `make check`; first test suite | §5.3, §5.6, §5.7, §6.2, §10 |

`CHANGELOG.md` was retired (deleted from the tree); git history and this table
are the record going forward.

---

## 13. Invariants (do not break without updating this spec)

1. stdout of a plain generation contains only the cleaned model response plus one
   newline; stdout of `-x`/`-y` execution adds only the command's own output.
2. Nothing is executed without `-x`, `-y`, or `gxx`.
3. Executed commands propagate their exit code.
4. No environment variable outside the §5.6 allow-list reaches the model; nothing
   secret-named ever does.
5. `gx` and `gxx` differ only in the default of `-y`.
6. Exactly one LLM request per generation; no hidden state between runs other
   than `~/.gx` and the prompt log.
7. All automation is reachable through a `make` target.
8. Files gx writes are owner-only (0600).
9. Tests never touch the network, real credentials, or the user's real home.
