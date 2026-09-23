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

// StartManager initializes MCP servers and registers their tools
func StartManager(registry *tools.Registry) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cfgPath := filepath.Join(homeDir, ".ollama", "claude_desktop_config.json")
	
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No config
		}
		return err
	}

	var cfg ConfigFile
	if err := json.Unmarshal(b, &cfg); err != nil {
		return err
	}

	for name, srvCfg := range cfg.MCPServers {
		if srvCfg.Disabled {
			continue
		}
		
		client, err := NewClient(name, srvCfg)
		if err != nil {
			slog.Error("failed to start mcp server", "server", name, "error", err)
			continue
		}

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
			continue
		}
		
		// Send initialized notification
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
			continue
		}

		// Register tools
		for _, t := range toolsRes.Tools {
			// Prefix tool name with server name to avoid collisions
			prefixedName := fmt.Sprintf("%s_%s", name, t.Name)
			mcpTool := &MCPTool{
				client:      client,
				name:        t.Name, // Keep original name for execution
				description: fmt.Sprintf("[%s] %s", name, t.Description),
				inputSchema: t.InputSchema,
			}
			// Use a wrapper to intercept Name() for the registry while keeping original for Call()
			registry.Register(&RegistryMCPTool{MCPTool: mcpTool, registeredName: prefixedName})
		}
		slog.Info("registered mcp server", "server", name, "tools", len(toolsRes.Tools))
	}

	return nil
}

type RegistryMCPTool struct {
	*MCPTool
	registeredName string
}

func (t *RegistryMCPTool) Name() string {
	return t.registeredName
}
