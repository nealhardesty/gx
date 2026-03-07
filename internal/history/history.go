// Package history manages the staged command file for gx.
package history

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	// DefaultStagingFile is the default path for the staging file.
	DefaultStagingFile = ".gx"
)

// Manager handles reading and writing the staged command.
type Manager struct {
	stagingPath string
}

// NewManager creates a new history manager.
func NewManager() (*Manager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	return &Manager{
		stagingPath: filepath.Join(homeDir, DefaultStagingFile),
	}, nil
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

// Clear removes the staging file.
func (m *Manager) Clear() error {
	if err := os.Remove(m.stagingPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove staging file: %w", err)
	}
	return nil
}

// StagingPath returns the path to the staging file.
func (m *Manager) StagingPath() string {
	return m.stagingPath
}
