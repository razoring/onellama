//go:build windows || darwin

package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ollama/ollama/app/ui/responses"
)

//mcpConfigFilename defines the default config file name
const mcpConfigFilename = "claude_desktop_config.json"

//getMCPConfigPath returns the path to the Ollama MCP configuration file
func getMCPConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home directory not found: %w", err)
	}
	return filepath.Join(homeDir, ".ollama", mcpConfigFilename), nil
}

//loadMCPConfig reads raw JSON and tolerant-parses each MCP server entry independently
func loadMCPConfig() (responses.MCPConfigResponse, error) {
	configPath, err := getMCPConfigPath()
	if err != nil {
		return responses.MCPConfigResponse{}, err
	}

	//create directory if missing
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return responses.MCPConfigResponse{}, fmt.Errorf("failed to create config dir: %w", err)
	}

	//create default config file if it does not exist
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		defaultContent := "{\n  \"mcpServers\": {}\n}\n"
		_ = os.WriteFile(configPath, []byte(defaultContent), 0644)
	}

	rawBytes, err := os.ReadFile(configPath)
	if err != nil {
		return responses.MCPConfigResponse{}, fmt.Errorf("failed to read mcp config: %w", err)
	}

	rawString := string(rawBytes)
	res := responses.MCPConfigResponse{
		Raw:        rawString,
		ConfigPath: configPath,
		Servers:    []responses.MCPServerItem{},
	}

	var rootObj map[string]json.RawMessage
	if err := json.Unmarshal(rawBytes, &rootObj); err != nil {
		res.ParseError = fmt.Sprintf("Invalid JSON syntax: %v", err)
		return res, nil
	}

	rawServers, exists := rootObj["mcpServers"]
	if !exists {
		return res, nil
	}

	var serversMap map[string]json.RawMessage
	if err := json.Unmarshal(rawServers, &serversMap); err != nil {
		res.ParseError = fmt.Sprintf("'mcpServers' must be an object: %v", err)
		return res, nil
	}

	//parse each mcp server independently to prevent one broken server from breaking others
	for serverName, rawServer := range serversMap {
		item := parseSingleMCPServer(serverName, rawServer)
		res.Servers = append(res.Servers, item)
	}

	return res, nil
}

//parseSingleMCPServer validates individual server definitions and returns diagnostic status
func parseSingleMCPServer(name string, raw json.RawMessage) responses.MCPServerItem {
	item := responses.MCPServerItem{
		Name:   name,
		Status: "ok",
		Type:   "unknown",
	}

	var rawMap map[string]any
	if err := json.Unmarshal(raw, &rawMap); err != nil {
		item.Status = "error"
		item.Error = fmt.Sprintf("Invalid JSON format for server '%s': %v", name, err)
		return item
	}

	//check disabled status
	if disabledVal, ok := rawMap["disabled"].(bool); ok {
		item.Disabled = disabledVal
	}

	commandVal, hasCommand := rawMap["command"].(string)
	urlVal, hasURL := rawMap["url"].(string)

	if !hasCommand && !hasURL {
		item.Status = "error"
		item.Error = "Server requires either a 'command' (stdio transport) or 'url' (sse transport)"
		return item
	}

	if hasCommand {
		item.Type = "stdio"
		item.Command = commandVal

		if argsRaw, hasArgs := rawMap["args"]; hasArgs {
			if argsSlice, ok := argsRaw.([]any); ok {
				var strArgs []string
				for _, arg := range argsSlice {
					if strArg, isStr := arg.(string); isStr {
						strArgs = append(strArgs, strArg)
					} else {
						item.Status = "error"
						item.Error = "Arguments in 'args' array must be strings"
						return item
					}
				}
				item.Args = strArgs
			} else {
				item.Status = "error"
				item.Error = "'args' must be an array of strings"
				return item
			}
		}

		if envRaw, hasEnv := rawMap["env"]; hasEnv {
			if envMap, ok := envRaw.(map[string]any); ok {
				strEnv := make(map[string]string)
				for k, v := range envMap {
					if strV, isStr := v.(string); isStr {
						strEnv[k] = strV
					} else {
						strEnv[k] = fmt.Sprintf("%v", v)
					}
				}
				item.Env = strEnv
			} else {
				item.Status = "error"
				item.Error = "'env' must be a key-value object"
				return item
			}
		}
	} else if hasURL {
		item.Type = "sse"
		item.URL = urlVal
	}

	return item
}

//saveMCPConfig validates raw JSON and atomically writes to config path with backup
func saveMCPConfig(raw string) (responses.MCPConfigResponse, error) {
	configPath, err := getMCPConfigPath()
	if err != nil {
		return responses.MCPConfigResponse{}, err
	}

	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		trimmed = "{\n  \"mcpServers\": {}\n}"
	}

	//validate JSON syntax before saving
	var check map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &check); err != nil {
		return responses.MCPConfigResponse{}, fmt.Errorf("Invalid JSON syntax: %w", err)
	}

	//create backup of existing file if present
	if _, err := os.Stat(configPath); err == nil {
		backupPath := configPath + ".bak"
		existingBytes, readErr := os.ReadFile(configPath)
		if readErr == nil {
			_ = os.WriteFile(backupPath, existingBytes, 0644)
		}
	}

	//write to config file
	if err := os.WriteFile(configPath, []byte(trimmed), 0644); err != nil {
		return responses.MCPConfigResponse{}, fmt.Errorf("failed to save mcp config: %w", err)
	}

	return loadMCPConfig()
}
