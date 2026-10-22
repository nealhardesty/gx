package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// env isolates a test: temp HOME, prompt log in the temp dir, no claude CLI on
// PATH and no cloud key, so provider selection is fully controlled by the test.
type env struct {
	home    string
	logPath string
}

func setupEnv(t *testing.T) env {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("cli tests drive a POSIX shell")
	}
	home := t.TempDir()
	e := env{home: home, logPath: filepath.Join(home, "prompt.log")}
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/sh")
	t.Setenv("PATH", "/usr/bin:/bin") // must not contain the claude CLI
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OLLAMA_HOST", "")
	t.Setenv("MODEL", "")
	t.Setenv("GX_MODEL", "")
	t.Setenv("GX_PROMPT_OUTPUT", e.logPath)
	return e
}

// fakeOllama serves /api/chat with a fixed reply and records the last request.
func fakeOllama(t *testing.T, reply string) *map[string]any {
	t.Helper()
	var last map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&last)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "fake",
			"message": map[string]string{"role": "assistant", "content": reply},
			"done":    true,
		})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("OLLAMA_HOST", srv.URL)
	return &last
}

type result struct {
	code           int
	stdout, stderr string
}

func gx(t *testing.T, opts Options, stdin string, args ...string) result {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(opts, args, strings.NewReader(stdin), &out, &errb)
	return result{code, out.String(), errb.String()}
}

func TestVersion(t *testing.T) {
	setupEnv(t)
	r := gx(t, Options{Version: "9.9.9"}, "", "--version")
	if r.code != 0 || r.stdout != "gx version 9.9.9\n" {
		t.Errorf("got %+v", r)
	}
}

func TestHelpAndBadFlag(t *testing.T) {
	setupEnv(t)
	if r := gx(t, Options{}, "", "-h"); r.code != 0 || !strings.Contains(r.stderr, "Usage: gx") {
		t.Errorf("-h: %+v", r)
	}
	if r := gx(t, Options{}, "", "-zz"); r.code != 2 {
		t.Errorf("unknown flag exit = %d, want 2", r.code)
	}
}

func TestEmptyPromptShowsUsage(t *testing.T) {
	setupEnv(t)
	r := gx(t, Options{}, "")
	if r.code != 1 || !strings.Contains(r.stderr, "Usage: gx") {
		t.Errorf("got %+v", r)
	}
}

func TestNoProvider(t *testing.T) {
	setupEnv(t)
	r := gx(t, Options{}, "", "list files")
	if r.code != 1 || !strings.Contains(r.stderr, "no LLM provider configured") {
		t.Errorf("got %+v", r)
	}
}

func TestGenerateStagesAndLogs(t *testing.T) {
	e := setupEnv(t)
	last := fakeOllama(t, "```bash\nls -la\n```")

	r := gx(t, Options{}, "", "list", "files")
	if r.code != 0 {
		t.Fatalf("exit %d, stderr: %s", r.code, r.stderr)
	}
	if r.stdout != "ls -la\n" {
		t.Errorf("stdout = %q, want fences stripped", r.stdout)
	}

	staged, err := os.ReadFile(filepath.Join(e.home, ".gx"))
	if err != nil || string(staged) != "ls -la" {
		t.Errorf("staged = %q (err %v)", staged, err)
	}

	info, err := os.Stat(e.logPath)
	if err != nil {
		t.Fatalf("prompt log not written: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("prompt log mode = %o, want 600", perm)
	}

	msgs, _ := (*last)["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("want system+user messages, got %v", *last)
	}
	user, _ := msgs[1].(map[string]any)
	if user["content"] != "list files" {
		t.Errorf("user content = %v", user["content"])
	}
	system, _ := msgs[0].(map[string]any)
	if s, _ := system["content"].(string); !strings.Contains(s, "ready-to-run sh command") {
		t.Errorf("system prompt not shell-aware:\n%s", s)
	}
}

func TestStdinAppended(t *testing.T) {
	setupEnv(t)
	last := fakeOllama(t, "true")

	r := gx(t, Options{}, "  boom: error 42\n", "explain", "-")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	msgs, _ := (*last)["messages"].([]any)
	user, _ := msgs[1].(map[string]any)
	if user["content"] != "explain\n\n---\n\nboom: error 42" {
		t.Errorf("user content = %q", user["content"])
	}
}

func TestPrintPromptSendsNothing(t *testing.T) {
	setupEnv(t)
	last := fakeOllama(t, "should not be called")

	r := gx(t, Options{}, "", "-p", "list files")
	if r.code != 0 || !strings.HasPrefix(r.stdout, "SYSTEM:\n") || !strings.Contains(r.stdout, "USER:\nlist files") {
		t.Errorf("got %+v", r)
	}
	if *last != nil {
		t.Error("-p must not contact the provider")
	}
}

func TestYoloExecutesAndPropagatesExitCode(t *testing.T) {
	setupEnv(t)
	fakeOllama(t, "echo yolo-ran; exit 3")

	r := gx(t, Options{ForceYolo: true}, "", "do it")
	if r.code != 3 {
		t.Errorf("exit = %d, want 3 (child's code)", r.code)
	}
	if r.stdout != "echo yolo-ran; exit 3\nyolo-ran\n" {
		t.Errorf("stdout = %q", r.stdout)
	}
	if !strings.Contains(r.stderr, "--- Executing ---") {
		t.Errorf("stderr = %q", r.stderr)
	}
}

func TestFlagsAfterPromptAreText(t *testing.T) {
	setupEnv(t)
	last := fakeOllama(t, "echo should-not-run")

	r := gx(t, Options{}, "", "list files", "-y")
	if r.stdout != "echo should-not-run\n" {
		t.Errorf("-y after the prompt must not execute; stdout = %q", r.stdout)
	}
	msgs, _ := (*last)["messages"].([]any)
	user, _ := msgs[1].(map[string]any)
	if user["content"] != "list files -y" {
		t.Errorf("user content = %q", user["content"])
	}
}

func TestExecuteStaged(t *testing.T) {
	e := setupEnv(t)

	if r := gx(t, Options{}, "", "-x"); r.code != 1 || !strings.Contains(r.stderr, "no staged command") {
		t.Errorf("-x with nothing staged: %+v", r)
	}

	if err := os.WriteFile(filepath.Join(e.home, ".gx"), []byte("echo staged-ok"), 0600); err != nil {
		t.Fatal(err)
	}
	r := gx(t, Options{}, "", "-x")
	if r.code != 0 || r.stdout != "staged-ok\n" {
		t.Errorf("-x: %+v (stdout must contain only the command's output)", r)
	}
	if !strings.Contains(r.stderr, "Executing: echo staged-ok") {
		t.Errorf("stderr = %q", r.stderr)
	}
}

func TestClearBeatsExecute(t *testing.T) {
	e := setupEnv(t)
	staged := filepath.Join(e.home, ".gx")
	_ = os.WriteFile(staged, []byte("echo must-not-run"), 0600)

	r := gx(t, Options{}, "", "-c", "-x")
	if r.code != 0 || r.stdout != "Staged command cleared.\n" {
		t.Errorf("got %+v", r)
	}
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Error("staging file should be removed")
	}
}

func TestBuildPrompt(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		stdin string
		want  string
	}{
		{"args only", []string{"a", "b"}, "ignored", "a b"},
		{"stdin only", []string{"-"}, " data \n", "data"},
		{"dash position irrelevant", []string{"-", "a"}, "data", "a\n\n---\n\ndata"},
		{"empty stdin", []string{"a", "-"}, "   ", "a"},
		{"nothing", nil, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildPrompt(tt.args, strings.NewReader(tt.stdin))
			if err != nil || got != tt.want {
				t.Errorf("buildPrompt = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}
