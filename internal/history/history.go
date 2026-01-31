// Package history manages the conversation history for context-aware prompting.
package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

const (
	// DefaultHistoryFile is the default path for the history file.
	DefaultHistoryFile = ".gxhistory"
	// DefaultStagingFile is the default path for the staging file.
	DefaultStagingFile = ".gx"
	// DefaultMaxHistory is the default number of history entries to keep.
	DefaultMaxHistory = 10
)

// Entry represents a single prompt/response pair in the history.
type Entry struct {
	Prompt   string `json:"prompt"`
	Response string `json:"response"`
}

// Manager handles reading and writing history.
type Manager struct {
	historyPath string
	stagingPath string
	maxHistory  int
}

// NewManager creates a new history manager.
func NewManager() (*Manager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	maxHistory := DefaultMaxHistory
	if envMax := os.Getenv("GX_HISTORY"); envMax != "" {
		if n, err := strconv.Atoi(envMax); err == nil && n > 0 {
			maxHistory = n
		}
	}

	return &Manager{
		historyPath: filepath.Join(homeDir, DefaultHistoryFile),
		stagingPath: filepath.Join(homeDir, DefaultStagingFile),
		maxHistory:  maxHistory,
	}, nil
}

// Load reads the history from disk.
func (m *Manager) Load() ([]Entry, error) {
	data, err := os.ReadFile(m.historyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, fmt.Errorf("failed to read history: %w", err)
	}

	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		// If the file is corrupted, start fresh
		return []Entry{}, nil
	}

	return entries, nil
}

// Save writes the history to disk.
func (m *Manager) Save(entries []Entry) error {
	// Trim to max history
	if len(entries) > m.maxHistory {
		entries = entries[len(entries)-m.maxHistory:]
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal history: %w", err)
	}

	if err := os.WriteFile(m.historyPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write history: %w", err)
	}

	return nil
}

// Append adds a new entry to the history and saves it.
func (m *Manager) Append(prompt, response string) error {
	entries, err := m.Load()
	if err != nil {
		entries = []Entry{}
	}

	entries = append(entries, Entry{
		Prompt:   prompt,
		Response: response,
	})

	return m.Save(entries)
}

// GetRecentContext returns the last n entries for context (typically 2-3).
func (m *Manager) GetRecentContext(n int) ([]Entry, error) {
	entries, err := m.Load()
	if err != nil {
		return nil, err
	}

	if len(entries) <= n {
		return entries, nil
	}

	return entries[len(entries)-n:], nil
}

// StageCommand writes a command to the staging file.
func (m *Manager) StageCommand(command string) error {
	if err := os.WriteFile(m.stagingPath, []byte(command), 0600); err != nil {
		return fmt.Errorf("failed to stage command: %w", err)
	}
	return nil
}

// GetStagedCommand reads the staged command.
func (m *Manager) GetStagedCommand() (string, error) {
	data, err := os.ReadFile(m.stagingPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no staged command found (run gx with a prompt first)")
		}
		return "", fmt.Errorf("failed to read staged command: %w", err)
	}
	return string(data), nil
}

// Clear removes both history and staging files.
func (m *Manager) Clear() error {
	// Remove history file
	if err := os.Remove(m.historyPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove history: %w", err)
	}

	// Remove staging file
	if err := os.Remove(m.stagingPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove staging file: %w", err)
	}

	return nil
}

// StagingPath returns the path to the staging file.
func (m *Manager) StagingPath() string {
	return m.stagingPath
}
