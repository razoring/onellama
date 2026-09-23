//go:build windows || darwin

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ollama/ollama/api"
)

// Tool defines the interface that all tools must implement
type Tool interface {
	// Name returns the unique identifier for the tool
	Name() string

	// Description returns a human-readable description of what the tool does
	Description() string

	// Schema returns the JSON schema for the tool's parameters
	Schema() map[string]any

	// Execute runs the tool with the given arguments and returns result to store in db, and a string result for the model
	Execute(ctx context.Context, args map[string]any) (any, string, error)

	// Prompt returns a prompt for the tool
	Prompt() string
}

// Registry manages the available tools and their execution
type Registry struct {
	tools      map[string]Tool
	workingDir string // Working directory for all tool operations
}

// NewRegistry creates a new tool registry with no tools
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry
func (r *Registry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

// Get retrieves a tool by name
func (r *Registry) Get(name string) (Tool, bool) {
	tool, exists := r.tools[name]
	return tool, exists
}

// List returns all available tools
func (r *Registry) List() []Tool {
	tools := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

// SetWorkingDir sets the working directory for all tool operations
func (r *Registry) SetWorkingDir(dir string) {
	r.workingDir = dir
}

// Execute runs a tool with the given name and arguments
func (r *Registry) Execute(ctx context.Context, name string, args map[string]any) (any, string, error) {
	tool, ok := r.tools[name]
	if !ok {
		return nil, "", fmt.Errorf("unknown tool: %s", name)
	}

	result, text, err := tool.Execute(ctx, args)
	if err != nil {
		return nil, "", err
	}
	return result, text, nil
}

// ToolCall represents a request to execute a tool
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction represents the function call details
type ToolFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    any    `json:"content"`
	Error      string `json:"error,omitempty"`
}

// ToolSchemas returns all tools as schema maps suitable for API calls
func (r *Registry) AvailableTools() []map[string]any {
	schemas := make([]map[string]any, 0, len(r.tools))
	for _, tool := range r.tools {
		schema := map[string]any{
			"name":        tool.Name(),
			"description": tool.Description(),
			"schema":      tool.Schema(),
		}
		schemas = append(schemas, schema)
	}
	return schemas
}

// ToolNames returns a list of all tool names
func (r *Registry) ToolNames() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// SystemPrompt returns the agent system prompt guiding proactive tool usage and providing current time context.
func (r *Registry) SystemPrompt() string {
	var sb strings.Builder
	now := time.Now()

	sb.WriteString("You are a proactive, highly autonomous AI assistant with direct access to local and external tools.\n\n")

	sb.WriteString(fmt.Sprintf("Current Local Time: %s (%s, UTC offset: %s)\n\n",
		now.Format("2006-01-02 15:04:05"),
		now.Weekday().String(),
		now.Format("-07:00"),
	))

	sb.WriteString("# Core Operating Guidelines:\n")
	sb.WriteString("1. **Action-Oriented & Decisive**: You are an agent, not merely a conversational model. When a user asks you to perform an action, find information, browse the web, or schedule a task, do NOT refuse, hesitate, or claim you lack real-time, browsing, or scheduling abilities. Formulate a plan and immediately execute the appropriate tool call(s).\n")
	sb.WriteString("2. **No Unnecessary Refusals**: Never say 'I cannot browse', 'I cannot interact with external services', or 'I cannot run recurring tasks' when relevant tools (such as browser automation, Playwright, web search, or Scheduler) are present in your toolset. If a tool exists for a domain, use it immediately.\n")
	sb.WriteString("3. **Handling Scheduled & Recurring Requests**:\n")
	sb.WriteString("   - When the user asks to perform an action at a future time or on a recurring basis (e.g., 'every day at 12:24am', 'in 30 minutes', 'tomorrow morning'):\n")
	sb.WriteString("     a. Calculate the exact target timestamp using the Current Local Time above.\n")
	sb.WriteString("     b. Immediately invoke the `Scheduler` tool with operation `schedule_create`.\n")
	sb.WriteString("     c. For the scheduled `prompt`, write clear, detailed instructions for the agent to execute when triggered (including any browser navigation, search queries, or data extraction requested by the user, and an explicit instruction to re-schedule itself for subsequent runs if recurring).\n")
	sb.WriteString("     d. Report to the user that the task has been scheduled along with the scheduled execution time.\n")
	sb.WriteString("4. **Multi-Step Workflows**: For complex or multi-step requests, break down the goal and execute the first step immediately via tool call. Do not ask for user confirmation for routine steps unless clarification is strictly necessary.\n")
	sb.WriteString("5. **Tool Precision**: Supply accurate, well-formed arguments matching each tool's schema.\n\n")

	if r != nil && len(r.tools) > 0 {
		toolPrompts := make([]string, 0)
		for _, tool := range r.tools {
			if p := strings.TrimSpace(tool.Prompt()); p != "" {
				toolPrompts = append(toolPrompts, fmt.Sprintf("- `%s`: %s", tool.Name(), p))
			}
		}

		if len(toolPrompts) > 0 {
			sb.WriteString("# Available Tools & Capabilities:\n")
			for _, p := range toolPrompts {
				sb.WriteString(p + "\n")
			}
		}
	}

	return sb.String()
}

// ConvertToOllamaTool converts a tool schema from our tools package format to Ollama API format
func ConvertToOllamaTool(toolSchema map[string]any) api.Tool {
	tool := api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        getStringFromMap(toolSchema, "name", ""),
			Description: getStringFromMap(toolSchema, "description", ""),
		},
	}

	tool.Function.Parameters.Type = "object"
	tool.Function.Parameters.Required = []string{}
	tool.Function.Parameters.Properties = api.NewToolPropertiesMap()

	if schemaProps, ok := toolSchema["schema"].(map[string]any); ok {
		tool.Function.Parameters.Type = getStringFromMap(schemaProps, "type", "object")

		if props, ok := schemaProps["properties"].(map[string]any); ok {
			tool.Function.Parameters.Properties = api.NewToolPropertiesMap()

			for propName, propDef := range props {
				if propMap, ok := propDef.(map[string]any); ok {
					prop := api.ToolProperty{
						Type:        api.PropertyType{getStringFromMap(propMap, "type", "string")},
						Description: getStringFromMap(propMap, "description", ""),
					}
					tool.Function.Parameters.Properties.Set(propName, prop)
				}
			}
		}

		if required, ok := schemaProps["required"].([]string); ok {
			tool.Function.Parameters.Required = required
		} else if requiredAny, ok := schemaProps["required"].([]any); ok {
			required := make([]string, len(requiredAny))
			for i, r := range requiredAny {
				if s, ok := r.(string); ok {
					required[i] = s
				}
			}
			tool.Function.Parameters.Required = required
		}
	}

	return tool
}

func getStringFromMap(m map[string]any, key, defaultValue string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return defaultValue
}

// OllamaTools returns all tools converted to api.Tools slice
func (r *Registry) OllamaTools() api.Tools {
	if r == nil {
		return nil
	}
	schemas := r.AvailableTools()
	tools := make(api.Tools, len(schemas))
	for i, s := range schemas {
		tools[i] = ConvertToOllamaTool(s)
	}
	return tools
}
