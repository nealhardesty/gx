// Package tools provides LLM tool implementations for file system and process operations.
package tools

import (
	"encoding/json"
	"fmt"

	"cloud.google.com/go/vertexai/genai"
)

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

// GetToolDefinitions returns the Gemini tool definitions for all available tools.
func (r *Registry) GetToolDefinitions() []*genai.Tool {
	if !r.enabled {
		return nil
	}

	return []*genai.Tool{
		{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				{
					Name:        "pwd",
					Description: "Get the current working directory",
					Parameters:  &genai.Schema{Type: genai.TypeObject, Properties: map[string]*genai.Schema{}},
				},
				{
					Name:        "ls",
					Description: "List files and directories in a path",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"path": {
								Type:        genai.TypeString,
								Description: "The directory path to list (defaults to current directory)",
							},
							"recursive": {
								Type:        genai.TypeBoolean,
								Description: "If true, list recursively (like ls -R)",
							},
						},
					},
				},
				{
					Name:        "stat",
					Description: "Get detailed file or directory information",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"path": {
								Type:        genai.TypeString,
								Description: "The file or directory path to stat",
							},
						},
						Required: []string{"path"},
					},
				},
				{
					Name:        "cat",
					Description: "Read and return the contents of a file",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"path": {
								Type:        genai.TypeString,
								Description: "The file path to read",
							},
						},
						Required: []string{"path"},
					},
				},
				{
					Name:        "ps",
					Description: "List running processes with details",
					Parameters:  &genai.Schema{Type: genai.TypeObject, Properties: map[string]*genai.Schema{}},
				},
				{
					Name:        "uptime",
					Description: "Get system uptime information",
					Parameters:  &genai.Schema{Type: genai.TypeObject, Properties: map[string]*genai.Schema{}},
				},
			},
		},
	}
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

// ParseFunctionCall extracts the function name and arguments from a FunctionCall.
func ParseFunctionCall(fc *genai.FunctionCall) (string, map[string]any, error) {
	args := make(map[string]any)
	if fc.Args != nil {
		// Convert the args to a map
		data, err := json.Marshal(fc.Args)
		if err != nil {
			return "", nil, fmt.Errorf("failed to marshal args: %w", err)
		}
		if err := json.Unmarshal(data, &args); err != nil {
			return "", nil, fmt.Errorf("failed to unmarshal args: %w", err)
		}
	}
	return fc.Name, args, nil
}
