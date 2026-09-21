//go:build windows || darwin

package tools

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

type AskUserTool struct{}

func (t *AskUserTool) Name() string {
	return "ask_question"
}

func (t *AskUserTool) Description() string {
	return "Ask the user a question with optional predefined choices when input or clarification is needed."
}

func (t *AskUserTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question": map[string]any{
				"type":        "string",
				"description": "The question to present to the user",
			},
			"options": map[string]any{
				"type":        "array",
				"description": "Optional list of choices for the user to select from",
				"items": map[string]any{
					"type": "string",
				},
			},
		},
		"required": []string{"question"},
	}
}

func (t *AskUserTool) Execute(ctx context.Context, args map[string]any) (any, string, error) {
	question, ok := args["question"].(string)
	if !ok || question == "" {
		return nil, "", fmt.Errorf("question parameter is required")
	}

	var options []string
	if rawOpts, ok := args["options"].([]any); ok {
		for _, opt := range rawOpts {
			if s, ok := opt.(string); ok {
				options = append(options, s)
			}
		}
	}

	promptID, _ := uuid.NewV7()
	id := promptID.String()

	req := &PromptRequest{
		ID:       id,
		Type:     PromptTypeQuestion,
		ToolName: t.Name(),
		Title:    "Question from AI",
		Message:  question,
		Options:  options,
	}

	pm := GetPromptManager()
	userResponse, err := pm.CreatePrompt(req)
	if err != nil {
		return nil, "", err
	}

	return userResponse, userResponse, nil
}

func (t *AskUserTool) Prompt() string {
	return "Use this tool to ask the user a question when clarification or input is needed."
}
