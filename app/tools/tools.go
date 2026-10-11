//go:build windows || darwin

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
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

// MCPAware is an optional interface implemented by tools provided by an MCP server.
type MCPAware interface {
	MCPServerName() string
}

// Registry manages the available tools and their execution
type Registry struct {
	mu         sync.RWMutex
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
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
}

// Unregister removes a tool from the registry
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
}

// Get retrieves a tool by name
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, exists := r.tools[name]
	return tool, exists
}

// List returns all available tools
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

// SetWorkingDir sets the working directory for all tool operations
func (r *Registry) SetWorkingDir(dir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workingDir = dir
}

// Execute runs a tool with the given name and arguments
func (r *Registry) Execute(ctx context.Context, name string, args map[string]any) (any, string, error) {
	r.mu.RLock()
	tool, ok := r.tools[name]
	r.mu.RUnlock()
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
	r.mu.RLock()
	defer r.mu.RUnlock()
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
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// MCPServers returns a map of MCP server names to their tools registered in this Registry.
func (r *Registry) MCPServers() map[string][]Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mcpServersLocked()
}

func (r *Registry) mcpServersLocked() map[string][]Tool {
	servers := make(map[string][]Tool)
	if r == nil {
		return servers
	}
	for _, tool := range r.tools {
		var sName string
		if mcp, ok := tool.(MCPAware); ok {
			sName = mcp.MCPServerName()
		}
		if sName == "" {
			desc := tool.Description()
			if strings.HasPrefix(desc, "[") && strings.Contains(desc, "]") {
				idx := strings.Index(desc, "]")
				sName = strings.TrimSpace(desc[1:idx])
			}
		}
		if sName != "" {
			servers[sName] = append(servers[sName], tool)
		}
	}
	return servers
}

// MCPServersPrompt returns the formatted instructions and tools list for active MCP servers.
func (r *Registry) MCPServersPrompt() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mcpServersPromptLocked()
}

func (r *Registry) mcpServersPromptLocked() string {
	servers := r.mcpServersLocked()
	if len(servers) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# Model Context Protocol (MCP) Servers:\n")
	sb.WriteString("You have access to tools provided by external MCP servers. Each MCP tool is registered with the prefix `<server_name>_<tool_name>`.\n")
	sb.WriteString("CRITICAL: When the user asks to use an MCP server by name (such as 'playwright MCP server'), recognize that there is NO individual tool called '<server_name> MCP server'. The tools prefixed with `<server_name>_*` (such as `playwright_browser_navigate`, `playwright_browser_click`, etc.) ARE the tools of that MCP server. You MUST use those tools immediately.\n\n")
	sb.WriteString("Active MCP Servers & Tools:\n")

	var serverNames []string
	for name := range servers {
		serverNames = append(serverNames, name)
	}
	sort.Strings(serverNames)

	for _, name := range serverNames {
		sb.WriteString(fmt.Sprintf("### %s MCP Server:\n", name))
		tools := servers[name]
		sort.Slice(tools, func(i, j int) bool { return tools[i].Name() < tools[j].Name() })
		for _, t := range tools {
			desc := strings.TrimSpace(t.Description())
			sb.WriteString(fmt.Sprintf("- `%s`: %s\n", t.Name(), desc))
		}
		sb.WriteString("\n")
	}

	return strings.TrimSpace(sb.String())
}

// SystemPrompt returns the agent system prompt guiding proactive tool usage and providing current time context.
func (r *Registry) SystemPrompt() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
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
	sb.WriteString("2. **Autonomous Problem Solving & Intent Inference**:\n")
	sb.WriteString("   - Infer the user's underlying intent without requiring explicit step-by-step guidance, exact URLs, or explicit tool names.\n")
	sb.WriteString("   - When asked to search, look up information (e.g., weather, news, documentation, public data), or browse using browser automation tools (such as Playwright MCP server) without an explicit URL provided, do NOT refuse or claim you lack a URL or specific target. Autonomously construct and navigate to an appropriate search engine or target URL (e.g., `https://www.google.com/search?q=<query>`, `https://duckduckgo.com/?q=<query>`, or `https://wttr.in/<location>`) and proceed with the task immediately.\n")
	sb.WriteString("   - When using Playwright browser MCP tools:\n")
	sb.WriteString("     a. **Navigate Directly to Query URLs**: Navigate directly to full search or query URLs (e.g., `playwright_browser_navigate` with `url: \"https://www.google.com/search?q=weather+in+cupertino\"` or `url: \"https://wttr.in/cupertino\"`). Do NOT navigate to a bare homepage (e.g. `google.com`) and try to guess how to type into and submit form fields when a direct search URL exists.\n")
	sb.WriteString("     b. **Selectors**: In `playwright_browser_type` and `playwright_browser_click`, `target` MUST be a real CSS selector (e.g., `textarea[name=\"q\"]`, `button[type=\"submit\"]`) or valid element reference. NEVER invent conversational English descriptions (such as 'Search Input Field' or 'viewport') as a target selector.\n")
	sb.WriteString("     c. **Snapshots**: To inspect the current page, call `playwright_browser_snapshot` with `{}` (no `target` required).\n")
	sb.WriteString("   - If any tool execution fails or an external service is unavailable (e.g., cloud web search disabled or rate-limited), immediately and autonomously pivot to alternative tools (such as browser navigation via MCP servers like `playwright_browser_navigate`) to achieve the user's objective without stalling, apologizing, or reporting failure.\n")
	sb.WriteString("   - Never assume you need a separate web search tool before you can browse. You can directly browse search engines (Google, DuckDuckGo) or specific sites using browser tools.\n")
	sb.WriteString("   - Chain multiple tool calls proactively: navigate -> inspect/snapshot -> click/extract -> summarize.\n")
	sb.WriteString("3. **No Unnecessary Refusals**: Never say 'I cannot browse', 'I cannot interact with external services', or 'I cannot run recurring tasks' when relevant tools (such as browser MCP servers or Scheduler) are present in your toolset. If a tool exists for a domain, use it immediately.\n")
	sb.WriteString("4. **Handling Scheduled & Recurring Requests**:\n")
	sb.WriteString("   - When the user asks to perform an action at a future time or on a recurring basis (e.g., 'every day at 12:24am', 'in 30 minutes', 'tomorrow morning'):\n")
	sb.WriteString("     a. Calculate the exact target timestamp using the Current Local Time above.\n")
	sb.WriteString("     b. Immediately invoke the `Scheduler` tool with operation `schedule_create`.\n")
	sb.WriteString("     c. For the scheduled `prompt`, write clear, detailed instructions for the agent to execute when triggered (including any browser navigation, search queries, or data extraction requested by the user, and an explicit instruction to re-schedule itself for subsequent runs if recurring).\n")
	sb.WriteString("     d. Report to the user that the task has been scheduled along with the scheduled execution time.\n")
	sb.WriteString("5. **Multi-Step Workflows**: For complex or multi-step requests, break down the goal and execute the first step immediately via tool call. Do not ask for user confirmation for routine steps unless clarification is strictly necessary.\n")
	sb.WriteString("6. **Tool Precision**: Supply accurate, well-formed arguments matching each tool's schema.\n\n")

	mcpSection := r.mcpServersPromptLocked()
	if mcpSection != "" {
		sb.WriteString(mcpSection)
		sb.WriteString("\n\n")
	}

	if r != nil && len(r.tools) > 0 {
		mcpMap := r.mcpServersLocked()
		var builtinTools []Tool
		for _, tool := range r.tools {
			isMCP := false
			for _, sTools := range mcpMap {
				for _, st := range sTools {
					if st.Name() == tool.Name() {
						isMCP = true
						break
					}
				}
				if isMCP {
					break
				}
			}
			if !isMCP {
				builtinTools = append(builtinTools, tool)
			}
		}

		if len(builtinTools) > 0 {
			sb.WriteString("# Built-in Tools & Capabilities:\n")
			sort.Slice(builtinTools, func(i, j int) bool { return builtinTools[i].Name() < builtinTools[j].Name() })
			for _, tool := range builtinTools {
				desc := strings.TrimSpace(tool.Prompt())
				if desc == "" {
					desc = strings.TrimSpace(tool.Description())
				}
				if desc != "" {
					sb.WriteString(fmt.Sprintf("- `%s`: %s\n", tool.Name(), desc))
				}
			}
			sb.WriteString("\n")
		}
	}

	return strings.TrimSpace(sb.String())
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
