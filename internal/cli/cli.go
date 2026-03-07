// Package cli provides the shared CLI logic for gx and gxx commands.
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/nealhardesty/gx/internal/history"
	"github.com/nealhardesty/gx/internal/llm"
)

// Options configures the CLI behavior.
type Options struct {
	// ForceYolo automatically sets the -y flag to true
	ForceYolo bool
	// Version is the application version string
	Version string
}

// Run executes the CLI with the given options and returns the exit code.
func Run(opts Options) int {
	// Define flags
	executeFlag := flag.Bool("x", false, "Execute the staged command from ~/.gx")
	yoloFlag := flag.Bool("y", opts.ForceYolo, "YOLO mode - generate and execute immediately")
	verboseFlag := flag.Bool("v", false, "Verbose mode - include detailed comments")
	clearFlag := flag.Bool("c", false, "Clear history and staged commands")
	printPromptFlag := flag.Bool("p", false, "Print the prompt that would be sent to the LLM (don't send it)")
	debugFlag := flag.Bool("D", false, "Debug mode - dump all activity to stderr (lines prefixed with #)")
	versionFlag := flag.Bool("version", false, "Show version information")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "gx - Convert natural language to shell commands\n\n")
		fmt.Fprintf(os.Stderr, "Usage: gx [options] [prompt] [-]\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nStdin Support:\n")
		fmt.Fprintf(os.Stderr, "  -               Read additional input from stdin and append to prompt\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  gx \"find all large files over 100mb\"\n")
		fmt.Fprintf(os.Stderr, "  gx -x                    # Execute staged command\n")
		fmt.Fprintf(os.Stderr, "  gx -y \"list docker containers\"\n")
		fmt.Fprintf(os.Stderr, "  gx -p \"list files\"       # Print prompt without sending\n")
		fmt.Fprintf(os.Stderr, "  cat error.log | gx - \"explain this error\"   # Read from stdin\n")
		fmt.Fprintf(os.Stderr, "  docker ps | gx -         # Use only stdin as prompt\n")
		fmt.Fprintf(os.Stderr, "\nDebug:\n")
		fmt.Fprintf(os.Stderr, "  -D                  Dump all activity to stderr (provider, prompt, response)\n")
		fmt.Fprintf(os.Stderr, "                      All lines are prefixed with # (shell comment syntax)\n")
		fmt.Fprintf(os.Stderr, "                      Implies -v (verbose comments in output)\n")
		fmt.Fprintf(os.Stderr, "\nEnvironment:\n")
		fmt.Fprintf(os.Stderr, "  OPENROUTER_API_KEY  OpenRouter API key (enables OpenRouter provider)\n")
		fmt.Fprintf(os.Stderr, "  OLLAMA_HOST         Ollama base URL, e.g. http://localhost:11434\n")
		fmt.Fprintf(os.Stderr, "  GX_MODEL            Model override (takes priority over MODEL)\n")
		fmt.Fprintf(os.Stderr, "  MODEL               Model override\n")
		fmt.Fprintf(os.Stderr, "  GX_HISTORY          Max history entries (default: 10)\n")
		fmt.Fprintf(os.Stderr, "  GX_PROMPT_OUTPUT    Path to write prompt logs (default: ~/.gxprompt)\n")
	}

	flag.Parse()

	// -D implies -v
	if *debugFlag {
		*verboseFlag = true
	}

	// Handle version flag
	if *versionFlag {
		fmt.Printf("gx version %s\n", opts.Version)
		return 0
	}

	// Initialize history manager
	histMgr, err := history.NewManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Handle clear flag
	if *clearFlag {
		if err := histMgr.Clear(); err != nil {
			fmt.Fprintf(os.Stderr, "Error clearing: %v\n", err)
			return 1
		}
		fmt.Println("History and staged commands cleared.")
		return 0
	}

	// Handle execute flag
	if *executeFlag {
		exitCode, err := executeStaged(histMgr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		return exitCode
	}

	// Get prompt from arguments
	args := flag.Args()

	// Check if "-" is in the arguments to read from stdin
	hasStdinFlag := false
	promptArgs := []string{}
	for _, arg := range args {
		if arg == "-" {
			hasStdinFlag = true
		} else {
			promptArgs = append(promptArgs, arg)
		}
	}

	// Build the prompt from non-"-" arguments
	prompt := strings.Join(promptArgs, " ")

	// Read from stdin if "-" was specified
	if hasStdinFlag {
		stdinBytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			return 1
		}
		stdinContent := strings.TrimSpace(string(stdinBytes))

		// Append stdin content to the prompt
		if prompt == "" {
			prompt = stdinContent
		} else {
			prompt = prompt + "\n\n---\n\n" + stdinContent
		}
	}

	if prompt == "" {
		flag.Usage()
		return 1
	}

	// Handle print prompt flag
	if *printPromptFlag {
		client, err := llm.NewClient(llm.Config{
			Verbose: *verboseFlag,
			Debug:   *debugFlag,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}

		// Get recent history for context
		histContext, err := histMgr.GetRecentContext(3)
		if err != nil {
			// Non-fatal, continue without history
			histContext = nil
		}

		// Build and print the prompt
		fullPrompt := client.BuildPrompt(prompt, histContext)
		fmt.Println(fullPrompt)
		return 0
	}

	// Generate command
	ctx := context.Background()
	command, err := generateCommand(ctx, prompt, *verboseFlag, *debugFlag, histMgr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Output the command
	fmt.Println(command)

	// Stage the command
	if err := histMgr.StageCommand(command); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to stage command: %v\n", err)
	}

	// Save to history
	if err := histMgr.Append(prompt, command); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save history: %v\n", err)
	}

	// YOLO mode - execute immediately
	if *yoloFlag {
		fmt.Fprintln(os.Stderr, "\n--- Executing ---")
		exitCode, err := executeCommand(command)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Execution error: %v\n", err)
			return 1
		}
		return exitCode
	}

	return 0
}

// generateCommand uses the LLM to generate a shell command from the prompt.
func generateCommand(ctx context.Context, prompt string, verbose, debug bool, histMgr *history.Manager) (string, error) {
	histContext, err := histMgr.GetRecentContext(3)
	if err != nil {
		histContext = nil
	}

	client, err := llm.NewClient(llm.Config{
		Verbose: verbose,
		Debug:   debug,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create client: %w", err)
	}

	return client.Generate(ctx, prompt, histContext)
}

// executeStaged executes the command staged in ~/.gx.
func executeStaged(histMgr *history.Manager) (int, error) {
	command, err := histMgr.GetStagedCommand()
	if err != nil {
		return 1, err
	}

	fmt.Printf("Executing: %s\n", command)
	fmt.Println("---")

	return executeCommand(command)
}

// executeCommand executes a shell command and returns the exit code from the subprocess.
// stdout and stderr are streamed directly to the parent process.
func executeCommand(command string) (int, error) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		// Try PowerShell first, fall back to cmd
		if os.Getenv("PSModulePath") != "" {
			cmd = exec.Command("powershell", "-Command", command)
		} else {
			cmd = exec.Command("cmd", "/C", command)
		}
	default:
		// Unix-like systems
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		cmd = exec.Command(shell, "-c", command)
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err == nil {
		// Command succeeded
		return 0, nil
	}

	// Check if it's an ExitError (command ran but failed)
	if exitError, ok := err.(*exec.ExitError); ok {
		return exitError.ExitCode(), nil
	}

	// Some other error occurred (couldn't start command, etc.)
	return 1, err
}
