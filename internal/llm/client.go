// Package llm provides the LLM client for shell command generation.
// It wraps easy-llm-wrapper with gx-specific system prompt logic,
// shell/platform detection, and environment collection.
package llm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	elw "github.com/nealhardesty/easy-llm-wrapper"
)

// Config holds configuration for the LLM client.
type Config struct {
	Verbose bool
	Debug   bool
}

// Client wraps easy-llm-wrapper with gx-specific system prompt and context logic.
type Client struct {
	elw      *elw.Client
	verbose  bool
	debug    bool
	shell    string
	platform string
}

// lookPath is a variable so tests can control which tools appear installed.
var lookPath = exec.LookPath

// NewClient creates a new Client, auto-detecting provider from environment variables.
// Provider priority: claude CLI on PATH > OPENROUTER_API_KEY > OLLAMA_HOST.
// Model priority: GX_MODEL > MODEL > provider default.
func NewClient(cfg Config) (*Client, error) {
	elwCfg, err := elw.ConfigFromEnv()
	if err != nil {
		return nil, err
	}
	if model := os.Getenv("GX_MODEL"); model != "" {
		elwCfg.Model = model
	}
	elwCfg.Debug = cfg.Debug

	elwClient, err := elw.NewClientWithConfig(elwCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	c := &Client{
		elw:      elwClient,
		verbose:  cfg.Verbose,
		debug:    cfg.Debug,
		shell:    DetectShell(),
		platform: detectPlatform(),
	}

	c.debugf("provider: %s", c.elw.Provider())
	c.debugf("model:    %s", c.elw.Model())
	c.debugf("shell:    %s", c.shell)
	c.debugf("platform: %s", c.platform)

	return c, nil
}

// debugf writes a debug line to stderr, prefixing every line with "# ".
// Does nothing when debug mode is off.
func (c *Client) debugf(format string, args ...any) {
	if !c.debug {
		return
	}
	msg := fmt.Sprintf(format, args...)
	for _, line := range strings.Split(msg, "\n") {
		fmt.Fprintf(os.Stderr, "# %s\n", line)
	}
}

// Generate generates a shell command from a natural language prompt.
func (c *Client) Generate(ctx context.Context, prompt string) (string, error) {
	systemPrompt := c.buildSystemInstruction()

	c.debugf("prompt:\n%s", prompt)

	messages := []elw.Message{
		{Role: elw.RoleUser, Parts: []elw.Part{elw.TextPart(prompt)}},
	}

	c.debugf("sending request...")
	resp, err := c.elw.Complete(ctx, elw.Request{
		System:   systemPrompt,
		Messages: messages,
	})
	if err != nil {
		c.writePromptLog(systemPrompt, prompt, "")
		return "", fmt.Errorf("failed to generate response: %w", err)
	}

	result := cleanResponse(resp.Text)
	c.debugf("response:\n%s", result)
	c.writePromptLog(systemPrompt, prompt, result)
	if result == "" {
		return "", errors.New("model returned an empty response")
	}
	return result, nil
}

// BuildPrompt returns the full prompt that would be sent to the LLM, for the -p flag.
func (c *Client) BuildPrompt(prompt string) string {
	return formatTranscript(c.buildSystemInstruction(), prompt, "")
}

// formatTranscript renders a SYSTEM/USER[/RESPONSE] transcript, shared by -p and the prompt log.
func formatTranscript(system, prompt, response string) string {
	parts := []string{
		"SYSTEM:\n" + system,
		"USER:\n" + prompt,
	}
	if response != "" {
		parts = append(parts, "RESPONSE:\n"+response)
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// fenceLangs are info strings that may directly follow an opening fence.
var fenceLangs = map[string]bool{
	"bash": true, "sh": true, "zsh": true, "fish": true, "shell": true, "console": true,
	"powershell": true, "pwsh": true, "ps1": true, "cmd": true, "bat": true, "batch": true,
}

// cleanResponse trims the model output and unwraps Markdown that models emit
// despite instructions: the first fenced code block anywhere in the reply
// (prose around it is dropped), or a single line wrapped in one pair of backticks.
func cleanResponse(text string) string {
	s := strings.TrimSpace(text)

	if start := strings.Index(s, "```"); start >= 0 {
		body := s[start+3:]
		if end := strings.Index(body, "```"); end >= 0 {
			body = body[:end]
		}
		if nl := strings.IndexByte(body, '\n'); nl >= 0 {
			// Multi-line fence: the rest of the opening line is the info string.
			body = body[nl+1:]
		} else if fields := strings.Fields(body); len(fields) > 1 && fenceLangs[strings.ToLower(fields[0])] {
			// Single-line fence such as ```bash ls```.
			body = strings.TrimSpace(body)[len(fields[0]):]
		}
		return strings.TrimSpace(body)
	}

	if len(s) >= 2 && s[0] == '`' && s[len(s)-1] == '`' && !strings.Contains(s, "\n") &&
		strings.Count(s, "`") == 2 {
		return strings.TrimSpace(s[1 : len(s)-1])
	}

	return s
}

// shellProfile captures the syntax facts the model most often gets wrong per shell.
type shellProfile struct {
	comment      string // line-comment token
	continuation string // how to continue a long line
	note         string // extra shell-specific guidance; may be empty
}

func profileFor(shell string) shellProfile {
	switch shell {
	case "powershell", "pwsh":
		return shellProfile{
			comment:      "#",
			continuation: "a trailing backtick (`)",
			note:         "Use PowerShell cmdlets and syntax. Comments start with #; never use REM.",
		}
	case "cmd":
		return shellProfile{
			comment:      "REM",
			continuation: "a trailing caret (^)",
			note:         "Use cmd.exe syntax and built-ins; chain steps with &&.",
		}
	case "fish":
		return shellProfile{
			comment:      "#",
			continuation: "a trailing backslash (\\)",
			note:         "Use fish syntax: set VAR value (not VAR=value), begin/end blocks, no heredocs.",
		}
	default:
		return shellProfile{
			comment:      "#",
			continuation: "a trailing backslash (\\)",
		}
	}
}

// platformNote returns userland caveats for the platform, or "".
func platformNote(platform string) string {
	switch {
	case strings.HasPrefix(platform, "darwin/"):
		return "macOS ships BSD userland, not GNU: use BSD-compatible flags (e.g. sed -i '', stat -f, date -v)."
	case strings.HasPrefix(platform, "wsl2/"):
		return "This is Linux under WSL2; Windows drives are mounted at /mnt/<letter>."
	}
	return ""
}

// buildSystemInstruction creates the system instruction for the LLM.
func (c *Client) buildSystemInstruction() string {
	p := profileFor(c.shell)

	commentRule := "No comments."
	if c.verbose {
		commentRule = fmt.Sprintf("Put a short %s comment before each step explaining what it does.", p.comment)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "You turn a request into a ready-to-run %s command on %s.\n\n", c.shell, c.platform)
	fmt.Fprintf(&b, "Reply with the command text only: no prose, no Markdown, no code fences. "+
		"Your reply is pasted into %s and executed exactly as written.\n", c.shell)
	fmt.Fprintf(&b, "- %s\n", commentRule)
	fmt.Fprintf(&b, "- Split long commands with %s; put independent steps on separate lines.\n", p.continuation)
	b.WriteString("- Prefer tools listed as installed. Do not add destructive options (force, recursive delete, overwrite) the request does not ask for.\n")
	fmt.Fprintf(&b, "- If no command can do it, reply only with a %s comment saying why.\n", p.comment)
	for _, note := range []string{p.note, platformNote(c.platform)} {
		if note != "" {
			fmt.Fprintf(&b, "- %s\n", note)
		}
	}

	if ctx := collectContext(); ctx != "" {
		b.WriteString("\nContext:\n")
		b.WriteString(ctx)
	}

	return strings.TrimRight(b.String(), "\n")
}

// envVar names a context variable; the first non-empty name in names wins.
type envVar struct {
	label string
	names []string
}

var (
	unixEnv = []envVar{
		{"HOME", []string{"HOME"}},
		{"USER", []string{"USER", "LOGNAME"}},
	}
	windowsEnv = []envVar{
		{"USERPROFILE", []string{"USERPROFILE"}},
		{"USERNAME", []string{"USERNAME"}},
		{"TEMP", []string{"TEMP", "TMP"}},
	}
	commonEnv = []envVar{
		{"GOPATH", []string{"GOPATH"}},
		{"VIRTUAL_ENV", []string{"VIRTUAL_ENV"}},
		{"DOCKER_HOST", []string{"DOCKER_HOST"}},
		{"KUBECONFIG", []string{"KUBECONFIG"}},
		{"AWS_PROFILE", []string{"AWS_PROFILE"}},
		{"AWS_REGION", []string{"AWS_REGION"}},
		{"GCP_PROJECT", []string{"GCP_PROJECT"}},
	}

	// probeTools are looked up on PATH so the model knows which non-default tools it may use.
	// The lookup is a stat per PATH entry; nothing is executed.
	probeTools = []string{
		"brew", "apt", "dnf", "pacman", "apk", "winget", "choco",
		"git", "docker", "podman", "kubectl",
		"rg", "fd", "jq", "yq", "fzf",
		"gsed", "gawk", "python3", "node", "go",
	}

	sensitivePatterns = []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "AUTH", "CREDENTIAL"}
)

// collectContext is a variable so tests can pin the context block.
var collectContext = collectEnvironment

// maxEnvValue caps any single context value, in runes.
const maxEnvValue = 200

// collectEnvironment formats the context lines (cwd, allow-listed env, installed tools).
func collectEnvironment() string {
	var lines []string

	if wd, err := os.Getwd(); err == nil {
		lines = append(lines, "- cwd: "+truncate(wd, maxEnvValue))
	}

	vars := unixEnv
	if runtime.GOOS == "windows" {
		vars = windowsEnv
	}
	for _, v := range append(vars[:len(vars):len(vars)], commonEnv...) {
		for _, name := range v.names {
			if val := os.Getenv(name); val != "" {
				lines = append(lines, fmt.Sprintf("- %s: %s", v.label, redact(v.label, truncate(val, maxEnvValue))))
				break
			}
		}
	}

	if tools := installedTools(); len(tools) > 0 {
		lines = append(lines, "- installed: "+strings.Join(tools, ", "))
	}

	return strings.Join(lines, "\n")
}

// installedTools returns the probeTools found on PATH, in list order.
func installedTools() []string {
	var found []string
	for _, t := range probeTools {
		if _, err := lookPath(t); err == nil {
			found = append(found, t)
		}
	}
	return found
}

// redact hides values whose variable name suggests a secret.
func redact(name, val string) string {
	upper := strings.ToUpper(name)
	for _, p := range sensitivePatterns {
		if strings.Contains(upper, p) {
			return "[REDACTED]"
		}
	}
	return val
}

// truncate shortens val to at most maxRunes runes without splitting a UTF-8 sequence.
func truncate(val string, maxRunes int) string {
	if utf8.RuneCountInString(val) <= maxRunes {
		return val
	}
	runes := []rune(val)
	return string(runes[:maxRunes]) + " (truncated)"
}

// DetectShell reports the shell gx generates for and executes with:
// sh, bash, zsh, fish, ..., powershell, pwsh or cmd.
func DetectShell() string {
	return shellFromEnv(runtime.GOOS, os.Getenv("SHELL"), os.Getenv("PSModulePath"), os.Getenv("ComSpec"))
}

// shellFromEnv is DetectShell's pure core.
func shellFromEnv(goos, shell, psModulePath, comspec string) string {
	if shell != "" {
		// Git Bash/MSYS may hand us either separator, and an .exe suffix in any case.
		base := shell[strings.LastIndexAny(shell, `/\`)+1:]
		if strings.HasSuffix(strings.ToLower(base), ".exe") {
			base = base[:len(base)-len(".exe")]
		}
		return strings.ToLower(base)
	}

	if goos != "windows" {
		return "sh"
	}

	// PSModulePath is a machine-wide variable on modern Windows, so its mere
	// presence proves nothing. A PowerShell host prepends the user's own module
	// directory (Documents\PowerShell for pwsh 7+, Documents\WindowsPowerShell for
	// Windows PowerShell 5.1); cmd.exe sees only the registry value without it.
	mods := strings.ToLower(psModulePath)
	switch {
	case strings.Contains(mods, `documents\powershell\modules`):
		return "pwsh"
	case strings.Contains(mods, `documents\windowspowershell\modules`):
		return "powershell"
	case strings.Contains(strings.ToLower(comspec), "cmd.exe"):
		return "cmd"
	}
	return "powershell"
}

// detectPlatform detects the current platform, including WSL2.
func detectPlatform() string {
	goos := runtime.GOOS
	arch := runtime.GOARCH

	if goos == "linux" {
		if data, err := exec.Command("uname", "-r").Output(); err == nil {
			kernel := strings.ToLower(string(data))
			if strings.Contains(kernel, "microsoft") || strings.Contains(kernel, "wsl") {
				return fmt.Sprintf("wsl2/%s", arch)
			}
		}
	}

	return fmt.Sprintf("%s/%s", goos, arch)
}

// promptLogPath returns where the prompt log is written, or "" when logging is off.
// Defaults to ~/.gxprompt; overridden by GX_PROMPT_OUTPUT (a leading ~/ is expanded,
// "off" disables the log, a relative path is relative to the working directory).
func promptLogPath() (string, error) {
	outputPath := os.Getenv("GX_PROMPT_OUTPUT")
	if strings.EqualFold(outputPath, "off") {
		return "", nil
	}
	if outputPath != "" && !strings.HasPrefix(outputPath, "~/") {
		return outputPath, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if outputPath == "" {
		return filepath.Join(homeDir, ".gxprompt"), nil
	}
	return filepath.Join(homeDir, strings.TrimPrefix(outputPath, "~/")), nil
}

// writePromptLog writes the prompt log to a file for debugging.
// The log holds the prompt, stdin, cwd and environment context, so a regular file
// is made owner-only (0600) before anything is written — including files left at
// 0644 by older versions. Non-regular targets (/dev/null, a tty) are left alone.
func (c *Client) writePromptLog(system, prompt, response string) {
	outputPath, err := promptLogPath()
	if err != nil || outputPath == "" {
		return
	}
	f, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	if info, err := f.Stat(); err == nil && info.Mode().IsRegular() {
		_ = f.Chmod(0600)
	}
	_, _ = f.WriteString(formatTranscript(system, prompt, response) + "\n")
}
