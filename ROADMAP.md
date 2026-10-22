# gx — Roadmap

Source of truth for what does **not** exist yet. What *does* exist is in
[SPEC.md](SPEC.md). When something here ships, delete it from this file and
document it in SPEC.md (and README.md if users touch it) in the same change.

Effort: **S** ≈ an hour or two · **M** ≈ a day · **L** ≈ several days.

---

## 1. Known gaps and debts

Things that are wrong or missing in the current design, found while writing SPEC.md.

| Item | Why | Effort |
|------|-----|--------|
| **CI** | No automated checks. Add a GitHub Actions workflow on push/PR that runs `make check` on Linux, macOS and Windows (AGENTS.md: CI calls make targets only). | S |
| **Request timeout + Ctrl-C** | Requests use `context.Background()`; a hung provider hangs gx. Use `signal.NotifyContext` plus a `GX_TIMEOUT` (default ~60s). | S |
| **Explicit provider choice** | The `claude` CLI on `PATH` always wins; the only way around it is a `PATH` hack. Add `GX_PROVIDER=claude\|openrouter\|ollama` (best done as `LLM_PROVIDER` in easy-llm-wrapper, honoured by `ConfigFromEnv`). | S–M |
| **Flags anywhere** | `gx "list files" -y` treats `-y` as prompt text — the most common usage trap. Parse known flags from any position, with `--` to force the rest to be prompt text. | S |
| **`-p` without a provider** | `-p` never sends anything but still fails when no provider is configured. Build the prompt without constructing a provider client. | S |
| **Parent-shell detection** | Shell comes from `$SHELL` (login shell). Running `bash` inside a zsh login gets zsh syntax. Inspect the parent process (`ps -o comm= -p $PPID`, or `/proc/$PPID/comm` on Linux) and fall back to `$SHELL`. | S |
| **Claude CLI prompt in argv** | easy-llm-wrapper passes the prompt and system prompt as `claude` arguments, visible in `ps` to other local users. Pipe the prompt on stdin instead (wrapper change). | S |
| **`-D` prefix consistency** | With the `claude` provider, wrapper debug lines are prefixed `[llm debug]`, breaking "every line starts with `#`". Let the wrapper take a debug writer/prefix. | S |
| **Windows test coverage** | CLI tests skip on Windows. Add PowerShell/cmd variants once CI has a Windows runner. | M |
| **Naming leftovers** | `internal/history` holds only the staging file (rename to `staging`); root `version.go` re-export is unused. | S |

---

## 2. Next up — high value, low risk

### Shell integration: put the command *on your prompt line* (M)
`eval "$(gx --init zsh)"` installs a widget/function so that a keybinding (e.g.
`Ctrl-G`) sends the current line to gx and replaces it with the generated
command — editable, reviewed with your own eyes, run with Enter, and saved in
your **real shell history**. zsh: `zle` widget + `BUFFER`; bash: `bind -x` +
`READLINE_LINE`; fish: `commandline -r`; PowerShell: `PSReadLine`
`[Microsoft.PowerShell.PSConsoleReadLine]::Replace`. This is the single biggest
usability upgrade: it removes the need for `-x` for most users and is safer than
`gxx`.

### Interactive confirm (`-i`) (S)
Show the command, then `[Enter] run · e edit ($EDITOR) · r refine · c copy · q quit`.
A safe middle ground between `gx` + `gx -x` and `gxx`.

### Danger guard for YOLO (S–M)
Before `-y`/`gxx` executes, scan the command locally for high-risk patterns —
`rm -rf` on `/`, `~` or `*`; `dd of=/dev/…`; `mkfs`; `chmod -R 777`; `curl|sh`;
`git push --force`; `> /dev/sd*`; `DROP TABLE` — and require an interactive
confirmation (or `--force`). Cheap, deterministic, and doesn't trust the model.

### Syntax pre-check with one retry (S)
Run `bash -n` / `zsh -n` / `fish --no-execute` on the generated command. On a
syntax error, retry once with the error message appended. Never execute
something that doesn't parse.

### Refine the last command (`-r`) (S)
`gx -r "but only .go files"` sends the staged command plus the new instruction
as one single-shot request. Keeps gx stateless (no history file) while covering
the main reason history existed.

### Copy to clipboard (`-C`) (S)
`pbcopy` / `wl-copy` / `xclip` / `clip.exe` / `Set-Clipboard`, picked by platform.

---

## 3. Bigger ideas

### Explain mode (`-e`) (S)
Reverse direction: `gx -e 'find . -mtime +30 -delete'` returns a commented
breakdown of an existing command. Different system prompt, same plumbing.

### Fix the last failure (`--fix`) (M, needs shell integration)
The shell hook records the last command and its exit code; `gx --fix` sends
both (and, optionally, captured stderr) and proposes a corrected command.

### Project-aware context (S)
Cheap file-existence checks in cwd add one context line such as
`project: go module, Makefile, git (branch main)` — so "run the tests" becomes
`make test`, not `go test ./...`. Checks: `Makefile`, `go.mod`, `package.json`
(+ lockfile → npm/pnpm/yarn), `Cargo.toml`, `pyproject.toml`, `.git`,
`docker-compose.yml`, `justfile`. No commands run except optionally `git branch --show-current`.

### Configurable tool probe (S)
`GX_TOOLS=+terraform,+gcloud,-node` to extend or trim `probeTools`.

### Streaming output (S)
Stream tokens to a TTY for slow local models; still stage only the final, cleaned text.

### Multiple candidates (`-n 3`) (M)
Ask for N alternatives (e.g. portable vs. fastest vs. modern-tool) and pick one
interactively.

### Config file (M)
`~/.config/gx/config.toml` for provider, model, timeout, default verbosity, tool
probe, danger-guard policy. Environment variables keep priority.

### Opt-in audit log (S)
`GX_AUDIT=1` appends every *executed* command with timestamp, cwd and exit code
to `~/.local/state/gx/audit.log` (0600). Useful for "what did gxx do yesterday?".

### Response cache (S)
Hash of (system prompt, user prompt, model) → response, short TTL. Makes
repeated prompts instant and free; bypass with `--no-cache`.

---

## 4. Quality and distribution

### Prompt evaluation harness (`make eval`) (M)
A golden set of ~50 prompts × {bash, zsh, fish, powershell, cmd} × {darwin,
linux}. For each response, score: parses (`-n`), no Markdown, uses BSD flags on
darwin, no destructive flags unless asked, uses installed tools. Run against any
provider to compare models and to catch prompt regressions before release. The
prompt in §5.3 of SPEC.md was tuned by hand; this makes tuning measurable.

### Small-model mode (S, after eval)
If the eval shows small Ollama models need it, add 2–3 few-shot examples only
when the model name suggests ≤ ~8B parameters, keeping the default prompt lean.

### Release binaries (M)
GoReleaser via a `make release` target: archives for darwin/linux/windows ×
amd64/arm64, checksums, a Homebrew tap and a Scoop manifest. Today the only
install path is `go install`.

### Shell completions (S)
`gx --completion zsh|bash|fish|powershell` for flags.
