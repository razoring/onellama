//go:build windows || darwin

package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMCPConfigIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "claude_desktop_config.json")

	rawContent := `{
  "mcpServers": {
    "valid-server": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-memory"]
    },
    "broken-server": {
      "args": "invalid-args-not-an-array"
    },
    "valid-sse": {
      "url": "http://localhost:8000/sse"
    }
  }
}`

	if err := os.WriteFile(configPath, []byte(rawContent), 0644); err != nil {
		t.Fatalf("failed to write test config file: %v", err)
	}

	rawBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read test config file: %v", err)
	}

	var rootObj map[string]json.RawMessage
	if err := json.Unmarshal(rawBytes, &rootObj); err != nil {
		t.Fatalf("unexpected root json unmarshal error: %v", err)
	}

	var serversMap map[string]json.RawMessage
	if err := json.Unmarshal(rootObj["mcpServers"], &serversMap); err != nil {
		t.Fatalf("unexpected mcpServers unmarshal error: %v", err)
	}

	parsedServers := make(map[string]string)
	for name, rawServer := range serversMap {
		item := parseSingleMCPServer(name, rawServer)
		parsedServers[name] = item.Status
	}

	if parsedServers["valid-server"] != "ok" {
		t.Errorf("expected valid-server status to be 'ok', got '%s'", parsedServers["valid-server"])
	}

	if parsedServers["broken-server"] != "error" {
		t.Errorf("expected broken-server status to be 'error', got '%s'", parsedServers["broken-server"])
	}

	if parsedServers["valid-sse"] != "ok" {
		t.Errorf("expected valid-sse status to be 'ok', got '%s'", parsedServers["valid-sse"])
	}
}

func TestMCPSaveValidation(t *testing.T) {
	invalidJSON := `{ "mcpServers": { "broken": } }`

	var check map[string]json.RawMessage
	if err := json.Unmarshal([]byte(invalidJSON), &check); err == nil {
		t.Error("expected syntax error for invalid json, got nil")
	}
}
