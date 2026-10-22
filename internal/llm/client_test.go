package llm

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// stubTools makes exactly the given tools appear installed.
func stubTools(t *testing.T, installed ...string) {
	t.Helper()
	set := map[string]bool{}
	for _, name := range installed {
		set[name] = true
	}
	orig := lookPath
	lookPath = func(name string) (string, error) {
		if set[name] {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() { lookPath = orig })
}

func TestCleanResponse(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"plain", "ls -la", "ls -la"},
		{"whitespace", "\n  ls -la  \n", "ls -la"},
		{"fenced with lang", "```bash\nls -la\n```", "ls -la"},
		{"fenced no lang", "```\nfind . -name '*.go'\n```", "find . -name '*.go'"},
		{"fenced multiline", "```sh\n# step\nls\npwd\n```\n", "# step\nls\npwd"},
		{"inline backticks", "`ls -la`", "ls -la"},
		{"command substitution kept", "echo `date`", "echo `date`"},
		{"two inline spans kept", "`a` and `b`", "`a` and `b`"},
		{"unterminated fence kept", "```bash\nls", "```bash\nls"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanResponse(tt.in); got != tt.want {
				t.Errorf("cleanResponse(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDetectShell(t *testing.T) {
	tests := []struct {
		name                     string
		shell, psModule, comspec string
		want                     string
	}{
		{"zsh path", "/bin/zsh", "", "", "zsh"},
		{"fish path", "/usr/local/bin/fish", "", "", "fish"},
		{"SHELL beats PSModulePath", "/bin/bash", "C:\\ps", "", "bash"},
		{"powershell before comspec", "", "C:\\ps", "C:\\Windows\\system32\\cmd.exe", "powershell"},
		{"cmd", "", "", "C:\\Windows\\system32\\cmd.exe", "cmd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SHELL", tt.shell)
			t.Setenv("PSModulePath", tt.psModule)
			t.Setenv("ComSpec", tt.comspec)
			if got := detectShell(); got != tt.want {
				t.Errorf("detectShell() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectShellFallback(t *testing.T) {
	t.Setenv("SHELL", "")
	t.Setenv("PSModulePath", "")
	t.Setenv("ComSpec", "")
	want := "bash"
	if runtime.GOOS == "windows" {
		want = "powershell"
	}
	if got := detectShell(); got != want {
		t.Errorf("detectShell() = %q, want %q", got, want)
	}
}

func TestSystemInstructionPerShell(t *testing.T) {
	stubTools(t)
	tests := []struct {
		shell    string
		contains []string
		excludes []string
	}{
		{"zsh", []string{"ready-to-run zsh command", "trailing backslash", "No comments."}, []string{"REM"}},
		{"powershell", []string{"trailing backtick", "never use REM", "# comment saying why"}, nil},
		{"cmd", []string{"trailing caret", "REM comment saying why"}, []string{"backslash"}},
		{"fish", []string{"set VAR value"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			c := &Client{shell: tt.shell, platform: "linux/amd64"}
			got := c.buildSystemInstruction()
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("missing %q in:\n%s", s, got)
				}
			}
			for _, s := range tt.excludes {
				if strings.Contains(got, s) {
					t.Errorf("unexpected %q in:\n%s", s, got)
				}
			}
		})
	}
}

func TestSystemInstructionVerbose(t *testing.T) {
	stubTools(t)
	quiet := (&Client{shell: "bash", platform: "linux/amd64"}).buildSystemInstruction()
	loud := (&Client{shell: "bash", platform: "linux/amd64", verbose: true}).buildSystemInstruction()
	if !strings.Contains(quiet, "No comments.") {
		t.Error("non-verbose prompt should forbid comments")
	}
	if !strings.Contains(loud, "comment before each step") || strings.Contains(loud, "No comments.") {
		t.Errorf("verbose prompt should request comments:\n%s", loud)
	}
}

func TestSystemInstructionPlatformNotes(t *testing.T) {
	stubTools(t)
	mac := (&Client{shell: "zsh", platform: "darwin/arm64"}).buildSystemInstruction()
	if !strings.Contains(mac, "BSD") {
		t.Error("darwin prompt should mention BSD userland")
	}
	wsl := (&Client{shell: "bash", platform: "wsl2/amd64"}).buildSystemInstruction()
	if !strings.Contains(wsl, "/mnt/") {
		t.Error("wsl2 prompt should mention /mnt drive mounts")
	}
	linux := (&Client{shell: "bash", platform: "linux/amd64"}).buildSystemInstruction()
	if strings.Contains(linux, "BSD") || strings.Contains(linux, "/mnt/") {
		t.Error("plain linux prompt should carry no platform note")
	}
}

func TestCollectEnvironment(t *testing.T) {
	stubTools(t, "jq", "rg")
	t.Setenv("HOME", "/home/tester")
	t.Setenv("USERPROFILE", "C:\\Users\\tester")
	t.Setenv("USER", "tester")
	t.Setenv("USERNAME", "tester")
	t.Setenv("AWS_PROFILE", "dev")
	t.Setenv("OPENROUTER_API_KEY", "sk-should-never-appear")
	t.Setenv("GITHUB_TOKEN", "ghp-should-never-appear")

	got := collectEnvironment()

	for _, want := range []string{"- cwd: ", "- AWS_PROFILE: dev", "- installed: rg, jq"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, secret := range []string{"sk-should-never-appear", "ghp-should-never-appear", "- PATH:"} {
		if strings.Contains(got, secret) {
			t.Errorf("context leaked %q:\n%s", secret, got)
		}
	}
}

func TestRedact(t *testing.T) {
	for _, name := range []string{"API_KEY", "github_token", "DB_PASSWORD", "MY_SECRET", "AUTH_HEADER", "CREDENTIALS"} {
		if got := redact(name, "v"); got != "[REDACTED]" {
			t.Errorf("redact(%q) = %q, want [REDACTED]", name, got)
		}
	}
	if got := redact("AWS_PROFILE", "dev"); got != "dev" {
		t.Errorf("redact(AWS_PROFILE) = %q, want dev", got)
	}
}

func TestTruncateIsRuneSafe(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate short = %q", got)
	}
	got := truncate("héllo wörld", 4)
	if got != "héll (truncated)" {
		t.Errorf("truncate = %q, want %q", got, "héll (truncated)")
	}
}

func TestBuildPromptFormat(t *testing.T) {
	stubTools(t)
	c := &Client{shell: "bash", platform: "linux/amd64"}
	got := c.BuildPrompt("list files")
	if !strings.HasPrefix(got, "SYSTEM:\n") || !strings.HasSuffix(got, "\n\n---\n\nUSER:\nlist files") {
		t.Errorf("unexpected -p format:\n%s", got)
	}
}

func TestPromptLogPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	t.Setenv("GX_PROMPT_OUTPUT", "")
	if got, _ := promptLogPath(); got != filepath.Join(home, ".gxprompt") {
		t.Errorf("default path = %q", got)
	}
	t.Setenv("GX_PROMPT_OUTPUT", "~/logs/p.txt")
	if got, _ := promptLogPath(); got != filepath.Join(home, "logs", "p.txt") {
		t.Errorf("tilde path = %q", got)
	}
	t.Setenv("GX_PROMPT_OUTPUT", "/var/tmp/x")
	if got, _ := promptLogPath(); got != "/var/tmp/x" {
		t.Errorf("absolute path = %q", got)
	}
}

func TestWritePromptLogPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions")
	}
	path := filepath.Join(t.TempDir(), "gxprompt")
	t.Setenv("GX_PROMPT_OUTPUT", path)

	// A log left behind by an older version with 0644 must be tightened.
	if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	c := &Client{}
	c.writePromptLog("sys", "user prompt", "ls")

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("prompt log mode = %o, want 600", perm)
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{"SYSTEM:\nsys", "USER:\nuser prompt", "RESPONSE:\nls"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("log missing %q:\n%s", want, data)
		}
	}
}

func TestWritePromptLogOmitsEmptyResponse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gxprompt")
	t.Setenv("GX_PROMPT_OUTPUT", path)
	(&Client{}).writePromptLog("sys", "p", "")
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "RESPONSE:") {
		t.Errorf("failed request should have no RESPONSE section:\n%s", data)
	}
}
