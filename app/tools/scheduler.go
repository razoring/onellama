//go:build windows || darwin

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ollama/ollama/app/store"
)

type SchedulerTool struct {
	Store *store.Store
}

func NewSchedulerTool(st *store.Store) *SchedulerTool {
	return &SchedulerTool{Store: st}
}

func (s *SchedulerTool) Name() string {
	return "Scheduler"
}

func (s *SchedulerTool) Description() string {
	return "Schedule prompts to run automatically in the background at a specified time. Supported operations: schedule_create, schedule_list, schedule_update, schedule_delete."
}

func (s *SchedulerTool) Prompt() string {
	return "Use the Scheduler tool to create, view, update, or cancel scheduled tasks for running prompts in the background at specific times."
}

func (s *SchedulerTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type":        "string",
				"enum":        []string{"schedule_create", "schedule_list", "schedule_update", "schedule_delete"},
				"description": "The operation to perform on scheduled tasks.",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "The prompt text to be executed when the scheduled time arrives (required for schedule_create).",
			},
			"scheduled_at": map[string]any{
				"type":        "string",
				"description": "The date and time when the prompt should execute in ISO 8601 / RFC3339 format (e.g. '2026-09-23T09:00:00Z' or '2026-09-23T14:30:00-04:00') or YYYY-MM-DD HH:MM:SS format.",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "The model to use when executing the prompt (optional, defaults to current model if unspecified).",
			},
			"id": map[string]any{
				"type":        "string",
				"description": "The ID of the task to update or delete (required for schedule_update and schedule_delete).",
			},
		},
		"required": []string{"operation"},
	}
}

func (s *SchedulerTool) Execute(ctx context.Context, args map[string]any) (any, string, error) {
	if s.Store == nil {
		return nil, "", fmt.Errorf("store unavailable")
	}

	op, _ := args["operation"].(string)
	switch op {
	case "schedule_create":
		prompt, _ := args["prompt"].(string)
		if prompt == "" {
			return nil, "", fmt.Errorf("prompt is required for schedule_create")
		}
		schedStr, _ := args["scheduled_at"].(string)
		if schedStr == "" {
			return nil, "", fmt.Errorf("scheduled_at is required for schedule_create")
		}
		t, err := parseTime(schedStr)
		if err != nil {
			return nil, "", fmt.Errorf("invalid scheduled_at date format (%s): %w", schedStr, err)
		}
		modelName, _ := args["model"].(string)
		if modelName == "" {
			settings, _ := s.Store.Settings()
			modelName = settings.SelectedModel
		}

		task := store.ScheduledTask{
			Prompt:      prompt,
			Model:       modelName,
			ScheduledAt: t,
			Status:      "pending",
		}
		if err := s.Store.CreateScheduledTask(task); err != nil {
			return nil, "", fmt.Errorf("failed to create scheduled task: %w", err)
		}
		tasks, _ := s.Store.GetScheduledTasks()
		createdTask := task
		if len(tasks) > 0 {
			createdTask = tasks[len(tasks)-1]
		}
		resp, _ := json.Marshal(createdTask)
		return createdTask, fmt.Sprintf("Successfully scheduled task ID %s for %s: %s", createdTask.ID, t.Format(time.RFC3339), string(resp)), nil

	case "schedule_list":
		tasks, err := s.Store.GetScheduledTasks()
		if err != nil {
			return nil, "", fmt.Errorf("failed to list scheduled tasks: %w", err)
		}
		resp, _ := json.Marshal(tasks)
		return tasks, string(resp), nil

	case "schedule_update":
		id, _ := args["id"].(string)
		if id == "" {
			return nil, "", fmt.Errorf("id is required for schedule_update")
		}
		existing, err := s.Store.GetScheduledTask(id)
		if err != nil || existing == nil {
			return nil, "", fmt.Errorf("scheduled task not found: %s", id)
		}
		if prompt, ok := args["prompt"].(string); ok && prompt != "" {
			existing.Prompt = prompt
		}
		if schedStr, ok := args["scheduled_at"].(string); ok && schedStr != "" {
			t, err := parseTime(schedStr)
			if err != nil {
				return nil, "", fmt.Errorf("invalid scheduled_at date format: %w", err)
			}
			existing.ScheduledAt = t
		}
		if modelName, ok := args["model"].(string); ok && modelName != "" {
			existing.Model = modelName
		}
		if err := s.Store.UpdateScheduledTask(*existing); err != nil {
			return nil, "", fmt.Errorf("failed to update scheduled task: %w", err)
		}
		return existing, fmt.Sprintf("Successfully updated scheduled task ID %s", id), nil

	case "schedule_delete":
		id, _ := args["id"].(string)
		if id == "" {
			return nil, "", fmt.Errorf("id is required for schedule_delete")
		}
		if err := s.Store.DeleteScheduledTask(id); err != nil {
			return nil, "", fmt.Errorf("failed to delete scheduled task: %w", err)
		}
		return map[string]string{"id": id, "status": "deleted"}, fmt.Sprintf("Successfully deleted scheduled task ID %s", id), nil

	default:
		return nil, "", fmt.Errorf("unknown operation: %s", op)
	}
}

func parseTime(str string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02T15:04",
	}
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, str, time.Local); err == nil {
			return t, nil
		}
		if t, err := time.Parse(f, str); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse time string '%s'", str)
}
