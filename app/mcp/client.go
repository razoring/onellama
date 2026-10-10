//go:build windows || darwin

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ollama/ollama/app/tools"
)

type ServerConfig struct {
	Command  string   `json:"command"`
	Args     []string `json:"args"`
	Disabled bool     `json:"disabled"`
	Env      map[string]string `json:"env"`
}

type ConfigFile struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

type RPCMessage struct {
	Jsonrpc string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *RPCError        `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type Client struct {
	name   string
	cmd    *exec.Cmd
	in     io.WriteCloser
	out    io.ReadCloser
	err    io.ReadCloser
	nextID int64
	mu     sync.Mutex
	calls  map[string]chan *RPCMessage
	ctx    context.Context
	cancel context.CancelFunc
}

func NewClient(name string, cfg ServerConfig) (*Client, error) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	
	// Add environment variables
	cmd.Env = os.Environ()
	for k, v := range cfg.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	in, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}

	c := &Client{
		name:   name,
		cmd:    cmd,
		in:     in,
		out:    out,
		err:    stderr,
		calls:  make(map[string]chan *RPCMessage),
		ctx:    ctx,
		cancel: cancel,
	}

	go c.readLoop()
	go c.readStderrLoop()

	return c, nil
}

func (c *Client) readLoop() {
	scanner := bufio.NewScanner(c.out)
	for scanner.Scan() {
		line := scanner.Bytes()
		var msg RPCMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			slog.Error("mcp parse error", "server", c.name, "error", err, "line", string(line))
			continue
		}
		
		if msg.ID != nil {
			idStr := string(*msg.ID)
			c.mu.Lock()
			ch, ok := c.calls[idStr]
			if ok {
				delete(c.calls, idStr)
			}
			c.mu.Unlock()

			if ok {
				ch <- &msg
			}
		} else {
			// Handle notifications
			slog.Debug("mcp notification", "server", c.name, "method", msg.Method)
		}
	}
	c.cancel()
}

func (c *Client) readStderrLoop() {
	scanner := bufio.NewScanner(c.err)
	for scanner.Scan() {
		slog.Debug("mcp stderr", "server", c.name, "msg", scanner.Text())
	}
}

func (c *Client) Call(method string, params any) (*RPCMessage, error) {
	idNum := atomic.AddInt64(&c.nextID, 1)
	idRaw := json.RawMessage(fmt.Sprintf("%d", idNum))
	idStr := string(idRaw)

	var paramsRaw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		paramsRaw = b
	}

	msg := RPCMessage{
		Jsonrpc: "2.0",
		ID:      &idRaw,
		Method:  method,
		Params:  paramsRaw,
	}

	b, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	ch := make(chan *RPCMessage, 1)
	c.mu.Lock()
	c.calls[idStr] = ch
	c.mu.Unlock()

	b = append(b, '\n')
	if _, err := c.in.Write(b); err != nil {
		c.mu.Lock()
		delete(c.calls, idStr)
		c.mu.Unlock()
		return nil, err
	}

	select {
	case <-c.ctx.Done():
		return nil, c.ctx.Err()
	case res := <-ch:
		if res.Error != nil {
			return nil, fmt.Errorf("rpc error %d: %s", res.Error.Code, res.Error.Message)
		}
		return res, nil
	}
}

func (c *Client) Close() {
	c.cancel()
	c.cmd.Process.Kill()
	c.cmd.Wait()
}

// MCP Tool Wrapper

type MCPTool struct {
	client      *Client
	name        string
	description string
	inputSchema map[string]any
}

func (t *MCPTool) Name() string {
	return t.name
}

func (t *MCPTool) Description() string {
	return t.description
}

func (t *MCPTool) Schema() map[string]any {
	return t.inputSchema
}

func (t *MCPTool) Execute(ctx context.Context, args map[string]any) (any, string, error) {
	params := map[string]any{
		"name":      t.name,
		"arguments": args,
	}
	
	res, err := t.client.Call("tools/call", params)
	if err != nil {
		return nil, "", err
	}

	var callRes struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}

	if err := json.Unmarshal(res.Result, &callRes); err != nil {
		return nil, "", err
	}

	var out string
	for _, c := range callRes.Content {
		if c.Type == "text" {
			out += c.Text + "\n"
		}
	}

	if callRes.IsError {
		return nil, "", fmt.Errorf("mcp tool error: %s", out)
	}

	return out, out, nil
}

func (t *MCPTool) Prompt() string {
	if t.description != "" {
		return t.description
	}
	return fmt.Sprintf("Use this tool to interact with the %s MCP server.", t.client.name)
}

type MCPToolItem struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Disabled    bool           `json:"disabled"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

type ToolStateFile struct {
	DisabledTools map[string][]string `json:"disabledTools"` // serverName -> list of disabled tool names
}

func getToolStatePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".ollama", "mcp_tools_state.json"), nil
}

func loadToolState() (*ToolStateFile, error) {
	path, err := getToolStatePath()
	if err != nil {
		return &ToolStateFile{DisabledTools: make(map[string][]string)}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return &ToolStateFile{DisabledTools: make(map[string][]string)}, nil
	}
	var state ToolStateFile
	if err := json.Unmarshal(b, &state); err != nil {
		return &ToolStateFile{DisabledTools: make(map[string][]string)}, nil
	}
	if state.DisabledTools == nil {
		state.DisabledTools = make(map[string][]string)
	}
	return &state, nil
}

func saveToolState(state *ToolStateFile) error {
	path, err := getToolStatePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

func isToolDisabledInState(state *ToolStateFile, serverName, toolName string) bool {
	if state == nil || state.DisabledTools == nil {
		return false
	}
	list := state.DisabledTools[serverName]
	for _, t := range list {
		if strings.EqualFold(t, toolName) {
			return true
		}
	}
	return false
}

type ManagedServer struct {
	Name   string
	Config ServerConfig
	Client *Client
	Tools  []MCPToolItem
	Status string
	Error  string
}

type Manager struct {
	mu       sync.RWMutex
	registry *tools.Registry
	servers  map[string]*ManagedServer
}

var (
	defaultManagerLock sync.Mutex
	defaultManager     *Manager
)

func GetManager() *Manager {
	defaultManagerLock.Lock()
	defer defaultManagerLock.Unlock()
	if defaultManager == nil {
		defaultManager = &Manager{
			servers: make(map[string]*ManagedServer),
		}
	}
	return defaultManager
}

func (m *Manager) SetRegistry(reg *tools.Registry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registry = reg
}

func serverConfigEqual(a, b ServerConfig) bool {
	if a.Command != b.Command || a.Disabled != b.Disabled || len(a.Args) != len(b.Args) || len(a.Env) != len(b.Env) {
		return false
	}
	for i := range a.Args {
		if a.Args[i] != b.Args[i] {
			return false
		}
	}
	for k, v := range a.Env {
		if b.Env[k] != v {
			return false
		}
	}
	return true
}

func (m *Manager) Sync() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cfgPath := filepath.Join(homeDir, ".ollama", "mcp_config.json")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		claudePath := filepath.Join(homeDir, ".ollama", "claude_desktop_config.json")
		if _, err := os.Stat(claudePath); err == nil {
			cfgPath = claudePath
		}
	}

	b, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var cfg ConfigFile
	if err := json.Unmarshal(b, &cfg); err != nil {
		return err
	}

	state, _ := loadToolState()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Clean up servers removed from config
	for sName, srv := range m.servers {
		if _, exists := cfg.MCPServers[sName]; !exists {
			if m.registry != nil {
				for _, t := range srv.Tools {
					m.registry.Unregister(fmt.Sprintf("%s_%s", sName, t.Name))
				}
			}
			if srv.Client != nil {
				srv.Client.Close()
			}
			delete(m.servers, sName)
		}
	}

	for name, srvCfg := range cfg.MCPServers {
		existing, hasExisting := m.servers[name]

		if srvCfg.Disabled {
			if hasExisting {
				if m.registry != nil {
					for _, t := range existing.Tools {
						m.registry.Unregister(fmt.Sprintf("%s_%s", name, t.Name))
					}
				}
				if existing.Client != nil {
					existing.Client.Close()
					existing.Client = nil
				}
				existing.Status = "disabled"
			}
			continue
		}

		// If existing client is already running and config didn't change command/args/env
		if hasExisting && existing.Client != nil && serverConfigEqual(existing.Config, srvCfg) {
			for i := range existing.Tools {
				toolName := existing.Tools[i].Name
				isDisabled := isToolDisabledInState(state, name, toolName)
				existing.Tools[i].Disabled = isDisabled
				prefixedName := fmt.Sprintf("%s_%s", name, toolName)
				if m.registry != nil {
					if isDisabled {
						m.registry.Unregister(prefixedName)
					} else {
						mcpTool := &MCPTool{
							client:      existing.Client,
							name:        toolName,
							description: fmt.Sprintf("[%s] %s", name, existing.Tools[i].Description),
							inputSchema: existing.Tools[i].InputSchema,
						}
						m.registry.Register(&RegistryMCPTool{MCPTool: mcpTool, registeredName: prefixedName})
					}
				}
			}
			continue
		}

		// Replace or start new client
		if hasExisting && existing.Client != nil {
			if m.registry != nil {
				for _, t := range existing.Tools {
					m.registry.Unregister(fmt.Sprintf("%s_%s", name, t.Name))
				}
			}
			existing.Client.Close()
			existing.Client = nil
		}

		managed := &ManagedServer{
			Name:   name,
			Config: srvCfg,
			Status: "ok",
		}
		m.servers[name] = managed

		if srvCfg.Command == "" {
			continue
		}

		client, err := NewClient(name, srvCfg)
		if err != nil {
			slog.Error("failed to start mcp server", "server", name, "error", err)
			managed.Status = "error"
			managed.Error = err.Error()
			continue
		}
		managed.Client = client

		// Initialize
		initParams := map[string]any{
			"protocolVersion": "2024-11-05",
			"clientInfo": map[string]string{
				"name":    "OllamaDesktop",
				"version": "1.0.0",
			},
			"capabilities": map[string]any{},
		}
		_, err = client.Call("initialize", initParams)
		if err != nil {
			slog.Error("failed to initialize mcp server", "server", name, "error", err)
			client.Close()
			managed.Client = nil
			managed.Status = "error"
			managed.Error = err.Error()
			continue
		}

		initNotif := RPCMessage{
			Jsonrpc: "2.0",
			Method:  "notifications/initialized",
		}
		initB, _ := json.Marshal(initNotif)
		initB = append(initB, '\n')
		client.in.Write(initB)

		// List tools
		res, err := client.Call("tools/list", map[string]any{})
		if err != nil {
			slog.Error("failed to list mcp tools", "server", name, "error", err)
			client.Close()
			managed.Client = nil
			managed.Status = "error"
			managed.Error = err.Error()
			continue
		}

		var toolsRes struct {
			Tools []struct {
				Name        string         `json:"name"`
				Description string         `json:"description"`
				InputSchema map[string]any `json:"inputSchema"`
			} `json:"tools"`
		}
		if err := json.Unmarshal(res.Result, &toolsRes); err != nil {
			slog.Error("failed to parse mcp tools", "server", name, "error", err)
			client.Close()
			managed.Client = nil
			managed.Status = "error"
			managed.Error = err.Error()
			continue
		}

		for _, t := range toolsRes.Tools {
			isDisabled := isToolDisabledInState(state, name, t.Name)
			item := MCPToolItem{
				Name:        t.Name,
				Description: t.Description,
				Disabled:    isDisabled,
				InputSchema: t.InputSchema,
			}
			managed.Tools = append(managed.Tools, item)

			if !isDisabled && m.registry != nil {
				prefixedName := fmt.Sprintf("%s_%s", name, t.Name)
				mcpTool := &MCPTool{
					client:      client,
					name:        t.Name,
					description: fmt.Sprintf("[%s] %s", name, t.Description),
					inputSchema: t.InputSchema,
				}
				m.registry.Register(&RegistryMCPTool{MCPTool: mcpTool, registeredName: prefixedName})
			}
		}
		slog.Info("registered mcp server", "server", name, "tools", len(managed.Tools))
	}

	return nil
}

func (m *Manager) GetServerTools(serverName string) []MCPToolItem {
	state, _ := loadToolState()

	m.mu.RLock()
	defer m.mu.RUnlock()

	srv, ok := m.servers[serverName]
	if !ok {
		return nil
	}
	out := make([]MCPToolItem, len(srv.Tools))
	for i, t := range srv.Tools {
		out[i] = t
		out[i].Disabled = isToolDisabledInState(state, serverName, t.Name)
	}
	return out
}

func (m *Manager) SetToolDisabled(serverName, toolName string, disabled bool) error {
	state, err := loadToolState()
	if err != nil {
		return err
	}
	if state.DisabledTools == nil {
		state.DisabledTools = make(map[string][]string)
	}

	currentList := state.DisabledTools[serverName]
	var newList []string
	for _, t := range currentList {
		if !strings.EqualFold(t, toolName) {
			newList = append(newList, t)
		}
	}
	if disabled {
		newList = append(newList, toolName)
	}
	if len(newList) > 0 {
		state.DisabledTools[serverName] = newList
	} else {
		delete(state.DisabledTools, serverName)
	}

	if err := saveToolState(state); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	srv, ok := m.servers[serverName]
	if ok {
		for i := range srv.Tools {
			if strings.EqualFold(srv.Tools[i].Name, toolName) {
				srv.Tools[i].Disabled = disabled
				prefixedName := fmt.Sprintf("%s_%s", serverName, toolName)
				if m.registry != nil {
					if disabled {
						m.registry.Unregister(prefixedName)
						slog.Info("unregistered disabled mcp tool", "server", serverName, "tool", toolName)
					} else if srv.Client != nil {
						mcpTool := &MCPTool{
							client:      srv.Client,
							name:        toolName,
							description: fmt.Sprintf("[%s] %s", serverName, srv.Tools[i].Description),
							inputSchema: srv.Tools[i].InputSchema,
						}
						m.registry.Register(&RegistryMCPTool{MCPTool: mcpTool, registeredName: prefixedName})
						slog.Info("registered enabled mcp tool", "server", serverName, "tool", toolName)
					}
				}
				break
			}
		}
	}

	return nil
}

// StartManager initializes MCP servers and registers their tools
func StartManager(registry *tools.Registry) error {
	mgr := GetManager()
	mgr.SetRegistry(registry)
	return mgr.Sync()
}

func SyncManager(registry *tools.Registry) error {
	mgr := GetManager()
	if registry != nil {
		mgr.SetRegistry(registry)
	}
	return mgr.Sync()
}

func GetServerTools(serverName string) []MCPToolItem {
	return GetManager().GetServerTools(serverName)
}

func SetToolDisabled(serverName, toolName string, disabled bool) error {
	return GetManager().SetToolDisabled(serverName, toolName, disabled)
}

type RegistryMCPTool struct {
	*MCPTool
	registeredName string
}

func (t *RegistryMCPTool) Name() string {
	return t.registeredName
}
