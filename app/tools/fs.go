//go:build windows || darwin

package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

type FSReadTool struct{}

func (t *FSReadTool) Name() string {
	return "fs_read"
}

func (t *FSReadTool) Description() string {
	return "Read the full contents of a file on the local file system."
}

func (t *FSReadTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute or relative path of the file to read",
			},
		},
		"required": []string{"path"},
	}
}

func (t *FSReadTool) Execute(ctx context.Context, args map[string]any) (any, string, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return nil, "", fmt.Errorf("path parameter is required")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read file '%s': %w", path, err)
	}

	strContent := string(content)
	return strContent, strContent, nil
}

func (t *FSReadTool) Prompt() string {
	return "Use this tool to read files from the file system."
}

type FSWriteTool struct{}

func (t *FSWriteTool) Name() string {
	return "fs_write"
}

func (t *FSWriteTool) Description() string {
	return "Create or overwrite a file on the local file system."
}

func (t *FSWriteTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "File path to write to",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "File content to write",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (t *FSWriteTool) Execute(ctx context.Context, args map[string]any) (any, string, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return nil, "", fmt.Errorf("path parameter is required")
	}
	content, _ := args["content"].(string)

	pm := GetPromptManager()
	promptID, _ := uuid.NewV7()
	req := &PromptRequest{
		ID:       promptID.String(),
		Type:     PromptTypePermission,
		ToolName: t.Name(),
		Title:    "File Write Request",
		Message:  fmt.Sprintf("The LLM is requesting to write to file:\n`%s`", path),
		Options:  []string{"Allow", "Allow for this chat", "Deny"},
	}

	resp, err := pm.CreatePrompt(req)
	if err != nil {
		return nil, "", err
	}

	switch strings.ToLower(strings.TrimSpace(resp)) {
	case "allow", "allow for this chat", "allow_chat":
		// Proceed
	case "deny":
		return "File write denied by user.", "File write denied by user.", nil
	default:
		return fmt.Sprintf("User denied file write: %s", resp), fmt.Sprintf("User denied file write: %s", resp), nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, "", fmt.Errorf("failed to create directory structure: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return nil, "", fmt.Errorf("failed to write file '%s': %w", path, err)
	}

	msg := fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path)
	return msg, msg, nil
}

func (t *FSWriteTool) Prompt() string {
	return "Use this tool to write content to files."
}
