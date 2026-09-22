import { useEffect, useState, useCallback } from "react";
import { pullModel, getModels, deleteLocalModel } from "@/api";
import { API_BASE } from "@/lib/config";
import { useSettings } from "@/hooks/useSettings";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input, InputGroup } from "@/components/ui/input";
import {
  GlobeAltIcon,
  CpuChipIcon,
  TrashIcon,
  ArrowDownTrayIcon,
  ArrowPathIcon,
  MagnifyingGlassIcon,
} from "@heroicons/react/24/outline";

interface LocalModel {
  name?: string;
  model: string;
  digest?: string;
  size?: number;
  details?: {
    parameter_size?: string;
    quantization_level?: string;
    family?: string;
  };
}

function useEffectiveTheme() {
  const { settings } = useSettings();
  const theme = settings.theme;

  const resolve = useCallback(() => {
    if (theme === "automatic") {
      return window.matchMedia("(prefers-color-scheme: dark)").matches
        ? "dark"
        : "light";
    }
    return theme === "dark" ? "dark" : "light";
  }, [theme]);

  const [effective, setEffective] = useState<"light" | "dark">(resolve);

  useEffect(() => {
    setEffective(resolve());
    if (theme !== "automatic") return;
    const mql = window.matchMedia("(prefers-color-scheme: dark)");
    const onChange = () => setEffective(resolve());
    mql.addEventListener("change", onChange);
    return () => mql.removeEventListener("change", onChange);
  }, [theme, resolve]);

  return effective;
}

export function ModelsScreen() {
  const [activeTab, setActiveTab] = useState<"discovery" | "installed">("discovery");
  const effectiveTheme = useEffectiveTheme();
  const [localModels, setLocalModels] = useState<LocalModel[]>([]);
  const [isLoadingModels, setIsLoadingModels] = useState(false);
  const [searchFilter, setSearchFilter] = useState("");
  const [pullInput, setPullInput] = useState("");
  const [pullStatus, setPullStatus] = useState<string | null>(null);
  const [pullPercent, setPullPercent] = useState<number | null>(null);
  const [isPulling, setIsPulling] = useState(false);
  const [deletingModel, setDeletingModel] = useState<string | null>(null);

  const fetchModels = useCallback(async () => {
    setIsLoadingModels(true);
    try {
      const models = await getModels();
      setLocalModels(models as LocalModel[]);
    } catch (e) {
      console.error("Failed to load local models:", e);
    } finally {
      setIsLoadingModels(false);
    }
  }, []);

  useEffect(() => {
    fetchModels();
  }, [fetchModels]);

  const handlePullModel = async (modelName: string) => {
    if (!modelName.trim() || isPulling) return;
    const name = modelName.trim();
    setIsPulling(true);
    setPullStatus("Initializing pull...");
    setPullPercent(null);

    try {
      for await (const progress of pullModel(name)) {
        if (progress.total && progress.completed) {
          const pct = Math.round((progress.completed / progress.total) * 100);
          setPullPercent(pct);
          setPullStatus(`Pulling: ${pct}%`);
        } else if (progress.status) {
          setPullStatus(progress.status);
        }
      }
      setPullStatus("Completed!");
      setPullPercent(100);
      setPullInput("");
      fetchModels();
      setTimeout(() => {
        setPullStatus(null);
        setPullPercent(null);
        setIsPulling(false);
      }, 3000);
    } catch (err: unknown) {
      console.error(err);
      setPullStatus(`Error: ${err instanceof Error ? err.message : "Failed to pull model"}`);
      setIsPulling(false);
    }
  };

  const handleDeleteModel = async (name: string) => {
    setDeletingModel(name);
    try {
      await deleteLocalModel(name);
      await fetchModels();
    } catch (err) {
      console.error("Failed to delete model", err);
    } finally {
      setDeletingModel(null);
    }
  };

  const formatBytes = (bytes?: number) => {
    if (!bytes) return "Unknown size";
    const k = 1024;
    const sizes = ["B", "KB", "MB", "GB", "TB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
  };

  const getModelName = (m: LocalModel): string => {
    return m.name || m.model || "";
  };

  const getModelSize = (m: LocalModel): number | undefined => {
    return m.size;
  };

  const getModelDetails = (m: LocalModel): LocalModel["details"] => {
    return m.details;
  };

  const filteredLocalModels = localModels.filter((m) =>
    getModelName(m).toLowerCase().includes(searchFilter.toLowerCase()),
  );

  const webviewSrc = `${API_BASE}/api/v1/models/webview?path=search&theme=${effectiveTheme}`;

  return (
    <div className="flex flex-col flex-1 min-h-0 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100">
      <div className="flex shrink-0 items-center justify-between border-b border-neutral-200 px-6 py-3 dark:border-neutral-800">
        <div className="inline-flex items-center rounded-lg bg-neutral-100 p-1 dark:bg-neutral-800">
          <button
            onClick={() => setActiveTab("discovery")}
            className={`flex items-center gap-2 rounded-md px-3.5 py-1.5 text-xs font-semibold transition-all ${
              activeTab === "discovery"
                ? "bg-white text-neutral-900 shadow-xs dark:bg-white/10 dark:text-neutral-100"
                : "text-neutral-600 hover:text-neutral-900 dark:text-neutral-400 dark:hover:text-neutral-200"
            }`}
          >
            <GlobeAltIcon className="h-4 w-4" />
            <span>Discovery</span>
          </button>
          <button
            onClick={() => setActiveTab("installed")}
            className={`flex items-center gap-2 rounded-md px-3.5 py-1.5 text-xs font-semibold transition-all ${
              activeTab === "installed"
                ? "bg-white text-neutral-900 shadow-xs dark:bg-white/10 dark:text-neutral-100"
                : "text-neutral-600 hover:text-neutral-900 dark:text-neutral-400 dark:hover:text-neutral-200"
            }`}
          >
            <CpuChipIcon className="h-4 w-4" />
            <span>Installed Models</span>
            <span className="ml-0.5 rounded-full bg-neutral-200 px-1.5 py-0.5 text-[10px] font-bold text-neutral-700 dark:bg-neutral-700 dark:text-neutral-300">
              {localModels.length}
            </span>
          </button>
        </div>

        {activeTab === "installed" && (
          <Button
            type="button"
            color="white"
            className="px-3"
            disabled={isLoadingModels}
            onClick={fetchModels}
          >
            <ArrowPathIcon
              data-slot="icon"
              className={isLoadingModels ? "animate-spin" : ""}
            />
            Refresh
          </Button>
        )}
      </div>

      <div className="relative flex-1 overflow-hidden">
        {activeTab === "discovery" ? (
          <iframe
            key={`webview-${effectiveTheme}`}
            src={webviewSrc}
            className="h-full w-full border-0"
            title="Ollama Model Discovery"
          />
        ) : (
          <div className="h-full w-full overflow-y-auto">
            <div className="mx-auto max-w-3xl px-6 py-6">
              {/* Install a Model */}
              <div className="mb-6 border-b border-neutral-200 pb-6 dark:border-neutral-800">
                <h2 className="text-sm font-medium text-neutral-900 dark:text-white">
                  Install a Model
                </h2>
                <p className="mb-3 mt-0.5 text-xs text-neutral-500 dark:text-neutral-400">
                  Enter any Ollama model name (e.g.,{" "}
                  <code className="rounded bg-neutral-100 px-1 py-0.5 text-neutral-700 dark:bg-neutral-800 dark:text-neutral-300">
                    llama3.2
                  </code>
                  ,{" "}
                  <code className="rounded bg-neutral-100 px-1 py-0.5 text-neutral-700 dark:bg-neutral-800 dark:text-neutral-300">
                    gemma:2b
                  </code>
                  ) to pull it directly.
                </p>
                <div className="flex items-center gap-3">
                  <Input
                    type="text"
                    placeholder="model:tag"
                    value={pullInput}
                    onChange={(e) => setPullInput(e.target.value)}
                    onKeyDown={(e) => e.key === "Enter" && handlePullModel(pullInput)}
                    disabled={isPulling}
                  />
                  <Button
                    type="button"
                    className="shrink-0 px-4"
                    disabled={!pullInput.trim() || isPulling}
                    onClick={() => handlePullModel(pullInput)}
                  >
                    <ArrowDownTrayIcon data-slot="icon" />
                    Pull Model
                  </Button>
                </div>

                {pullStatus && (
                  <div className="mt-4">
                    <div className="mb-1 flex justify-between text-xs text-neutral-600 dark:text-neutral-400">
                      <span>{pullStatus}</span>
                      {pullPercent !== null && <span>{pullPercent}%</span>}
                    </div>
                    {pullPercent !== null && (
                      <div className="h-1.5 w-full overflow-hidden rounded-full bg-neutral-200 dark:bg-neutral-700">
                        <div
                          className="h-1.5 rounded-full bg-neutral-900 transition-all duration-300 dark:bg-neutral-200"
                          style={{ width: `${pullPercent}%` }}
                        />
                      </div>
                    )}
                  </div>
                )}
              </div>

              {/* Installed Models */}
              <div className="mb-4 flex items-center justify-between">
                <h2 className="text-sm font-medium text-neutral-900 dark:text-white">
                  Installed Models ({localModels.length})
                </h2>
              </div>

              <div className="relative mb-4">
                <InputGroup>
                  <MagnifyingGlassIcon data-slot="icon" />
                  <Input
                    type="search"
                    placeholder="Filter local models..."
                    value={searchFilter}
                    onChange={(e) => setSearchFilter(e.target.value)}
                  />
                </InputGroup>
              </div>

              {isLoadingModels ? (
                <div className="py-12 text-center text-sm text-neutral-500">
                  Loading installed models...
                </div>
              ) : filteredLocalModels.length === 0 ? (
                <div className="rounded-xl border border-dashed border-neutral-300 p-8 py-12 text-center dark:border-neutral-800">
                  <CpuChipIcon className="mx-auto mb-3 h-10 w-10 text-neutral-400" />
                  <h3 className="text-sm font-semibold text-neutral-900 dark:text-white">
                    No models found
                  </h3>
                  <p className="mx-auto mt-1 max-w-sm text-xs text-neutral-500">
                    {searchFilter
                      ? "No models match your search query."
                      : "You have no models installed locally. Use the Discovery tab to explore available models."}
                  </p>
                </div>
              ) : (
                <ul className="divide-y divide-neutral-100 dark:divide-neutral-800">
                  {filteredLocalModels.map((m) => {
                    const name = getModelName(m);
                    const size = getModelSize(m);
                    const details = getModelDetails(m);
                    const meta = [
                      details?.parameter_size,
                      details?.quantization_level,
                      details?.family,
                      size ? formatBytes(size) : null,
                    ]
                      .filter(Boolean)
                      .join(" · ");

                    return (
                      <li
                        key={name || m.digest}
                        className="flex items-center justify-between gap-4 py-3"
                      >
                        <div className="min-w-0">
                          <div className="flex items-center gap-2">
                            <span
                              className="truncate text-sm font-medium text-neutral-900 dark:text-white"
                              title={name}
                            >
                              {name}
                            </span>
                            <Badge color="green">Ready</Badge>
                          </div>
                          {meta && (
                            <p className="mt-0.5 truncate text-xs text-neutral-500 dark:text-neutral-400">
                              {meta}
                            </p>
                          )}
                          {m.digest && (
                            <p className="mt-0.5 truncate font-mono text-[11px] text-neutral-400">
                              {m.digest.substring(0, 12)}…
                            </p>
                          )}
                        </div>
                        <button
                          type="button"
                          onClick={() => handleDeleteModel(name)}
                          disabled={deletingModel === name}
                          className="flex shrink-0 items-center gap-1 rounded px-2 py-1 text-xs font-medium text-red-600 transition-colors hover:bg-red-50 hover:text-red-700 disabled:opacity-50 dark:text-red-400 dark:hover:bg-red-950/40 dark:hover:text-red-300"
                        >
                          <TrashIcon className="h-3.5 w-3.5" />
                          <span>
                            {deletingModel === name ? "Deleting…" : "Delete"}
                          </span>
                        </button>
                      </li>
                    );
                  })}
                </ul>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}