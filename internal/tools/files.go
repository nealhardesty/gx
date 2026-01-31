package tools

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// executePwd returns the current working directory.
func executePwd() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	return cwd, nil
}

// executeLs lists files in the given directory.
func executeLs(path string, recursive bool) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("failed to access path: %w", err)
	}

	if !info.IsDir() {
		// If it's a file, just return its name
		return info.Name(), nil
	}

	var result strings.Builder

	if recursive {
		err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			relPath, err := filepath.Rel(path, p)
			if err != nil {
				relPath = p
			}
			if relPath == "." {
				return nil
			}

			info, err := d.Info()
			if err != nil {
				return nil // Skip files we can't stat
			}

			prefix := ""
			if d.IsDir() {
				prefix = "d "
			} else {
				prefix = "- "
			}
			result.WriteString(fmt.Sprintf("%s%s (%d bytes)\n", prefix, relPath, info.Size()))
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("failed to walk directory: %w", err)
		}
	} else {
		entries, err := os.ReadDir(path)
		if err != nil {
			return "", fmt.Errorf("failed to read directory: %w", err)
		}

		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				continue // Skip files we can't stat
			}

			prefix := ""
			if entry.IsDir() {
				prefix = "d "
			} else {
				prefix = "- "
			}
			result.WriteString(fmt.Sprintf("%s%s (%d bytes)\n", prefix, entry.Name(), info.Size()))
		}
	}

	return strings.TrimSuffix(result.String(), "\n"), nil
}

// executeStat returns detailed file information.
func executeStat(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("failed to stat file: %w", err)
	}

	fileType := "file"
	if info.IsDir() {
		fileType = "directory"
	} else if info.Mode()&os.ModeSymlink != 0 {
		fileType = "symlink"
	}

	return fmt.Sprintf(
		"Name: %s\nType: %s\nSize: %d bytes\nMode: %s\nModified: %s",
		info.Name(),
		fileType,
		info.Size(),
		info.Mode().String(),
		info.ModTime().Format("2006-01-02 15:04:05"),
	), nil
}

// executeCat reads and returns file contents.
func executeCat(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("failed to access file: %w", err)
	}

	if info.IsDir() {
		return "", fmt.Errorf("cannot cat a directory")
	}

	// Limit file size to prevent reading huge files
	const maxSize = 100 * 1024 // 100KB
	if info.Size() > maxSize {
		return "", fmt.Errorf("file too large (max %d bytes, got %d bytes)", maxSize, info.Size())
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return string(content), nil
}
