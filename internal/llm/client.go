// Package llm provides the LLM client for shell command generation.
// It wraps easy-llm-wrapper with gx-specific system prompt logic,
// shell/platform detection, and environment collection.
package llm

import (
	"context"
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
		shell:    detectShell(),
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

// cleanResponse trims the model output and strips a Markdown code fence or
// inline backticks wrapping the whole reply, which models emit despite instructions.
func cleanResponse(text string) string {
	s := strings.TrimSpace(text)

	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) >= 2 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
			return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
		}
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

	if ctx := collectEnvironment(); ctx != "" {
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

// detectShell detects the current shell.
func detectShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		parts := strings.Split(shell, "/")
		return strings.TrimSuffix(parts[len(parts)-1], ".exe")
	}

	// Check PSModulePath before ComSpec — PSModulePath is set in PowerShell
	// but ComSpec is often also set even when running in PowerShell.
	if os.Getenv("PSModulePath") != "" {
		return "powershell"
	}

	if comspec := os.Getenv("ComSpec"); comspec != "" {
		if strings.Contains(strings.ToLower(comspec), "cmd.exe") {
			return "cmd"
		}
	}

	if runtime.GOOS == "windows" {
		return "powershell"
	}
	return "bash"
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

// promptLogPath returns where the prompt log is written.
// Defaults to ~/.gxprompt; overridden by GX_PROMPT_OUTPUT (a leading ~/ is expanded).
func promptLogPath() (string, error) {
	outputPath := os.Getenv("GX_PROMPT_OUTPUT")
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
// The log holds the prompt, stdin, cwd and environment context, so it is owner-only (0600);
// existing files from older versions (0644) are tightened on the next write.
func (c *Client) writePromptLog(system, prompt, response string) {
	outputPath, err := promptLogPath()
	if err != nil {
		return
	}
	content := formatTranscript(system, prompt, response)
	if err := os.WriteFile(outputPath, []byte(content+"\n"), 0600); err != nil {
		return
	}
	_ = os.Chmod(outputPath, 0600)
}
