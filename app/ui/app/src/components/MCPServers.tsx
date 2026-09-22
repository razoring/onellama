import { useEffect, useState, useCallback, useMemo } from "react";
import { getMCPConfig, saveMCPConfig } from "@/api";
import { MCPConfigResponse, MCPServerItem } from "@/gotypes";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
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
    } catch {
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
    } catch {
      setError("Cannot add template while JSON has syntax errors.");
    }
  };

  const toggleEnvMask = (serverName: string) => {
    setShowEnvValues((prev) => ({ ...prev, [serverName]: !prev[serverName] }));
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center gap-2 bg-white p-12 text-sm text-neutral-500 dark:bg-neutral-900 dark:text-neutral-400">
        <ArrowPathIcon className="h-5 w-5 animate-spin" />
        <span>Loading MCP configuration...</span>
      </div>
    );
  }

  const validCount = config?.servers.filter((s) => s.status === "ok").length || 0;
  const disabledCount = config?.servers.filter((s) => s.disabled).length || 0;
  const brokenCount = config?.servers.filter((s) => s.status === "error").length || 0;

  return (
    <div className="flex min-h-0 flex-1 flex-col bg-white text-neutral-900 dark:bg-neutral-900 dark:text-neutral-100">
      {/* Header */}
      <div className="flex shrink-0 flex-wrap items-center justify-between gap-3 border-b border-neutral-200 px-6 py-3 dark:border-neutral-800">
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-2">
            <h2 className="text-sm font-medium text-neutral-900 dark:text-white">
              MCP Servers Configuration
            </h2>
            <Badge color="green">{validCount} Active</Badge>
            {disabledCount > 0 && <Badge color="zinc">{disabledCount} Disabled</Badge>}
            {brokenCount > 0 && <Badge color="red">{brokenCount} Broken</Badge>}
          </div>
        </div>
        <Button type="button" color="white" className="px-3" onClick={fetchConfig}>
          <ArrowPathIcon data-slot="icon" />
          Refresh
        </Button>
      </div>

      {error && (
        <div className="flex items-start gap-2 border-b border-neutral-200 px-6 py-3 text-sm text-red-700 dark:border-neutral-800 dark:text-red-300">
          <ExclamationTriangleIcon className="mt-0.5 h-5 w-5 shrink-0 text-red-500" />
          <div>{error}</div>
        </div>
      )}

      {config?.parseError && (
        <div className="flex items-start gap-2 border-b border-neutral-200 px-6 py-3 text-sm text-amber-700 dark:border-neutral-800 dark:text-amber-300">
          <ExclamationTriangleIcon className="mt-0.5 h-5 w-5 shrink-0 text-amber-500" />
          <div>
            <strong>Config Parsing Warning:</strong> {config.parseError}
          </div>
        </div>
      )}

      {/* Single toolbar */}
      <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b border-neutral-200 px-6 py-2 dark:border-neutral-800">
        <div className="flex items-center gap-2">
          <span className="mr-1 text-xs text-neutral-500 dark:text-neutral-400">
            Add template:
          </span>
          <button
            onClick={() => handleAddTemplate("stdio")}
            className="flex items-center gap-1 rounded-md bg-neutral-100 px-2 py-1 text-xs font-medium text-neutral-700 hover:bg-neutral-200 dark:bg-neutral-800 dark:text-neutral-300 dark:hover:bg-neutral-700"
          >
            <PlusIcon className="h-3.5 w-3.5" /> stdio
          </button>
          <button
            onClick={() => handleAddTemplate("sse")}
            className="flex items-center gap-1 rounded-md bg-neutral-100 px-2 py-1 text-xs font-medium text-neutral-700 hover:bg-neutral-200 dark:bg-neutral-800 dark:text-neutral-300 dark:hover:bg-neutral-700"
          >
            <PlusIcon className="h-3.5 w-3.5" /> sse
          </button>
        </div>

        <div className="flex items-center gap-2">
          <button
            onClick={handleFormat}
            disabled={!!syntaxError}
            className="rounded-md bg-neutral-100 px-2.5 py-1 text-xs font-medium text-neutral-700 hover:bg-neutral-200 disabled:opacity-50 dark:bg-neutral-800 dark:text-neutral-300 dark:hover:bg-neutral-700"
          >
            Format JSON
          </button>
          <button
            onClick={() => setRawInput(config?.raw || "")}
            disabled={!isDirty}
            className="rounded-md border border-neutral-300 px-2.5 py-1 text-xs font-medium text-neutral-600 hover:bg-neutral-100 disabled:opacity-40 dark:border-neutral-700 dark:text-neutral-400 dark:hover:bg-neutral-800"
          >
            Revert
          </button>
        </div>
      </div>

      {/* Two-pane body */}
      <div className="flex min-h-0 flex-1 gap-6 overflow-hidden p-6">
        {/* Left: server cards */}
        <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
          <h3 className="mb-3 text-sm font-medium text-neutral-900 dark:text-white">
            Configured Servers ({config?.servers.length || 0})
          </h3>

          <div className="min-h-0 flex-1 overflow-y-auto pr-1">
            {config?.servers.length === 0 ? (
              <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-neutral-300 p-8 py-12 text-center dark:border-neutral-800">
                <CodeBracketIcon className="mb-3 h-10 w-10 text-neutral-400" />
                <p className="text-sm font-medium text-neutral-900 dark:text-white">
                  No MCP servers configured
                </p>
                <p className="mt-1 max-w-sm text-xs text-neutral-500">
                  Add server definitions using the raw JSON editor or click a template button above.
                </p>
              </div>
            ) : (
              <div className="flex flex-col gap-3">
                {config?.servers.map((server: MCPServerItem) => {
                  const isExpanded = expandedServer === server.name;
                  const isBroken = server.status === "error";
                  const isDisabled = server.disabled;

                  return (
                    <div
                      key={server.name}
                      className={`rounded-xl border p-4 ${
                        isBroken
                          ? "border-red-300 bg-red-50/50 dark:border-red-900 dark:bg-red-950/20"
                          : "border-neutral-200 bg-white dark:border-neutral-800 dark:bg-neutral-900"
                      }`}
                    >
                      <div className="flex items-center justify-between gap-3">
                        <div className="flex min-w-0 items-center gap-2">
                          <span className="truncate text-sm font-semibold text-neutral-900 dark:text-neutral-100">
                            {server.name}
                          </span>
                          <Badge
                            color={isBroken ? "red" : isDisabled ? "zinc" : "green"}
                          >
                            {isBroken ? "Error" : isDisabled ? "Disabled" : "Active"}
                          </Badge>
                          <Badge color="zinc">{server.type}</Badge>
                        </div>

                        <button
                          onClick={() =>
                            setExpandedServer(isExpanded ? null : server.name)
                          }
                          className="shrink-0 p-1 text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200"
                        >
                          {isExpanded ? (
                            <ChevronUpIcon className="h-4 w-4" />
                          ) : (
                            <ChevronDownIcon className="h-4 w-4" />
                          )}
                        </button>
                      </div>

                      {/* Brief sub-header */}
                      <div className="mt-2 truncate font-mono text-xs text-neutral-500 dark:text-neutral-400">
                        {server.command ? (
                          <span>
                            {server.command} {(server.args || []).join(" ")}
                          </span>
                        ) : server.url ? (
                          <span>{server.url}</span>
                        ) : (
                          <span className="italic text-red-500">
                            Malformed server entry
                          </span>
                        )}
                      </div>

                      {/* Broken servers always surface their fault */}
                      {isBroken && server.error && (
                        <div className="mt-3 flex items-start gap-2 rounded-lg border border-red-200 bg-red-100 p-2.5 text-xs text-red-800 dark:border-red-900 dark:bg-red-950/60 dark:text-red-200">
                          <ExclamationTriangleIcon className="mt-0.5 h-4 w-4 shrink-0 text-red-500" />
                          <div>
                            <strong>Fault Diagnostic:</strong> {server.error}
                          </div>
                        </div>
                      )}

                      {/* Expanded details */}
                      {isExpanded && !isBroken && (
                        <div className="mt-4 flex flex-col gap-3 border-t border-neutral-100 pt-3 text-xs dark:border-neutral-800">
                          {server.command && (
                            <div>
                              <span className="mb-1 block font-medium text-neutral-400">
                                Command & Arguments:
                              </span>
                              <div className="overflow-x-auto rounded bg-neutral-100 p-2 font-mono text-neutral-800 dark:bg-neutral-950 dark:text-neutral-200">
                                {server.command} {(server.args || []).join(" ")}
                              </div>
                            </div>
                          )}

                          {server.url && (
                            <div>
                              <span className="mb-1 block font-medium text-neutral-400">
                                SSE Endpoint URL:
                              </span>
                              <div className="truncate rounded bg-neutral-100 p-2 font-mono text-neutral-800 dark:bg-neutral-950 dark:text-neutral-200">
                                {server.url}
                              </div>
                            </div>
                          )}

                          {server.env && Object.keys(server.env).length > 0 && (
                            <div>
                              <div className="mb-1 flex items-center justify-between">
                                <span className="font-medium text-neutral-400">
                                  Environment Variables:
                                </span>
                                <button
                                  onClick={() => toggleEnvMask(server.name)}
                                  className="flex items-center gap-1 text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300"
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
                              <div className="flex flex-col gap-1 rounded bg-neutral-100 p-2 font-mono text-neutral-800 dark:bg-neutral-950 dark:text-neutral-200">
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
        </div>

        {/* Right: raw JSON editor */}
        <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
          <div className="mb-3 flex items-center justify-between gap-2">
            <h3 className="text-sm font-medium text-neutral-900 dark:text-white">
              Raw JSON Editor
            </h3>
            {config?.configPath && (
              <div className="flex items-center gap-2 rounded-lg bg-neutral-100 px-3 py-1.5 font-mono text-xs text-neutral-600 dark:bg-neutral-800 dark:text-neutral-400">
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
          </div>

          {syntaxError && (
            <div className="mb-3 flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 p-3 font-mono text-xs text-red-700 dark:border-red-900 dark:bg-red-950/40 dark:text-red-300">
              <ExclamationTriangleIcon className="mt-0.5 h-4 w-4 shrink-0 text-red-500" />
              <div>Syntax Error: {syntaxError}</div>
            </div>
          )}

          <div className="relative flex min-h-[350px] flex-1 flex-col">
            <textarea
              value={rawInput}
              onChange={(e) => setRawInput(e.target.value)}
              className="w-full flex-1 resize-y rounded-xl border border-neutral-200 bg-neutral-50 p-4 font-mono text-xs leading-relaxed text-neutral-900 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:border-neutral-800 dark:bg-neutral-950 dark:text-neutral-100"
              spellCheck={false}
              rows={20}
            />
          </div>

          <div className="mt-3 flex items-center justify-between">
            <div className="text-xs text-neutral-500 dark:text-neutral-400">
              {isDirty ? (
                <span className="font-medium text-amber-500">Unsaved changes</span>
              ) : saveSuccess ? (
                <span className="flex items-center gap-1 font-medium text-green-500">
                  <CheckIcon className="h-4 w-4" /> Configuration saved successfully
                </span>
              ) : (
                <span>All changes saved</span>
              )}
            </div>

            <Button
              type="button"
              onClick={handleSave}
              disabled={!isDirty || !!syntaxError || isSaving}
              className="px-4 text-xs font-medium"
            >
              {isSaving ? "Saving..." : "Save Changes"}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}