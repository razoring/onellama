//go:build windows || darwin

package tools

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/ollama/ollama/app/store"
)

type SaveMemoryTool struct {
	Store *store.Store
}

func NewSaveMemoryTool(st *store.Store) *SaveMemoryTool {
	return &SaveMemoryTool{Store: st}
}

func (t *SaveMemoryTool) Name() string {
	return "save_memory"
}

func (t *SaveMemoryTool) Description() string {
	return "Stores a new piece of persistent memory. Use this to remember facts, preferences, or reusable workflows for future conversations."
}

func (t *SaveMemoryTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{
				"type":        "string",
				"description": "The exact content to remember. Make this detailed and descriptive.",
			},
			"title": map[string]any{
				"type":        "string",
				"description": "A short, optional title summarizing the memory.",
			},
		},
		"required": []string{"content"},
	}
}

func (t *SaveMemoryTool) Execute(ctx context.Context, args map[string]any) (any, string, error) {
	content, ok := args["content"].(string)
	if !ok || content == "" {
		return nil, "", fmt.Errorf("content is required")
	}

	title, _ := args["title"].(string)

	id := uuid.New().String()

	m := store.Memory{
		ID:      id,
		Title:   title,
		Content: content,
	}

	if err := t.Store.CreateMemory(m); err != nil {
		return nil, "", fmt.Errorf("failed to save memory: %w", err)
	}

	return m, fmt.Sprintf("Successfully saved memory with ID %s", id), nil
}

func (t *SaveMemoryTool) Prompt() string {
	return "When the user shares a significant personal fact, preference, or reusable workflow, automatically use the save_memory tool to store it. In your conversational text response, explicitly state what you just saved to memory and politely offer to remove it if they do not want it remembered."
}

// UpdateMemoryTool updates an existing memory
type UpdateMemoryTool struct {
	Store *store.Store
}

func NewUpdateMemoryTool(st *store.Store) *UpdateMemoryTool {
	return &UpdateMemoryTool{Store: st}
}

func (t *UpdateMemoryTool) Name() string {
	return "update_memory"
}

func (t *UpdateMemoryTool) Description() string {
	return "Updates an existing piece of persistent memory by its ID. Use this when the user modifies a previously stated preference or fact."
}

func (t *UpdateMemoryTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "string",
				"description": "The ID of the memory to update.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "The new, updated content.",
			},
			"title": map[string]any{
				"type":        "string",
				"description": "The new, updated title.",
			},
		},
		"required": []string{"id", "content"},
	}
}

func (t *UpdateMemoryTool) Execute(ctx context.Context, args map[string]any) (any, string, error) {
	id, ok := args["id"].(string)
	if !ok || id == "" {
		return nil, "", fmt.Errorf("id is required")
	}

	content, ok := args["content"].(string)
	if !ok || content == "" {
		return nil, "", fmt.Errorf("content is required")
	}

	title, _ := args["title"].(string)

	m := store.Memory{
		ID:      id,
		Title:   title,
		Content: content,
	}

	if err := t.Store.UpdateMemory(m); err != nil {
		return nil, "", fmt.Errorf("failed to update memory: %w", err)
	}

	return m, fmt.Sprintf("Successfully updated memory ID %s", id), nil
}

func (t *UpdateMemoryTool) Prompt() string {
	return "Use this tool to correct or update a memory if a previously stated preference changes."
}

// ForgetMemoryTool removes an existing memory
type ForgetMemoryTool struct {
	Store *store.Store
}

func NewForgetMemoryTool(st *store.Store) *ForgetMemoryTool {
	return &ForgetMemoryTool{Store: st}
}

func (t *ForgetMemoryTool) Name() string {
	return "forget_memory"
}

func (t *ForgetMemoryTool) Description() string {
	return "Deletes an existing piece of persistent memory by its ID. Use this when the user asks you to remove or forget something you previously saved."
}

func (t *ForgetMemoryTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "string",
				"description": "The ID of the memory to forget.",
			},
		},
		"required": []string{"id"},
	}
}

func (t *ForgetMemoryTool) Execute(ctx context.Context, args map[string]any) (any, string, error) {
	id, ok := args["id"].(string)
	if !ok || id == "" {
		return nil, "", fmt.Errorf("id is required")
	}

	if err := t.Store.DeleteMemory(id); err != nil {
		return nil, "", fmt.Errorf("failed to forget memory: %w", err)
	}

	return map[string]string{"id": id}, fmt.Sprintf("Successfully removed memory ID %s", id), nil
}

func (t *ForgetMemoryTool) Prompt() string {
	return "If the user explicitly asks you to remove or forget a memory, use this tool to delete it."
}
