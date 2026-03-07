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

// NewClient creates a new Client, auto-detecting provider from environment variables.
// Provider priority: claude CLI > OPENROUTER_API_KEY > OLLAMA_HOST.
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

	result := strings.TrimSpace(resp.Text)
	c.debugf("response:\n%s", result)
	c.writePromptLog(systemPrompt, prompt, result)
	return result, nil
}

// BuildPrompt returns the full prompt that would be sent to the LLM, for the -p flag.
func (c *Client) BuildPrompt(prompt string) string {
	system := c.buildSystemInstruction()
	parts := []string{
		fmt.Sprintf("SYSTEM:\n%s", system),
		fmt.Sprintf("USER:\n%s", prompt),
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// buildSystemInstruction creates the system instruction for the LLM.
func (c *Client) buildSystemInstruction() string {
	commentSyntax := "#"
	commentWarning := ""
	if c.shell == "powershell" || c.shell == "pwsh" {
		commentSyntax = "#"
		commentWarning = "CRITICAL: For PowerShell, use # for comments. NEVER use REM (REM is only for CMD).\n\n"
	} else if c.shell == "cmd" {
		commentSyntax = "REM"
		commentWarning = "For CMD, use REM for comments.\n\n"
	}

	verboseInstruction := "Do not include comments unless absolutely necessary for understanding."
	if c.verbose {
		verboseInstruction = "Include helpful comments explaining what each part of the command does."
	}

	envSection := c.collectEnvironment()
	envText := ""
	if envSection != "" {
		envText = "\n\nENVIRONMENT:\n" + envSection
	}

	return fmt.Sprintf(`You are a shell command generator. Your task is to convert natural language requests into executable shell commands.

%sCRITICAL RULES:
1. Return ONLY the shell command(s) - no explanations, no markdown, no backticks.
2. Do not wrap output in code blocks or use markdown formatting.
3. If you need to add comments, use the appropriate syntax for the shell: %s
4. %s
5. The command must be directly executable - copy-paste ready. This is an absolute requirement no matter what.
6. For multi-line commands, use appropriate line continuation for the shell.
7. If a task cannot be accomplished with a shell command, explain briefly using shell comments.

PAY ATTENTION:
Again, the command must be directly executable - copy-paste ready. This is an absolute requirement no matter what.

CONTEXT:
- Shell: %s
- Platform: %s
- Operating System: %s%s`,
		commentWarning, commentSyntax, verboseInstruction,
		c.shell, c.platform, runtime.GOOS, envText)
}

// collectEnvironment collects and formats relevant environment variables for the system prompt.
func (c *Client) collectEnvironment() string {
	var envVars []string

	getEnv := func(key string) (string, bool) {
		val := os.Getenv(key)
		if val == "" {
			return "", false
		}
		return val, true
	}

	sanitize := func(key, val string) string {
		keyUpper := strings.ToUpper(key)
		sensitivePatterns := []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "AUTH", "CREDENTIAL"}
		for _, pattern := range sensitivePatterns {
			if strings.Contains(keyUpper, pattern) {
				return "[REDACTED]"
			}
		}
		return val
	}

	truncate := func(val string, maxLen int) string {
		if len(val) <= maxLen {
			return val
		}
		return val[:maxLen] + " (truncated)"
	}

	// gx config vars
	if val, ok := getEnv("GX_MODEL"); ok {
		envVars = append(envVars, fmt.Sprintf("- GX_MODEL: %s", val))
	}
	if val, ok := getEnv("GX_PROMPT_OUTPUT"); ok {
		envVars = append(envVars, fmt.Sprintf("- GX_PROMPT_OUTPUT: %s", val))
	}

	// Platform-specific vars
	if runtime.GOOS == "windows" {
		if val, ok := getEnv("USERPROFILE"); ok {
			envVars = append(envVars, fmt.Sprintf("- USERPROFILE: %s", val))
		}
		if val, ok := getEnv("USERNAME"); ok {
			envVars = append(envVars, fmt.Sprintf("- USERNAME: %s", val))
		}
		if val, ok := getEnv("ComSpec"); ok {
			envVars = append(envVars, fmt.Sprintf("- ComSpec: %s", val))
		}
		if val, ok := getEnv("PSModulePath"); ok {
			envVars = append(envVars, fmt.Sprintf("- PSModulePath: %s", truncate(val, 200)))
		}
		if val, ok := getEnv("TEMP"); ok {
			envVars = append(envVars, fmt.Sprintf("- TEMP: %s", val))
		} else if val, ok := getEnv("TMP"); ok {
			envVars = append(envVars, fmt.Sprintf("- TMP: %s", val))
		}
	} else {
		if val, ok := getEnv("HOME"); ok {
			envVars = append(envVars, fmt.Sprintf("- HOME: %s", val))
		}
		if val, ok := getEnv("USER"); ok {
			envVars = append(envVars, fmt.Sprintf("- USER: %s", val))
		} else if val, ok := getEnv("LOGNAME"); ok {
			envVars = append(envVars, fmt.Sprintf("- LOGNAME: %s", val))
		}
		if val, ok := getEnv("SHELL"); ok {
			envVars = append(envVars, fmt.Sprintf("- SHELL: %s", val))
		}
		if val, ok := getEnv("PWD"); ok {
			envVars = append(envVars, fmt.Sprintf("- PWD: %s", val))
		}
	}

	// Common vars
	if val, ok := getEnv("PATH"); ok {
		envVars = append(envVars, fmt.Sprintf("- PATH: %s", truncate(val, 300)))
	}
	if val, ok := getEnv("GOPATH"); ok {
		envVars = append(envVars, fmt.Sprintf("- GOPATH: %s", val))
	}
	if val, ok := getEnv("GOROOT"); ok {
		envVars = append(envVars, fmt.Sprintf("- GOROOT: %s", val))
	}
	if val, ok := getEnv("DOCKER_HOST"); ok {
		envVars = append(envVars, fmt.Sprintf("- DOCKER_HOST: %s", val))
	}
	if val, ok := getEnv("KUBECONFIG"); ok {
		envVars = append(envVars, fmt.Sprintf("- KUBECONFIG: %s", val))
	}
	if val, ok := getEnv("AWS_PROFILE"); ok {
		envVars = append(envVars, fmt.Sprintf("- AWS_PROFILE: %s", val))
	}
	if val, ok := getEnv("AWS_REGION"); ok {
		envVars = append(envVars, fmt.Sprintf("- AWS_REGION: %s", val))
	}
	if val, ok := getEnv("GCP_PROJECT"); ok {
		envVars = append(envVars, fmt.Sprintf("- GCP_PROJECT: %s", sanitize("GCP_PROJECT", val)))
	}

	if len(envVars) == 0 {
		return ""
	}
	return strings.Join(envVars, "\n")
}

// detectShell detects the current shell.
func detectShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		parts := strings.Split(shell, "/")
		return parts[len(parts)-1]
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

// writePromptLog writes the prompt log to a file for debugging.
// Defaults to ~/.gxprompt; overridden by GX_PROMPT_OUTPUT.
func (c *Client) writePromptLog(system, prompt, response string) {
	outputPath := os.Getenv("GX_PROMPT_OUTPUT")
	if outputPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return
		}
		outputPath = filepath.Join(homeDir, ".gxprompt")
	} else if strings.HasPrefix(outputPath, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return
		}
		outputPath = filepath.Join(homeDir, strings.TrimPrefix(outputPath, "~/"))
	}

	parts := []string{
		fmt.Sprintf("SYSTEM:\n%s", system),
		fmt.Sprintf("USER:\n%s", prompt),
	}
	if response != "" {
		parts = append(parts, fmt.Sprintf("RESPONSE:\n%s", response))
	}

	content := strings.Join(parts, "\n---\n\n")
	_ = os.WriteFile(outputPath, []byte(content), 0644)
}
