//go:build windows || darwin

package tools

import (
	"errors"
	"sync"
	"time"
)

type PromptType string

const (
	PromptTypePermission PromptType = "permission"
	PromptTypeQuestion   PromptType = "question"
)

type PromptRequest struct {
	ID       string      `json:"id"`
	Type     PromptType  `json:"type"`
	ChatID   string      `json:"chat_id"`
	ToolName string      `json:"tool_name"`
	Title    string      `json:"title"`
	Message  string      `json:"message"`
	Options  []string    `json:"options"`
	Channel  chan string `json:"-"`
}

type PromptManager struct {
	mu          sync.RWMutex
	pending     map[string]*PromptRequest
	chatAllowed map[string]map[string]bool // chat_id -> tool_name -> true
}

var globalPromptManager = &PromptManager{
	pending:     make(map[string]*PromptRequest),
	chatAllowed: make(map[string]map[string]bool),
}

func GetPromptManager() *PromptManager {
	return globalPromptManager
}

func (pm *PromptManager) IsChatAllowed(chatID, toolName string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if tools, ok := pm.chatAllowed[chatID]; ok {
		return tools[toolName]
	}
	return false
}

func (pm *PromptManager) AllowForChat(chatID, toolName string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if _, ok := pm.chatAllowed[chatID]; !ok {
		pm.chatAllowed[chatID] = make(map[string]bool)
	}
	pm.chatAllowed[chatID][toolName] = true
}

func (pm *PromptManager) CreatePrompt(req *PromptRequest) (string, error) {
	req.Channel = make(chan string, 1)

	pm.mu.Lock()
	pm.pending[req.ID] = req
	pm.mu.Unlock()

	// Wait for response or timeout after 5 minutes
	select {
	case resp := <-req.Channel:
		pm.mu.Lock()
		delete(pm.pending, req.ID)
		pm.mu.Unlock()
		return resp, nil
	case <-time.After(5 * time.Minute):
		pm.mu.Lock()
		delete(pm.pending, req.ID)
		pm.mu.Unlock()
		return "", errors.New("prompt timed out waiting for user response")
	}
}

func (pm *PromptManager) Respond(id string, response string) bool {
	pm.mu.RLock()
	req, ok := pm.pending[id]
	pm.mu.RUnlock()

	if !ok {
		return false
	}

	select {
	case req.Channel <- response:
		return true
	default:
		return false
	}
}

func (pm *PromptManager) GetPending() *PromptRequest {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	for _, req := range pm.pending {
		return req
	}
	return nil
}
