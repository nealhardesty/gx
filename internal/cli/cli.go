// Package cli provides the shared CLI logic for gx and gxx commands.
package cli

import (
	"context"
	"errors"
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
	return run(opts, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
}

// run is Run with injectable arguments and streams, so it can be tested.
func run(opts Options, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gx", flag.ContinueOnError)
	fs.SetOutput(stderr)

	executeFlag := fs.Bool("x", false, "Execute the staged command from ~/.gx")
	yoloFlag := fs.Bool("y", opts.ForceYolo, "YOLO mode - generate and execute immediately")
	verboseFlag := fs.Bool("v", false, "Verbose mode - include detailed comments")
	clearFlag := fs.Bool("c", false, "Clear staged command")
	printPromptFlag := fs.Bool("p", false, "Print the prompt that would be sent to the LLM (don't send it)")
	debugFlag := fs.Bool("D", false, "Debug mode - dump all activity to stderr (lines prefixed with #)")
	versionFlag := fs.Bool("version", false, "Show version information")

	fs.Usage = func() {
		fmt.Fprintf(stderr, "gx - Convert natural language to shell commands\n\n")
		fmt.Fprintf(stderr, "Usage: gx [options] [prompt] [-]\n")
		fmt.Fprintf(stderr, "       (options must come before the prompt)\n\n")
		fmt.Fprintf(stderr, "Options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(stderr, "\nStdin Support:\n")
		fmt.Fprintf(stderr, "  -               Read additional input from stdin and append to prompt\n")
		fmt.Fprintf(stderr, "\nExamples:\n")
		fmt.Fprintf(stderr, "  gx \"find all large files over 100mb\"\n")
		fmt.Fprintf(stderr, "  gx -x                    # Execute staged command\n")
		fmt.Fprintf(stderr, "  gx -y \"list docker containers\"\n")
		fmt.Fprintf(stderr, "  gx -p \"list files\"       # Print prompt without sending\n")
		fmt.Fprintf(stderr, "  cat error.log | gx - \"explain this error\"   # Read from stdin\n")
		fmt.Fprintf(stderr, "  docker ps | gx -         # Use only stdin as prompt\n")
		fmt.Fprintf(stderr, "\nDebug:\n")
		fmt.Fprintf(stderr, "  -D                  Dump all activity to stderr (provider, prompt, response)\n")
		fmt.Fprintf(stderr, "                      All lines are prefixed with # (shell comment syntax)\n")
		fmt.Fprintf(stderr, "                      Implies -v (verbose comments in output)\n")
		fmt.Fprintf(stderr, "\nEnvironment:\n")
		fmt.Fprintf(stderr, "  (claude CLI on PATH is used first when present)\n")
		fmt.Fprintf(stderr, "  OPENROUTER_API_KEY  OpenRouter API key (enables OpenRouter provider)\n")
		fmt.Fprintf(stderr, "  OLLAMA_HOST         Ollama base URL, e.g. http://localhost:11434\n")
		fmt.Fprintf(stderr, "  GX_MODEL            Model override (takes priority over MODEL)\n")
		fmt.Fprintf(stderr, "  MODEL               Model override\n")
		fmt.Fprintf(stderr, "  GX_PROMPT_OUTPUT    Path to write prompt logs (default: ~/.gxprompt)\n")
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	// -D implies -v
	if *debugFlag {
		*verboseFlag = true
	}

	// Handle version flag
	if *versionFlag {
		fmt.Fprintf(stdout, "gx version %s\n", opts.Version)
		return 0
	}

	// Initialize history manager
	histMgr, err := history.NewManager()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	// Handle clear flag
	if *clearFlag {
		if err := histMgr.Clear(); err != nil {
			fmt.Fprintf(stderr, "Error clearing: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, "Staged command cleared.")
		return 0
	}

	// Handle execute flag
	if *executeFlag {
		exitCode, err := executeStaged(histMgr, stdin, stdout, stderr)
		if err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return exitCode
	}

	prompt, err := buildPrompt(fs.Args(), stdin)
	if err != nil {
		fmt.Fprintf(stderr, "Error reading stdin: %v\n", err)
		return 1
	}
	if prompt == "" {
		fs.Usage()
		return 1
	}

	client, err := llm.NewClient(llm.Config{
		Verbose: *verboseFlag,
		Debug:   *debugFlag,
	})
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	// Handle print prompt flag
	if *printPromptFlag {
		fmt.Fprintln(stdout, client.BuildPrompt(prompt))
		return 0
	}

	// Generate command
	command, err := client.Generate(context.Background(), prompt)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	// Output the command
	fmt.Fprintln(stdout, command)

	// Stage the command
	if err := histMgr.StageCommand(command); err != nil {
		fmt.Fprintf(stderr, "Warning: failed to stage command: %v\n", err)
	}

	// YOLO mode - execute immediately
	if *yoloFlag {
		fmt.Fprintln(stderr, "\n--- Executing ---")
		exitCode, err := executeCommand(command, stdin, stdout, stderr)
		if err != nil {
			fmt.Fprintf(stderr, "Execution error: %v\n", err)
			return 1
		}
		return exitCode
	}

	return 0
}

// buildPrompt joins the positional args into a prompt. A literal "-" among them
// reads stdin to EOF and appends it after a "---" separator (or uses it alone).
func buildPrompt(args []string, stdin io.Reader) (string, error) {
	hasStdinFlag := false
	promptArgs := []string{}
	for _, arg := range args {
		if arg == "-" {
			hasStdinFlag = true
		} else {
			promptArgs = append(promptArgs, arg)
		}
	}

	prompt := strings.Join(promptArgs, " ")
	if !hasStdinFlag {
		return prompt, nil
	}

	stdinBytes, err := io.ReadAll(stdin)
	if err != nil {
		return "", err
	}
	stdinContent := strings.TrimSpace(string(stdinBytes))

	switch {
	case prompt == "":
		return stdinContent, nil
	case stdinContent == "":
		return prompt, nil
	default:
		return prompt + "\n\n---\n\n" + stdinContent, nil
	}
}

// executeStaged executes the command staged in ~/.gx.
func executeStaged(histMgr *history.Manager, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	command, err := histMgr.GetStagedCommand()
	if err != nil {
		return 1, err
	}

	fmt.Fprintf(stderr, "Executing: %s\n", command)
	fmt.Fprintln(stderr, "---")

	return executeCommand(command, stdin, stdout, stderr)
}

// executeCommand executes a shell command and returns the exit code from the subprocess.
// stdin, stdout and stderr are connected directly to the child.
func executeCommand(command string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
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

	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	if err == nil {
		// Command succeeded
		return 0, nil
	}

	// Check if it's an ExitError (command ran but failed)
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode(), nil
	}

	// Some other error occurred (couldn't start command, etc.)
	return 1, err
}
