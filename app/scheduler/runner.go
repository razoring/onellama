//go:build windows || darwin

package scheduler

import (
	"context"
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

	ticker := time.NewTicker(5 * time.Second)
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
	tasks, err := r.store.GetPendingScheduledTasks(time.Now())
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
			r.executeTask(ctx, task)
		}
	}
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
	}

	task.Status = "running"
	task.UpdatedAt = time.Now()
	if err := r.store.UpdateScheduledTask(task); err != nil {
		slog.Error("failed to mark task running", "id", task.ID, "error", err)
		return
	}

	chat, err := r.store.Chat(chatID)
	if err != nil || chat == nil {
		title := task.Prompt
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		title = fmt.Sprintf("[Scheduled] %s", title)

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

	assistantMsg := store.NewMessage("assistant", "", &store.MessageOptions{Model: task.Model})
	if err := r.store.AppendMessage(chatID, assistantMsg); err != nil {
		slog.Error("failed to append assistant message for scheduled task", "chatID", chatID, "error", err)
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

	var msgs []api.Message
	if r.toolRegistry != nil {
		if sp := r.toolRegistry.SystemPrompt(); sp != "" {
			msgs = append(msgs, api.Message{
				Role:    "system",
				Content: sp,
			})
		}
	}
	msgs = append(msgs, api.Message{
		Role:    "user",
		Content: task.Prompt,
	})

	chatReq := &api.ChatRequest{
		Model:    task.Model,
		Messages: msgs,
	}
	if r.toolRegistry != nil {
		if tools := r.toolRegistry.OllamaTools(); len(tools) > 0 {
			chatReq.Tools = tools
		}
	}

	var contentBuf, thinkingBuf strings.Builder
	err = client.Chat(ctx, chatReq, func(res api.ChatResponse) error {
		if res.Message.Content != "" {
			contentBuf.WriteString(res.Message.Content)
		}
		if res.Message.Thinking != "" {
			thinkingBuf.WriteString(res.Message.Thinking)
		}
		assistantMsg.Content = contentBuf.String()
		assistantMsg.Thinking = thinkingBuf.String()
		assistantMsg.UpdatedAt = time.Now()
		_ = r.store.UpdateLastMessage(chatID, assistantMsg)
		return nil
	})

	task.UpdatedAt = time.Now()
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
