// Package gemini provides the Vertex AI Gemini client for command generation.
package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"cloud.google.com/go/vertexai/genai"

	"github.com/nealhardesty/gx/internal/history"
	"github.com/nealhardesty/gx/internal/tools"
)

const (
	// DefaultModel is the default Gemini model to use.
	DefaultModel = "gemini-2.5-flash-lite"
	// DefaultLocation is the default Vertex AI location.
	DefaultLocation = "us-central1"
)

// Client wraps the Vertex AI Gemini client.
type Client struct {
	client   *genai.Client
	model    *genai.GenerativeModel
	tools    *tools.Registry
	verbose  bool
	shell    string
	platform string
}

// Config holds configuration for the Gemini client.
type Config struct {
	ProjectID string
	Location  string
	Model     string
	Verbose   bool
	NoTools   bool
}

// NewClient creates a new Gemini client.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.ProjectID == "" {
		// Try to get project ID from gcloud
		projectID, err := getDefaultProject()
		if err != nil {
			return nil, fmt.Errorf("no project ID specified and failed to get default: %w", err)
		}
		cfg.ProjectID = projectID
	}

	if cfg.Location == "" {
		cfg.Location = DefaultLocation
	}

	if cfg.Model == "" {
		cfg.Model = os.Getenv("GX_MODEL")
		if cfg.Model == "" {
			cfg.Model = DefaultModel
		}
	}

	client, err := genai.NewClient(ctx, cfg.ProjectID, cfg.Location)
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	toolRegistry := tools.NewRegistry(!cfg.NoTools)
	model := client.GenerativeModel(cfg.Model)

	// Configure the model
	model.SetTemperature(0.1) // Low temperature for deterministic output
	model.SetTopP(0.95)

	// Set up tools if enabled
	if toolRegistry.IsEnabled() {
		model.Tools = toolRegistry.GetToolDefinitions()
	}

	// Detect shell and platform
	shell := detectShell()
	platform := detectPlatform()

	c := &Client{
		client:   client,
		model:    model,
		tools:    toolRegistry,
		verbose:  cfg.Verbose,
		shell:    shell,
		platform: platform,
	}

	// Set system instruction
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{
			genai.Text(c.buildSystemInstruction()),
		},
	}

	return c, nil
}

// Close closes the underlying client.
func (c *Client) Close() error {
	return c.client.Close()
}

// BuildPrompt builds the full prompt that would be sent to the LLM without actually sending it.
// This is useful for debugging and the -p flag.
func (c *Client) BuildPrompt(prompt string, historyContext []history.Entry) string {
	var parts []string

	// Add system instruction
	systemInstruction := c.buildSystemInstruction()
	parts = append(parts, fmt.Sprintf("SYSTEM INSTRUCTION:\n%s", systemInstruction))

	// Add history context
	if len(historyContext) > 0 {
		histText := "HISTORY CONTEXT:\n"
		for _, entry := range historyContext {
			histText += fmt.Sprintf("User: %s\nAssistant: %s\n", entry.Prompt, entry.Response)
		}
		parts = append(parts, histText)
	}

	// Add the current user prompt
	parts = append(parts, fmt.Sprintf("USER PROMPT:\n%s", prompt))

	return strings.Join(parts, "\n\n")
}

// Generate generates a shell command from a natural language prompt.
func (c *Client) Generate(ctx context.Context, prompt string, historyContext []history.Entry) (string, error) {
	// Track prompts for debugging output
	var promptLog []string

	// Add system instruction to log
	systemInstruction := c.buildSystemInstruction()
	promptLog = append(promptLog, fmt.Sprintf("SYSTEM INSTRUCTION:\n%s", systemInstruction))

	// Add history context to log
	if len(historyContext) > 0 {
		histText := "HISTORY CONTEXT:\n"
		for _, entry := range historyContext {
			histText += fmt.Sprintf("User: %s\nAssistant: %s\n", entry.Prompt, entry.Response)
		}
		promptLog = append(promptLog, histText)
	}

	chat := c.model.StartChat()

	// If we have history, add it to the chat
	if len(historyContext) > 0 {
		for _, entry := range historyContext {
			chat.History = append(chat.History,
				&genai.Content{
					Role:  "user",
					Parts: []genai.Part{genai.Text(entry.Prompt)},
				},
				&genai.Content{
					Role:  "model",
					Parts: []genai.Part{genai.Text(entry.Response)},
				},
			)
		}
	}

	// Add initial user prompt to log
	promptLog = append(promptLog, fmt.Sprintf("USER PROMPT:\n%s", prompt))

	// Send the message
	resp, err := chat.SendMessage(ctx, genai.Text(prompt))
	if err != nil {
		// Write prompt log even on error
		c.writePromptLog(promptLog)
		return "", fmt.Errorf("failed to generate response: %w", err)
	}

	// Process the response, handling tool calls
	result, err := c.processResponse(ctx, chat, resp, promptLog)
	
	// Write prompt log
	c.writePromptLog(promptLog)
	
	return result, err
}

// formatToolArgs formats tool arguments as a function call parameter list.
func (c *Client) formatToolArgs(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	var parts []string
	for k, v := range args {
		var valStr string
		switch val := v.(type) {
		case string:
			valStr = fmt.Sprintf("%q", val)
		case bool:
			valStr = fmt.Sprintf("%t", val)
		case float64:
			valStr = fmt.Sprintf("%g", val)
		default:
			valStr = fmt.Sprintf("%v", val)
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, valStr))
	}
	return strings.Join(parts, ", ")
}

// formatToolResult formats tool result for verbose output, truncating if too long.
func (c *Client) formatToolResult(result string) string {
	const maxLen = 200
	if len(result) <= maxLen {
		return result
	}
	// Truncate and add ellipsis
	truncated := result[:maxLen]
	// Try to break at a newline if near the limit
	if idx := strings.LastIndex(truncated, "\n"); idx > maxLen-50 {
		truncated = truncated[:idx]
	}
	return truncated + "... (truncated)"
}

// processResponse handles the response, including any tool calls.
func (c *Client) processResponse(ctx context.Context, chat *genai.ChatSession, resp *genai.GenerateContentResponse, promptLog []string) (string, error) {
	turnNum := 1
	for {
		if len(resp.Candidates) == 0 {
			return "", fmt.Errorf("no response candidates")
		}

		candidate := resp.Candidates[0]
		if candidate.Content == nil {
			return "", fmt.Errorf("empty response content")
		}

		// Check for function calls
		var functionCalls []*genai.FunctionCall
		var textParts []string

		for _, part := range candidate.Content.Parts {
			switch p := part.(type) {
			case genai.FunctionCall:
				functionCalls = append(functionCalls, &p)
			case genai.Text:
				textParts = append(textParts, string(p))
			}
		}

		// If there are function calls, execute them and continue
		if len(functionCalls) > 0 {
			// Log the function calls
			funcCallText := fmt.Sprintf("TURN %d - MODEL RESPONSE (FUNCTION CALLS):\n", turnNum)
			for _, fc := range functionCalls {
				argsJSON, _ := json.MarshalIndent(fc.Args, "", "  ")
				funcCallText += fmt.Sprintf("Function: %s\nArgs: %s\n", fc.Name, string(argsJSON))
			}
			promptLog = append(promptLog, funcCallText)

			// Verbose output: show that function calls were received
			if c.verbose {
				fmt.Fprintf(os.Stderr, "[tool] Received %d function call(s)\n", len(functionCalls))
			}

			var functionResponses []genai.Part
			funcResponseText := fmt.Sprintf("TURN %d - TOOL RESPONSES:\n", turnNum)
			for _, fc := range functionCalls {
				name, args, err := tools.ParseFunctionCall(fc)
				if err != nil {
					if c.verbose {
						fmt.Fprintf(os.Stderr, "[tool] %s() - Error parsing: %s\n", fc.Name, err.Error())
					}
					funcResponseText += fmt.Sprintf("Function: %s - Error: %s\n", fc.Name, err.Error())
					functionResponses = append(functionResponses, genai.FunctionResponse{
						Name:     fc.Name,
						Response: map[string]any{"error": err.Error()},
					})
					continue
				}

				// Verbose output: show tool call with arguments
				if c.verbose {
					argsStr := c.formatToolArgs(args)
					fmt.Fprintf(os.Stderr, "[tool] %s(%s)\n", name, argsStr)
				}

				result, err := c.tools.ExecuteTool(name, args)
				if err != nil {
					if c.verbose {
						fmt.Fprintf(os.Stderr, "[tool] %s -> error: %s\n", name, err.Error())
					}
					funcResponseText += fmt.Sprintf("Function: %s - Error: %s\n", name, err.Error())
					functionResponses = append(functionResponses, genai.FunctionResponse{
						Name:     fc.Name,
						Response: map[string]any{"error": err.Error()},
					})
				} else {
					// Verbose output: show result (truncated if too long)
					if c.verbose {
						resultStr := c.formatToolResult(result)
						fmt.Fprintf(os.Stderr, "[tool] %s -> %s\n", name, resultStr)
					}
					resultJSON, _ := json.MarshalIndent(result, "", "  ")
					funcResponseText += fmt.Sprintf("Function: %s\nResult: %s\n", name, string(resultJSON))
					functionResponses = append(functionResponses, genai.FunctionResponse{
						Name:     fc.Name,
						Response: map[string]any{"result": result},
					})
				}
			}
			promptLog = append(promptLog, funcResponseText)

			// Send function responses back
			var err error
			resp, err = chat.SendMessage(ctx, functionResponses...)
			if err != nil {
				return "", fmt.Errorf("failed to send function responses: %w", err)
			}
			turnNum++
			continue
		}

		// No more function calls, log final response and return
		if len(textParts) > 0 {
			finalResponse := strings.TrimSpace(strings.Join(textParts, "\n"))
			promptLog = append(promptLog, fmt.Sprintf("TURN %d - MODEL RESPONSE (FINAL):\n%s", turnNum, finalResponse))
		}
		return strings.TrimSpace(strings.Join(textParts, "\n")), nil
	}
}

// buildSystemInstruction creates the system instruction based on shell and platform.
func (c *Client) buildSystemInstruction() string {
	commentSyntax := "#"
	commentWarning := ""
	if c.shell == "powershell" || c.shell == "pwsh" {
		commentSyntax = "#"
		commentWarning = "CRITICAL: For PowerShell, use # for comments. NEVER use REM (REM is only for CMD)."
	} else if c.shell == "cmd" {
		commentSyntax = "REM"
		commentWarning = "For CMD, use REM for comments."
	}

	verboseInstruction := ""
	if c.verbose {
		verboseInstruction = "Include helpful comments explaining what each part of the command does."
	} else {
		verboseInstruction = "Do not include comments unless absolutely necessary for understanding."
	}

	var warningSection string
	if commentWarning != "" {
		warningSection = commentWarning + "\n\n"
	}
	
	instruction := fmt.Sprintf(`You are a shell command generator. Your task is to convert natural language requests into executable shell commands.

%sCRITICAL RULES:
1. Return ONLY the shell command(s) - no explanations, no markdown, no backticks.
2. Do not wrap output in code blocks or use markdown formatting.
3. If you need to add comments, use the appropriate syntax for the shell: %s
4. %s
5. The command must be directly executable - copy-paste ready.
6. For multi-line commands, use appropriate line continuation for the shell.
7. If a task cannot be accomplished with a shell command, explain briefly using shell comments.

CONTEXT:
- Shell: %s
- Platform: %s
- Operating System: %s

You have access to tools for gathering context about the file system and running processes. Use them when needed to provide accurate commands.`, warningSection, commentSyntax, verboseInstruction, c.shell, c.platform, runtime.GOOS)
	
	return instruction
}

// detectShell detects the current shell.
func detectShell() string {
	// Check SHELL environment variable (Unix)
	if shell := os.Getenv("SHELL"); shell != "" {
		// Extract just the shell name
		parts := strings.Split(shell, "/")
		return parts[len(parts)-1]
	}

	// Check PSModulePath for PowerShell first (Windows)
	// This must be checked before ComSpec because ComSpec is often set
	// even when running PowerShell
	if os.Getenv("PSModulePath") != "" {
		return "powershell"
	}

	// Check ComSpec for Windows CMD (only if PowerShell not detected)
	if comspec := os.Getenv("ComSpec"); comspec != "" {
		if strings.Contains(strings.ToLower(comspec), "cmd.exe") {
			return "cmd"
		}
	}

	// Default based on OS
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	return "bash"
}

// detectPlatform detects the current platform.
func detectPlatform() string {
	os := runtime.GOOS
	arch := runtime.GOARCH

	// Check for WSL
	if os == "linux" {
		if data, err := exec.Command("uname", "-r").Output(); err == nil {
			if strings.Contains(strings.ToLower(string(data)), "microsoft") ||
				strings.Contains(strings.ToLower(string(data)), "wsl") {
				return fmt.Sprintf("wsl2/%s", arch)
			}
		}
	}

	return fmt.Sprintf("%s/%s", os, arch)
}

// writePromptLog writes the prompt log to a file if GX_PROMPT_OUTPUT is set.
// If GX_PROMPT_OUTPUT is not set, defaults to ~/.gxprompt.
func (c *Client) writePromptLog(promptLog []string) {
	outputPath := os.Getenv("GX_PROMPT_OUTPUT")
	
	// If not set, default to ~/.gxprompt
	if outputPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return // Silently fail if we can't get home directory
		}
		outputPath = filepath.Join(homeDir, ".gxprompt")
	} else {
		// Expand ~ if present in the env var value
		if strings.HasPrefix(outputPath, "~") {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return // Silently fail if we can't get home directory
			}
			// Handle both ~ and ~/ cases
			if outputPath == "~" {
				outputPath = homeDir
			} else if strings.HasPrefix(outputPath, "~/") {
				outputPath = filepath.Join(homeDir, strings.TrimPrefix(outputPath, "~/"))
			} else {
				outputPath = filepath.Join(homeDir, strings.TrimPrefix(outputPath, "~"))
			}
		}
	}

	// Join all prompts with separator
	content := strings.Join(promptLog, "\n---\n\n")

	// Write to file (create or overwrite)
	if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
		// Silently fail - this is a debugging feature
		_ = err
	}
}

// getDefaultProject gets the default GCP project from gcloud config.
func getDefaultProject() (string, error) {
	cmd := exec.Command("gcloud", "config", "get-value", "project")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get default project: %w (ensure gcloud is installed and configured)", err)
	}

	project := strings.TrimSpace(string(output))
	if project == "" {
		return "", fmt.Errorf("no default project set (run: gcloud config set project PROJECT_ID)")
	}

	return project, nil
}
