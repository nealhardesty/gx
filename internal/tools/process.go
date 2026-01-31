package tools

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// executePs lists running processes.
func executePs() (string, error) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		// Use PowerShell to get process list
		cmd = exec.Command("powershell", "-Command",
			"Get-Process | Select-Object Id, ProcessName, CPU, WorkingSet64 | Format-Table -AutoSize | Out-String -Width 200")
	case "darwin":
		// macOS ps command
		cmd = exec.Command("ps", "aux")
	default:
		// Linux and other Unix-like systems
		cmd = exec.Command("ps", "aux")
	}

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to execute ps: %w", err)
	}

	// Truncate output if too long
	result := string(output)
	const maxLen = 8000
	if len(result) > maxLen {
		lines := strings.Split(result, "\n")
		var truncated strings.Builder
		for _, line := range lines {
			if truncated.Len()+len(line)+1 > maxLen {
				truncated.WriteString("\n... (output truncated)")
				break
			}
			truncated.WriteString(line)
			truncated.WriteString("\n")
		}
		result = truncated.String()
	}

	return strings.TrimSpace(result), nil
}

// executeUptime returns system uptime information.
func executeUptime() (string, error) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		// Use PowerShell to get uptime
		cmd = exec.Command("powershell", "-Command",
			"$os = Get-CimInstance Win32_OperatingSystem; $uptime = (Get-Date) - $os.LastBootUpTime; \"System up for $($uptime.Days) days, $($uptime.Hours) hours, $($uptime.Minutes) minutes\"")
	case "darwin", "linux":
		cmd = exec.Command("uptime")
	default:
		// Fallback for other systems
		cmd = exec.Command("uptime")
	}

	output, err := cmd.Output()
	if err != nil {
		// Fallback: just return current time if uptime command fails
		return fmt.Sprintf("Current time: %s (uptime command unavailable)", time.Now().Format("2006-01-02 15:04:05")), nil
	}

	return strings.TrimSpace(string(output)), nil
}
