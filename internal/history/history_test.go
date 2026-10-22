package history

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if want := filepath.Join(home, DefaultStagingFile); m.StagingPath() != want {
		t.Fatalf("StagingPath = %q, want %q", m.StagingPath(), want)
	}
	return m
}

func TestStageAndGet(t *testing.T) {
	m := newTestManager(t)

	if err := m.StageCommand("ls -la"); err != nil {
		t.Fatalf("StageCommand: %v", err)
	}
	got, err := m.GetStagedCommand()
	if err != nil {
		t.Fatalf("GetStagedCommand: %v", err)
	}
	if got != "ls -la" {
		t.Errorf("staged = %q, want %q", got, "ls -la")
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(m.StagingPath())
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("staging file mode = %o, want 600", perm)
		}
	}
}

func TestStageOverwrites(t *testing.T) {
	m := newTestManager(t)
	_ = m.StageCommand("first")
	_ = m.StageCommand("second")
	if got, _ := m.GetStagedCommand(); got != "second" {
		t.Errorf("staged = %q, want %q", got, "second")
	}
}

func TestGetMissing(t *testing.T) {
	m := newTestManager(t)
	_, err := m.GetStagedCommand()
	if err == nil || !strings.Contains(err.Error(), "no staged command") {
		t.Errorf("err = %v, want 'no staged command'", err)
	}
}

func TestClear(t *testing.T) {
	m := newTestManager(t)

	// Clearing when nothing is staged is not an error.
	if err := m.Clear(); err != nil {
		t.Fatalf("Clear on empty: %v", err)
	}

	_ = m.StageCommand("echo hi")
	if err := m.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := os.Stat(m.StagingPath()); !os.IsNotExist(err) {
		t.Errorf("staging file still exists after Clear (err=%v)", err)
	}
}
