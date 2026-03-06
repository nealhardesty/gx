// Package tools provides readonly tool implementations for file system and process operations.
// These are used to pre-collect system context injected into the LLM system prompt.
package tools

import "fmt"

// Registry holds all available tools and provides dispatch functionality.
type Registry struct {
	enabled bool
}

// NewRegistry creates a new tool registry.
func NewRegistry(enabled bool) *Registry {
	return &Registry{enabled: enabled}
}

// IsEnabled returns whether tools are enabled.
func (r *Registry) IsEnabled() bool {
	return r.enabled
}

// ExecuteTool executes a tool by name with the given arguments.
func (r *Registry) ExecuteTool(name string, args map[string]any) (string, error) {
	if !r.enabled {
		return "", fmt.Errorf("tools are disabled")
	}

	switch name {
	case "pwd":
		return executePwd()
	case "ls":
		path, _ := args["path"].(string)
		if path == "" {
			path = "."
		}
		recursive, _ := args["recursive"].(bool)
		return executeLs(path, recursive)
	case "stat":
		path, ok := args["path"].(string)
		if !ok || path == "" {
			return "", fmt.Errorf("stat requires a path argument")
		}
		return executeStat(path)
	case "cat":
		path, ok := args["path"].(string)
		if !ok || path == "" {
			return "", fmt.Errorf("cat requires a path argument")
		}
		return executeCat(path)
	case "ps":
		return executePs()
	case "uptime":
		return executeUptime()
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}
