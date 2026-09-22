import { useState, useCallback, useEffect } from "react";
import { API_BASE } from "@/lib/config";
import { useSettings } from "@/hooks/useSettings";

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
  const effectiveTheme = useEffectiveTheme();
  const webviewSrc = `${API_BASE}/api/v1/models/webview?path=search&theme=${effectiveTheme}`;

  return (
    <div className="relative flex-1 min-h-0 overflow-hidden bg-white dark:bg-neutral-900">
      <iframe
        key={`webview-${effectiveTheme}`}
        src={webviewSrc}
        className="h-full w-full border-0"
        title="Ollama Model Discovery"
      />
    </div>
  );
}