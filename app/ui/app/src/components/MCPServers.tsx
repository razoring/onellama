import { useEffect, useState, useCallback, useMemo } from "react";
import { getMCPConfig, saveMCPConfig } from "@/api";
import { MCPConfigResponse, MCPServerItem } from "@/gotypes";
import { Button } from "@/components/ui/button";
import {
  CheckIcon,
  ClipboardDocumentIcon,
  ExclamationTriangleIcon,
  ArrowPathIcon,
  PlusIcon,
  ChevronDownIcon,
  ChevronUpIcon,
  EyeIcon,
  EyeSlashIcon,
  CodeBracketIcon,
} from "@heroicons/react/20/solid";

const STDIO_TEMPLATE = `{
  "command": "npx",
  "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/dir"]
}`;

const SSE_TEMPLATE = `{
  "url": "http://localhost:8000/sse"
}`;

export function MCPServers() {
  const [config, setConfig] = useState<MCPConfigResponse | null>(null);
  const [rawInput, setRawInput] = useState<string>("");
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [isSaving, setIsSaving] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const [saveSuccess, setSaveSuccess] = useState<boolean>(false);
  const [copiedPath, setCopiedPath] = useState<boolean>(false);
  const [expandedServer, setExpandedServer] = useState<string | null>(null);
  const [showEnvValues, setShowEnvValues] = useState<Record<string, boolean>>({});

  const fetchConfig = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await getMCPConfig();
      setConfig(res);
      setRawInput(res.raw || "{\n  \"mcpServers\": {}\n}");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to load MCP configuration");
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchConfig();
  }, [fetchConfig]);

  //live syntax checking for raw JSON editor
  const syntaxError = useMemo(() => {
    if (!rawInput.trim()) return null;
    try {
      JSON.parse(rawInput);
      return null;
    } catch (e: unknown) {
      return e instanceof Error ? e.message : "Invalid JSON syntax";
    }
  }, [rawInput]);

  const isDirty = useMemo(() => {
    return config ? rawInput !== config.raw : false;
  }, [config, rawInput]);

  const handleSave = async () => {
    if (syntaxError) return;
    setIsSaving(true);
    setError(null);
    setSaveSuccess(false);
    try {
      const updated = await saveMCPConfig(rawInput);
      setConfig(updated);
      setRawInput(updated.raw);
      setSaveSuccess(true);
      setTimeout(() => setSaveSuccess(false), 3000);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to save configuration");
    } finally {
      setIsSaving(false);
    }
  };

  const handleFormat = () => {
    try {
      const parsed = JSON.parse(rawInput);
      setRawInput(JSON.stringify(parsed, null, 2));
    } catch (e) {
      //syntax error will be caught by syntaxError alert
    }
  };

  const handleCopyPath = () => {
    if (!config?.configPath) return;
    navigator.clipboard.writeText(config.configPath);
    setCopiedPath(true);
    setTimeout(() => setCopiedPath(false), 2000);
  };

  const handleAddTemplate = (type: "stdio" | "sse") => {
    try {
      const parsed = JSON.parse(rawInput || '{"mcpServers": {}}');
      if (!parsed.mcpServers || typeof parsed.mcpServers !== "object") {
        parsed.mcpServers = {};
      }
      const newName = type === "stdio" ? "filesystem-server" : "remote-sse-server";
      parsed.mcpServers[newName] = JSON.parse(type === "stdio" ? STDIO_TEMPLATE : SSE_TEMPLATE);
      const formatted = JSON.stringify(parsed, null, 2);
      setRawInput(formatted);
    } catch (e) {
      setError("Cannot add template while JSON has syntax errors.");
    }
  };

  const toggleEnvMask = (serverName: string) => {
    setShowEnvValues((prev) => ({ ...prev, [serverName]: !prev[serverName] }));
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center p-12 text-neutral-500">
        <ArrowPathIcon className="h-6 w-6 animate-spin mr-2" />
        <span>Loading MCP configuration...</span>
      </div>
    );
  }

  const validCount = config?.servers.filter((s) => s.status === "ok").length || 0;
  const brokenCount = config?.servers.filter((s) => s.status === "error").length || 0;

  return (
    <div className="flex flex-col gap-6 p-6 max-w-7xl mx-auto w-full">
      {/* Top Banner & Status */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-neutral-200 dark:border-neutral-800 pb-4">
        <div>
          <h1 className="text-xl font-semibold text-neutral-900 dark:text-neutral-100 flex items-center gap-2">
            <CodeBracketIcon className="h-6 w-6 text-neutral-500" />
            MCP Servers Configuration
          </h1>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
            Configure Model Context Protocol servers in standard Claude desktop format.
          </p>
        </div>

        <div className="flex items-center gap-3">
          <div className="flex items-center gap-2 text-xs">
            <span className="px-2.5 py-1 rounded-full bg-green-100 text-green-800 dark:bg-green-950 dark:text-green-300 font-medium">
              {validCount} Active
            </span>
            {brokenCount > 0 && (
              <span className="px-2.5 py-1 rounded-full bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-300 font-medium">
                {brokenCount} Broken
              </span>
            )}
          </div>
          <Button onClick={fetchConfig} plain className="text-xs">
            <ArrowPathIcon className="h-4 w-4" />
            Refresh
          </Button>
        </div>
      </div>

      {error && (
        <div className="p-4 rounded-lg bg-red-50 border border-red-200 dark:bg-red-950/40 dark:border-red-900 text-red-700 dark:text-red-300 text-sm flex items-start gap-2">
          <ExclamationTriangleIcon className="h-5 w-5 shrink-0 mt-0.5 text-red-500" />
          <div>{error}</div>
        </div>
      )}

      {/* Split View Layout */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 min-h-[500px]">
        {/* Left Column: Server Chips & Cards */}
        <div className="flex flex-col gap-4">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
              Server Chips ({config?.servers.length || 0})
            </h2>
            <div className="flex items-center gap-2">
              <span className="text-xs text-neutral-400">Add template:</span>
              <button
                onClick={() => handleAddTemplate("stdio")}
                className="text-xs px-2 py-1 rounded bg-neutral-100 hover:bg-neutral-200 dark:bg-neutral-800 dark:hover:bg-neutral-700 text-neutral-700 dark:text-neutral-300 flex items-center gap-1"
              >
                <PlusIcon className="h-3.5 w-3.5" /> stdio
              </button>
              <button
                onClick={() => handleAddTemplate("sse")}
                className="text-xs px-2 py-1 rounded bg-neutral-100 hover:bg-neutral-200 dark:bg-neutral-800 dark:hover:bg-neutral-700 text-neutral-700 dark:text-neutral-300 flex items-center gap-1"
              >
                <PlusIcon className="h-3.5 w-3.5" /> sse
              </button>
            </div>
          </div>

          {config?.parseError && (
            <div className="p-4 rounded-lg bg-amber-50 border border-amber-200 dark:bg-amber-950/40 dark:border-amber-900 text-amber-800 dark:text-amber-300 text-sm">
              <div className="font-semibold flex items-center gap-2 mb-1">
                <ExclamationTriangleIcon className="h-5 w-5 text-amber-500" />
                Config Parsing Warning
              </div>
              {config.parseError}
            </div>
          )}

          {config?.servers.length === 0 ? (
            <div className="flex flex-col items-center justify-center p-8 border border-dashed border-neutral-300 dark:border-neutral-700 rounded-xl text-center">
              <CodeBracketIcon className="h-10 w-10 text-neutral-400 mb-2" />
              <p className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
                No MCP servers configured
              </p>
              <p className="text-xs text-neutral-500 mt-1 max-w-sm">
                Add server definitions using the raw JSON editor on the right or click a template button above.
              </p>
            </div>
          ) : (
            <div className="flex flex-col gap-3">
              {config?.servers.map((server: MCPServerItem) => {
                const isExpanded = expandedServer === server.name;
                const isBroken = server.status === "error";

                return (
                  <div
                    key={server.name}
                    className={`border rounded-xl p-4 transition-colors ${
                      isBroken
                        ? "border-red-300 bg-red-50/50 dark:border-red-900 dark:bg-red-950/20"
                        : "border-neutral-200 bg-white dark:border-neutral-800 dark:bg-neutral-900"
                    }`}
                  >
                    <div className="flex items-center justify-between gap-3">
                      <div className="flex items-center gap-2 min-w-0">
                        {/* Status Chip Dot */}
                        <span
                          className={`h-2.5 w-2.5 rounded-full shrink-0 ${
                            isBroken
                              ? "bg-red-500 shadow-red-500/50 shadow-sm"
                              : server.disabled
                              ? "bg-neutral-400"
                              : "bg-green-500 shadow-green-500/50 shadow-sm"
                          }`}
                        />
                        <span className="font-semibold text-sm text-neutral-900 dark:text-neutral-100 truncate">
                          {server.name}
                        </span>
                        <span className="text-[10px] uppercase font-mono tracking-wider px-2 py-0.5 rounded bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400">
                          {server.type}
                        </span>
                      </div>

                      <button
                        onClick={() =>
                          setExpandedServer(isExpanded ? null : server.name)
                        }
                        className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 p-1"
                      >
                        {isExpanded ? (
                          <ChevronUpIcon className="h-4 w-4" />
                        ) : (
                          <ChevronDownIcon className="h-4 w-4" />
                        )}
                      </button>
                    </div>

                    {/* Brief Sub-header */}
                    <div className="mt-2 text-xs font-mono text-neutral-500 dark:text-neutral-400 truncate">
                      {server.command ? (
                        <span>
                          {server.command} {(server.args || []).join(" ")}
                        </span>
                      ) : server.url ? (
                        <span>{server.url}</span>
                      ) : (
                        <span className="italic text-red-500">Malformed server entry</span>
                      )}
                    </div>

                    {/* Diagnostics for Broken Server */}
                    {isBroken && server.error && (
                      <div className="mt-3 p-2.5 rounded bg-red-100 dark:bg-red-950/60 text-red-800 dark:text-red-200 text-xs font-sans border border-red-200 dark:border-red-900 flex items-start gap-2">
                        <ExclamationTriangleIcon className="h-4 w-4 shrink-0 text-red-500 mt-0.5" />
                        <div>
                          <strong>Fault Diagnostic:</strong> {server.error}
                        </div>
                      </div>
                    )}

                    {/* Expanded Details */}
                    {isExpanded && !isBroken && (
                      <div className="mt-4 pt-3 border-t border-neutral-100 dark:border-neutral-800 flex flex-col gap-3 text-xs">
                        {server.command && (
                          <div>
                            <span className="text-neutral-400 font-medium block mb-1">
                              Command & Arguments:
                            </span>
                            <div className="bg-neutral-50 dark:bg-neutral-950 p-2 rounded font-mono text-neutral-800 dark:text-neutral-200 overflow-x-auto">
                              {server.command} {(server.args || []).join(" ")}
                            </div>
                          </div>
                        )}

                        {server.url && (
                          <div>
                            <span className="text-neutral-400 font-medium block mb-1">
                              SSE Endpoint URL:
                            </span>
                            <div className="bg-neutral-50 dark:bg-neutral-950 p-2 rounded font-mono text-neutral-800 dark:text-neutral-200 truncate">
                              {server.url}
                            </div>
                          </div>
                        )}

                        {server.env && Object.keys(server.env).length > 0 && (
                          <div>
                            <div className="flex items-center justify-between mb-1">
                              <span className="text-neutral-400 font-medium">
                                Environment Variables:
                              </span>
                              <button
                                onClick={() => toggleEnvMask(server.name)}
                                className="text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300 flex items-center gap-1"
                              >
                                {showEnvValues[server.name] ? (
                                  <>
                                    <EyeSlashIcon className="h-3.5 w-3.5" /> Mask
                                  </>
                                ) : (
                                  <>
                                    <EyeIcon className="h-3.5 w-3.5" /> Reveal
                                  </>
                                )}
                              </button>
                            </div>
                            <div className="bg-neutral-50 dark:bg-neutral-950 p-2 rounded font-mono flex flex-col gap-1 text-neutral-800 dark:text-neutral-200">
                              {Object.entries(server.env).map(([k, v]) => (
                                <div key={k} className="flex justify-between gap-2 truncate">
                                  <span className="text-neutral-500">{k}=</span>
                                  <span className="truncate">
                                    {showEnvValues[server.name]
                                      ? v
                                      : "••••••••••••••••"}
                                  </span>
                                </div>
                              ))}
                            </div>
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </div>

        {/* Right Column: Live Raw JSON Editor */}
        <div className="flex flex-col gap-4">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
              Raw JSON Editor
            </h2>

            <div className="flex items-center gap-2">
              <button
                onClick={handleFormat}
                disabled={!!syntaxError}
                className="text-xs px-2.5 py-1 rounded bg-neutral-100 hover:bg-neutral-200 dark:bg-neutral-800 dark:hover:bg-neutral-700 text-neutral-700 dark:text-neutral-300 disabled:opacity-50"
              >
                Format JSON
              </button>
              <button
                onClick={() => setRawInput(config?.raw || "")}
                disabled={!isDirty}
                className="text-xs px-2.5 py-1 rounded border border-neutral-300 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:opacity-40"
              >
                Revert
              </button>
            </div>
          </div>

          {/* Config Path Indicator */}
          {config?.configPath && (
            <div className="flex items-center justify-between gap-2 px-3 py-1.5 bg-neutral-100 dark:bg-neutral-800 rounded-lg text-xs font-mono text-neutral-600 dark:text-neutral-400">
              <span className="truncate">File: {config.configPath}</span>
              <button
                onClick={handleCopyPath}
                className="shrink-0 hover:text-neutral-900 dark:hover:text-neutral-100"
                title="Copy Path"
              >
                {copiedPath ? (
                  <CheckIcon className="h-3.5 w-3.5 text-green-500" />
                ) : (
                  <ClipboardDocumentIcon className="h-3.5 w-3.5" />
                )}
              </button>
            </div>
          )}

          {/* Syntax Error Alert */}
          {syntaxError && (
            <div className="p-3 rounded-lg bg-red-50 border border-red-200 dark:bg-red-950/40 dark:border-red-900 text-red-700 dark:text-red-300 text-xs font-mono flex items-start gap-2">
              <ExclamationTriangleIcon className="h-4 w-4 shrink-0 text-red-500 mt-0.5" />
              <div>Syntax Error: {syntaxError}</div>
            </div>
          )}

          {/* Raw Textarea */}
          <div className="relative flex-1 min-h-[350px] flex flex-col">
            <textarea
              value={rawInput}
              onChange={(e) => setRawInput(e.target.value)}
              className="w-full flex-1 p-4 rounded-xl border border-neutral-300 dark:border-neutral-700 bg-neutral-900 text-neutral-100 font-mono text-xs leading-relaxed focus:outline-none focus:ring-2 focus:ring-neutral-500 resize-y"
              spellCheck={false}
              rows={20}
            />
          </div>

          {/* Save Action Footer */}
          <div className="flex items-center justify-between">
            <div className="text-xs text-neutral-400">
              {isDirty ? (
                <span className="text-amber-500 font-medium">Unsaved changes</span>
              ) : saveSuccess ? (
                <span className="text-green-500 font-medium flex items-center gap-1">
                  <CheckIcon className="h-4 w-4" /> Configuration saved successfully
                </span>
              ) : (
                <span>All changes saved</span>
              )}
            </div>

            <Button
              onClick={handleSave}
              disabled={!isDirty || !!syntaxError || isSaving}
              className="px-4 py-2 text-xs font-medium"
            >
              {isSaving ? "Saving..." : "Save Changes"}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
