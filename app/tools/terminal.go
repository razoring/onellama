//go:build windows || darwin

package tools

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/google/uuid"
)

type TerminalTool struct{}

func (t *TerminalTool) Name() string {
	return "terminal_run"
}

func (t *TerminalTool) Description() string {
	return "Execute a shell command on the host machine and return stdout and stderr."
}

func (t *TerminalTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The command line string to execute",
			},
		},
		"required": []string{"command"},
	}
}

func (t *TerminalTool) checkPermission(cmdStr string) (bool, string, error) {
	pm := GetPromptManager()

	promptID, _ := uuid.NewV7()
	req := &PromptRequest{
		ID:       promptID.String(),
		Type:     PromptTypePermission,
		ToolName: t.Name(),
		Title:    "Terminal Command Execution",
		Message:  fmt.Sprintf("The LLM is requesting to run the following terminal command:\n\n`%s`", cmdStr),
		Options:  []string{"Allow", "Allow for this chat", "Deny"},
	}

	resp, err := pm.CreatePrompt(req)
	if err != nil {
		return false, "", err
	}

	switch strings.ToLower(strings.TrimSpace(resp)) {
	case "allow":
		return true, "", nil
	case "allow for this chat", "allow_chat":
		return true, "chat", nil
	case "deny":
		return false, "Command execution denied by user.", nil
	default:
		// Custom text entered in "Other"
		return false, fmt.Sprintf("User denied command with reason: %s", resp), nil
	}
}

func (t *TerminalTool) Execute(ctx context.Context, args map[string]any) (any, string, error) {
	cmdStr, ok := args["command"].(string)
	if !ok || cmdStr == "" {
		return nil, "", fmt.Errorf("command parameter is required")
	}

	// Check permission
	allowed, denyMsg, err := t.checkPermission(cmdStr)
	if err != nil {
		return nil, "", err
	}
	if !allowed {
		return denyMsg, denyMsg, nil
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", cmdStr)
	} else {
		cmd = exec.CommandContext(ctx, "bash", "-c", cmdStr)
	}

	outputBytes, err := cmd.CombinedOutput()
	outputStr := string(outputBytes)
	if err != nil {
		if outputStr == "" {
			outputStr = err.Error()
		} else {
			outputStr = fmt.Sprintf("%s\n(Exit Error: %v)", outputStr, err)
		}
	}

	return outputStr, outputStr, nil
}

func (t *TerminalTool) Prompt() string {
	return "Use this tool to execute terminal commands on the user's system."
}
