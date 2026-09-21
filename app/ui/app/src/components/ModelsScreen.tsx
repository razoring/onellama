import { useEffect, useState, useCallback } from "react";
import { pullModel, getModels, deleteLocalModel } from "@/api";
import { Model } from "@/gotypes";
import { 
  GlobeAltIcon, 
  CpuChipIcon, 
  TrashIcon, 
  ArrowDownTrayIcon, 
  ArrowPathIcon,
  MagnifyingGlassIcon,
  SunIcon,
  MoonIcon
} from "@heroicons/react/24/outline";

export function ModelsScreen() {
  const [activeTab, setActiveTab] = useState<"discovery" | "installed">("discovery");
  const [theme, setTheme] = useState<"light" | "dark">("dark");
  const [localModels, setLocalModels] = useState<Model[]>([]);
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
      setLocalModels(models);
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
    } catch (err: any) {
      console.error(err);
      setPullStatus(`Error: ${err?.message || "Failed to pull model"}`);
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

  const getModelName = (m: Model): string => {
    return (m as any).name || m.model || "";
  };

  const getModelSize = (m: Model): number | undefined => {
    return (m as any).size;
  };

  const getModelDetails = (m: Model): any => {
    return (m as any).details;
  };

  const filteredLocalModels = localModels.filter((m) =>
    getModelName(m).toLowerCase().includes(searchFilter.toLowerCase())
  );

  const webviewSrc = `/api/v1/models/webview?path=search&theme=${theme}`;


  return (
    <div className="flex flex-col h-full bg-neutral-50 dark:bg-neutral-950 text-neutral-900 dark:text-neutral-100">
      {/* Top Bar with Custom Switcher & Actions */}
      <header className="flex items-center justify-between px-6 py-3 border-b border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 shadow-xs z-10 shrink-0">
        <div className="flex items-center gap-4">
          <h1 className="text-lg font-bold tracking-tight text-neutral-900 dark:text-white">Models</h1>

          {/* Custom Toggle Button for Discovery vs Installed Models */}
          <div className="inline-flex items-center rounded-lg bg-neutral-100 dark:bg-neutral-800 p-1 border border-neutral-200/80 dark:border-neutral-700/60">
            <button
              onClick={() => setActiveTab("discovery")}
              className={`flex items-center gap-2 px-3.5 py-1.5 rounded-md text-xs font-semibold transition-all ${
                activeTab === "discovery"
                  ? "bg-white dark:bg-neutral-900 text-neutral-900 dark:text-white shadow-xs"
                  : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-200"
              }`}
            >
              <GlobeAltIcon className="w-4 h-4 text-blue-500" />
              <span>Discovery</span>
            </button>
            <button
              onClick={() => setActiveTab("installed")}
              className={`flex items-center gap-2 px-3.5 py-1.5 rounded-md text-xs font-semibold transition-all ${
                activeTab === "installed"
                  ? "bg-white dark:bg-neutral-900 text-neutral-900 dark:text-white shadow-xs"
                  : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-200"
              }`}
            >
              <CpuChipIcon className="w-4 h-4 text-emerald-500" />
              <span>Installed Models</span>
              <span className="ml-0.5 rounded-full bg-neutral-200 dark:bg-neutral-700 px-1.5 py-0.5 text-[10px] font-bold text-neutral-700 dark:text-neutral-300">
                {localModels.length}
              </span>
            </button>
          </div>
        </div>

        {/* Theme matching control */}
        <div className="flex items-center gap-2">
          {activeTab === "discovery" && (
            <button
              onClick={() => setTheme(theme === "light" ? "dark" : "light")}
              title={`Switch to ${theme === "light" ? "Dark" : "Light"} mode for WebView`}
              className="flex items-center gap-1.5 px-2.5 py-1.5 text-xs font-medium rounded-md border border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
            >
              {theme === "light" ? (
                <>
                  <SunIcon className="w-4 h-4 text-amber-500" />
                  <span>Light Mode</span>
                </>
              ) : (
                <>
                  <MoonIcon className="w-4 h-4 text-indigo-400" />
                  <span>Dark Mode</span>
                </>
              )}
            </button>
          )}

          {activeTab === "installed" && (
            <button
              onClick={fetchModels}
              disabled={isLoadingModels}
              className="p-1.5 text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300 rounded-md hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
              title="Refresh models list"
            >
              <ArrowPathIcon className={`w-4 h-4 ${isLoadingModels ? "animate-spin" : ""}`} />
            </button>
          )}
        </div>
      </header>

      {/* Main View Area */}
      <div className="flex-1 overflow-hidden relative">
        {activeTab === "discovery" ? (
          <iframe
            key={`webview-${theme}`}
            src={webviewSrc}
            className="w-full h-full border-0 bg-white dark:bg-neutral-900"
            title="Ollama Model Discovery"
          />
        ) : (
          <div className="h-full overflow-y-auto p-6 max-w-6xl mx-auto w-full">
            {/* Pull Model Card / Bar */}
            <div className="mb-6 rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-5 shadow-xs">
              <h2 className="text-sm font-semibold text-neutral-900 dark:text-white mb-1">
                Install a Model
              </h2>
              <p className="text-xs text-neutral-500 dark:text-neutral-400 mb-3">
                Enter any Ollama model name (e.g., <code className="bg-neutral-100 dark:bg-neutral-800 px-1 py-0.5 rounded text-neutral-700 dark:text-neutral-300">llama3.2</code>, <code className="bg-neutral-100 dark:bg-neutral-800 px-1 py-0.5 rounded text-neutral-700 dark:text-neutral-300">gemma:2b</code>, <code className="bg-neutral-100 dark:bg-neutral-800 px-1 py-0.5 rounded text-neutral-700 dark:text-neutral-300">deepseek-r1:8b</code>) to pull it directly.
              </p>
              <div className="flex items-center gap-3">
                <input
                  type="text"
                  placeholder="model:tag"
                  value={pullInput}
                  onChange={(e) => setPullInput(e.target.value)}
                  onKeyDown={(e) => e.key === "Enter" && handlePullModel(pullInput)}
                  disabled={isPulling}
                  className="flex-1 rounded-lg border border-neutral-300 dark:border-neutral-700 bg-neutral-50 dark:bg-neutral-800 px-3.5 py-2 text-sm text-neutral-900 dark:text-white placeholder:text-neutral-400 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50"
                />
                <button
                  onClick={() => handlePullModel(pullInput)}
                  disabled={!pullInput.trim() || isPulling}
                  className="flex items-center gap-2 rounded-lg bg-blue-600 hover:bg-blue-500 text-white px-4 py-2 text-sm font-medium shadow-xs transition-colors disabled:opacity-50"
                >
                  <ArrowDownTrayIcon className="w-4 h-4" />
                  <span>Pull Model</span>
                </button>
              </div>

              {/* Progress indicator */}
              {pullStatus && (
                <div className="mt-4">
                  <div className="flex justify-between text-xs text-neutral-600 dark:text-neutral-400 mb-1">
                    <span>{pullStatus}</span>
                    {pullPercent !== null && <span>{pullPercent}%</span>}
                  </div>
                  {pullPercent !== null && (
                    <div className="w-full bg-neutral-200 dark:bg-neutral-700 rounded-full h-2 overflow-hidden">
                      <div
                        className="bg-blue-600 h-2 rounded-full transition-all duration-300"
                        style={{ width: `${pullPercent}%` }}
                      />
                    </div>
                  )}
                </div>
              )}
            </div>

            {/* Filter Bar */}
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-base font-semibold text-neutral-900 dark:text-white">
                Installed Models ({localModels.length})
              </h2>
              <div className="relative w-64">
                <MagnifyingGlassIcon className="w-4 h-4 text-neutral-400 absolute left-3 top-2.5" />
                <input
                  type="text"
                  placeholder="Filter local models..."
                  value={searchFilter}
                  onChange={(e) => setSearchFilter(e.target.value)}
                  className="w-full pl-9 pr-3 py-1.5 text-xs rounded-lg border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-white placeholder:text-neutral-400 focus:outline-none focus:ring-1 focus:ring-blue-500"
                />
              </div>
            </div>

            {/* Models Grid */}
            {isLoadingModels ? (
              <div className="py-12 text-center text-sm text-neutral-500">
                Loading installed models...
              </div>
            ) : filteredLocalModels.length === 0 ? (
              <div className="py-12 text-center rounded-xl border border-dashed border-neutral-300 dark:border-neutral-800 p-8">
                <CpuChipIcon className="w-10 h-10 text-neutral-400 mx-auto mb-3" />
                <h3 className="text-sm font-semibold text-neutral-900 dark:text-white">No models found</h3>
                <p className="text-xs text-neutral-500 mt-1 max-w-sm mx-auto">
                  {searchFilter ? "No models match your search query." : "You have no models installed locally. Use the Discovery tab to explore available models."}
                </p>
              </div>
            ) : (
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {filteredLocalModels.map((m) => {
                  const name = getModelName(m);
                  const size = getModelSize(m);
                  const details = getModelDetails(m);

                  return (
                    <div
                      key={name || m.digest}
                      className="flex flex-col justify-between rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-4 shadow-xs hover:border-neutral-300 dark:hover:border-neutral-700 transition-all"
                    >
                      <div>
                        <div className="flex items-start justify-between gap-2 mb-2">
                          <h3 className="text-sm font-semibold text-neutral-900 dark:text-white truncate" title={name}>
                            {name}
                          </h3>
                          <span className="shrink-0 text-[10px] font-medium px-2 py-0.5 rounded-full bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800">
                            Ready
                          </span>
                        </div>
                        <div className="space-y-1 text-xs text-neutral-500 dark:text-neutral-400">
                          {size ? <div><strong className="text-neutral-700 dark:text-neutral-300 font-medium">Size:</strong> {formatBytes(size)}</div> : null}
                          {details?.family ? <div><strong className="text-neutral-700 dark:text-neutral-300 font-medium">Family:</strong> {details.family}</div> : null}
                          {details?.parameter_size ? <div><strong className="text-neutral-700 dark:text-neutral-300 font-medium">Params:</strong> {details.parameter_size}</div> : null}
                          {details?.quantization_level ? <div><strong className="text-neutral-700 dark:text-neutral-300 font-medium">Quant:</strong> {details.quantization_level}</div> : null}
                        </div>
                      </div>

                      <div className="mt-4 pt-3 border-t border-neutral-100 dark:border-neutral-800 flex items-center justify-between">
                        <span className="text-[11px] text-neutral-400 font-mono truncate max-w-[150px]">
                          {m.digest ? `${m.digest.substring(0, 12)}...` : ""}
                        </span>
                        <button
                          onClick={() => handleDeleteModel(name)}
                          disabled={deletingModel === name}
                          className="flex items-center gap-1 text-xs font-medium text-red-600 dark:text-red-400 hover:text-red-700 dark:hover:text-red-300 hover:bg-red-50 dark:hover:bg-red-950/40 px-2 py-1 rounded transition-colors disabled:opacity-50"
                        >
                          <TrashIcon className="w-3.5 h-3.5" />
                          <span>{deletingModel === name ? "Deleting..." : "Delete"}</span>
                        </button>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
