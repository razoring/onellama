//go:build windows || darwin

package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ollama/ollama/api"
	"github.com/ollama/ollama/app/store"
	"github.com/ollama/ollama/app/tools"
)

type Runner struct {
	store        *store.Store
	toolRegistry *tools.Registry
	stop         chan struct{}
	wg           sync.WaitGroup
}

func NewRunner(s *store.Store, tr *tools.Registry) *Runner {
	return &Runner{
		store:        s,
		toolRegistry: tr,
		stop:         make(chan struct{}),
	}
}

func (r *Runner) Start(ctx context.Context) {
	r.wg.Add(1)
	go r.loop(ctx)
}

func (r *Runner) Stop() {
	close(r.stop)
	r.wg.Wait()
}

func (r *Runner) loop(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Initial check on startup
	r.processPendingTasks(ctx)

	for {
		select {
		case <-r.stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.processPendingTasks(ctx)
		}
	}
}

func (r *Runner) processPendingTasks(ctx context.Context) {
	tasks, err := r.store.GetPendingScheduledTasks(time.Now().UTC())
	if err != nil {
		slog.Error("failed to get pending scheduled tasks", "error", err)
		return
	}

	for _, task := range tasks {
		select {
		case <-r.stop:
			return
		case <-ctx.Done():
			return
		default:
			if task.ChatID == "" {
				u, err := uuid.NewV7()
				if err != nil {
					task.ChatID = fmt.Sprintf("scheduled-%d", time.Now().UnixNano())
				} else {
					task.ChatID = u.String()
				}
			}
			// Mark running immediately with ChatID populated so UI knows this chat is running
			task.Status = "running"
			task.UpdatedAt = time.Now().UTC()
			if err := r.store.UpdateScheduledTask(task); err != nil {
				slog.Error("failed to mark task running", "id", task.ID, "error", err)
				continue
			}

			r.wg.Add(1)
			go func(t store.ScheduledTask) {
				defer r.wg.Done()
				r.executeTask(ctx, t)
			}(task)
		}
	}
}

func buildApiMessages(chat *store.Chat, systemPrompt string) []api.Message {
	var msgs []api.Message
	if systemPrompt != "" {
		msgs = append(msgs, api.Message{
			Role:    "system",
			Content: systemPrompt,
		})
	}
	for _, m := range chat.Messages {
		if m.Content == "" && m.Thinking == "" && len(m.ToolCalls) == 0 && len(m.Attachments) == 0 {
			continue
		}
		apiMsg := api.Message{Role: m.Role, Thinking: m.Thinking, Content: m.Content}

		switch m.Role {
		case "assistant":
			if len(m.ToolCalls) > 0 {
				var toolCalls []api.ToolCall
				for _, tc := range m.ToolCalls {
					var args api.ToolCallFunctionArguments
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err == nil {
						toolCalls = append(toolCalls, api.ToolCall{
							Function: api.ToolCallFunction{
								Name:      tc.Function.Name,
								Arguments: args,
							},
						})
					}
				}
				apiMsg.ToolCalls = toolCalls
			}
		case "tool":
			apiMsg.Role = "tool"
			apiMsg.Content = m.Content
			apiMsg.ToolName = m.ToolName
		}

		msgs = append(msgs, apiMsg)
	}
	return msgs
}

func (r *Runner) executeTask(ctx context.Context, task store.ScheduledTask) {
	slog.Info("executing scheduled task", "id", task.ID, "prompt", task.Prompt, "model", task.Model)

	chatID := task.ChatID
	if chatID == "" {
		u, err := uuid.NewV7()
		if err != nil {
			chatID = fmt.Sprintf("scheduled-%d", time.Now().UnixNano())
		} else {
			chatID = u.String()
		}
		task.ChatID = chatID
		_ = r.store.UpdateScheduledTask(task)
	}

	chat, err := r.store.Chat(chatID)
	if err != nil || chat == nil {
		title := task.Prompt
		if len(title) > 40 {
			title = title[:37] + "..."
		}

		newChat := store.Chat{
			ID:        chatID,
			Title:     title,
			CreatedAt: time.Now(),
			Messages:  []store.Message{},
		}
		if err := r.store.SetChat(newChat); err != nil {
			slog.Error("failed to create chat for scheduled task", "chatID", chatID, "error", err)
			task.Status = "failed"
			task.LastError = fmt.Sprintf("failed to create chat: %v", err)
			task.UpdatedAt = time.Now()
			_ = r.store.UpdateScheduledTask(task)
			return
		}
		chat = &newChat
	}

	if len(chat.Messages) == 0 {
		userMsg := store.NewMessage("user", task.Prompt, &store.MessageOptions{Model: task.Model})
		if err := r.store.AppendMessage(chatID, userMsg); err != nil {
			slog.Error("failed to append user message for scheduled task", "chatID", chatID, "error", err)
		}
	}

	client, err := api.ClientFromEnvironment()
	if err != nil {
		slog.Error("failed to get ollama client for scheduled task", "error", err)
		task.Status = "failed"
		task.LastError = fmt.Sprintf("failed to get client: %v", err)
		task.UpdatedAt = time.Now()
		_ = r.store.UpdateScheduledTask(task)
		return
	}

	maxPasses := 10
	pass := 0

	for pass < maxPasses {
		pass++
		latestChat, err := r.store.Chat(chatID)
		if err != nil || latestChat == nil {
			slog.Error("failed to fetch chat from store", "chatID", chatID, "error", err)
			break
		}

		assistantMsg := store.NewMessage("assistant", "", &store.MessageOptions{Model: task.Model})
		if err := r.store.AppendMessage(chatID, assistantMsg); err != nil {
			slog.Error("failed to append assistant message for scheduled task", "chatID", chatID, "error", err)
			break
		}

		systemPrompt := ""
		var ollamaTools []api.Tool
		if r.toolRegistry != nil {
			systemPrompt = r.toolRegistry.SystemPrompt()
			ollamaTools = r.toolRegistry.OllamaTools()
		}

		apiMsgs := buildApiMessages(latestChat, systemPrompt)
		chatReq := &api.ChatRequest{
			Model:    task.Model,
			Messages: apiMsgs,
			Tools:    ollamaTools,
		}

		var contentBuf, thinkingBuf strings.Builder
		var lastToolCalls []api.ToolCall

		err = client.Chat(ctx, chatReq, func(res api.ChatResponse) error {
			if res.Message.Content != "" {
				contentBuf.WriteString(res.Message.Content)
			}
			if res.Message.Thinking != "" {
				thinkingBuf.WriteString(res.Message.Thinking)
			}
			if len(res.Message.ToolCalls) > 0 {
				lastToolCalls = res.Message.ToolCalls
			}
			assistantMsg.Content = contentBuf.String()
			assistantMsg.Thinking = thinkingBuf.String()
			assistantMsg.UpdatedAt = time.Now()
			_ = r.store.UpdateLastMessage(chatID, assistantMsg)
			return nil
		})

		if err != nil {
			slog.Error("scheduled task streaming failed", "id", task.ID, "error", err)
			break
		}

		if len(lastToolCalls) > 0 {
			toolCalls := make([]store.ToolCall, len(lastToolCalls))
			for i, tc := range lastToolCalls {
				argsJSON, _ := json.Marshal(tc.Function.Arguments)
				toolCalls[i] = store.ToolCall{
					Type: "function",
					Function: store.ToolFunction{
						Name:      tc.Function.Name,
						Arguments: string(argsJSON),
					},
				}
			}
			assistantMsg.ToolCalls = toolCalls
			assistantMsg.UpdatedAt = time.Now()
			_ = r.store.UpdateLastMessage(chatID, assistantMsg)

			for _, tc := range lastToolCalls {
				var result any
				var content string
				var execErr error

				if r.toolRegistry != nil {
					result, content, execErr = r.toolRegistry.Execute(ctx, tc.Function.Name, tc.Function.Arguments.ToMap())
				} else {
					execErr = fmt.Errorf("no tool registry available")
				}

				if execErr != nil {
					errContent := fmt.Sprintf("Error: %v", execErr)
					toolErrMsg := store.NewMessage("tool", errContent, nil)
					toolErrMsg.ToolName = tc.Function.Name
					_ = r.store.AppendMessage(chatID, toolErrMsg)
				} else {
					var tr json.RawMessage
					tr, _ = json.Marshal(result)
					modelContent := content
					if modelContent == "" && len(tr) > 0 {
						modelContent = string(tr)
					}
					toolMsg := store.NewMessage("tool", modelContent, &store.MessageOptions{
						ToolResult: &tr,
					})
					toolMsg.ToolName = tc.Function.Name
					_ = r.store.AppendMessage(chatID, toolMsg)
				}
			}
			continue
		}

		break
	}

	task.UpdatedAt = time.Now().UTC()
	if err != nil {
		slog.Error("scheduled task execution failed", "id", task.ID, "error", err)
		task.Status = "failed"
		task.LastError = err.Error()
	} else {
		slog.Info("scheduled task executed successfully", "id", task.ID)
		task.Status = "completed"
		task.LastError = ""
	}
	_ = r.store.UpdateScheduledTask(task)
}
