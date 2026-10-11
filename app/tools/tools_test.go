//go:build windows || darwin

package tools

import (
	"context"
	"strings"
	"testing"
)

func TestRegistrySystemPrompt(t *testing.T) {
	r := NewRegistry()
	sched := NewSchedulerTool(nil)
	r.Register(sched)

	prompt := r.SystemPrompt()
	if !strings.Contains(prompt, "Current Local Time:") {
		t.Errorf("expected SystemPrompt to contain Current Local Time, got: %s", prompt)
	}
	if !strings.Contains(prompt, "Action-Oriented & Decisive") {
		t.Errorf("expected SystemPrompt to contain Action-Oriented guidelines, got: %s", prompt)
	}
	if !strings.Contains(prompt, "Autonomous Problem Solving & Intent Inference") {
		t.Errorf("expected SystemPrompt to contain problem solving guidelines, got: %s", prompt)
	}
	if !strings.Contains(prompt, "Handling Scheduled & Recurring Requests") {
		t.Errorf("expected SystemPrompt to contain scheduling instructions, got: %s", prompt)
	}
	if !strings.Contains(prompt, "Scheduler") {
		t.Errorf("expected SystemPrompt to contain Scheduler tool name, got: %s", prompt)
	}
}

type mockMCPTool struct {
	name        string
	serverName  string
	description string
}

func (m *mockMCPTool) Name() string                                 { return m.name }
func (m *mockMCPTool) Description() string                          { return m.description }
func (m *mockMCPTool) Schema() map[string]any                       { return nil }
func (m *mockMCPTool) Prompt() string                              { return m.description }
func (m *mockMCPTool) Execute(_ context.Context, _ map[string]any) (any, string, error) { return nil, "", nil }
func (m *mockMCPTool) MCPServerName() string                        { return m.serverName }

func TestRegistryMCPServersPrompt(t *testing.T) {
	r := NewRegistry()
	mock := &mockMCPTool{
		name:        "playwright_browser_navigate",
		serverName:  "playwright",
		description: "[playwright] Navigates the current page to the specified URL",
	}
	r.Register(mock)

	prompt := r.SystemPrompt()
	if !strings.Contains(prompt, "Model Context Protocol (MCP) Servers:") {
		t.Errorf("expected prompt to contain MCP header, got: %s", prompt)
	}
	if !strings.Contains(prompt, "playwright MCP Server") {
		t.Errorf("expected prompt to contain playwright MCP Server, got: %s", prompt)
	}
	if !strings.Contains(prompt, "playwright_browser_navigate") {
		t.Errorf("expected prompt to list tool playwright_browser_navigate, got: %s", prompt)
	}

	mcpOnly := r.MCPServersPrompt()
	if !strings.Contains(mcpOnly, "Active MCP Servers & Tools:") {
		t.Errorf("expected MCPServersPrompt to contain active tools, got: %s", mcpOnly)
	}
}
