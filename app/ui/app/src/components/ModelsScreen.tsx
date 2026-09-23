import { useState, useCallback, useEffect, useRef } from "react";
import { API_BASE } from "@/lib/config";
import { useSettings } from "@/hooks/useSettings";
import { useStreamingContext } from "@/contexts/StreamingContext";
import { DownloadEvent } from "@/gotypes";

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
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const { setDownloadProgress } = useStreamingContext();

  useEffect(() => {
    const handleMessage = (event: MessageEvent) => {
      if (event.data?.type === 'pullModel') {
        const { tag } = event.data;
        const modelName = tag;

        fetch(`${API_BASE}/api/pull`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ name: tag, stream: true })
        }).then(async (res) => {
          if (!res.body) return;
          const reader = res.body.getReader();
          const decoder = new TextDecoder();
          let buffer = '';

          while (true) {
            const { done, value } = await reader.read();
            if (done) break;
            buffer += decoder.decode(value, { stream: true });
            const lines = buffer.split('\n');
            buffer = lines.pop() || '';

            for (const line of lines) {
              if (!line.trim()) continue;
              try {
                const parsed = JSON.parse(line);
                const downloadEvt = new DownloadEvent({
                  completed: parsed.completed ?? 0,
                  total: parsed.total ?? 0,
                  done: parsed.status === 'success' || (parsed.completed > 0 && parsed.completed === parsed.total),
                });

                setDownloadProgress((prev) => {
                  const newMap = new Map(prev);
                  const existing = newMap.get(modelName);
                  if (downloadEvt.done) {
                    newMap.delete(modelName);
                  } else {
                    newMap.set(modelName, {
                      event: downloadEvt,
                      timestamp: existing?.timestamp ?? Date.now(),
                    });
                  }
                  return newMap;
                });
              } catch (e) {
                console.error("Error parsing pull NDJSON chunk:", e);
              }
            }
          }
        }).finally(() => {
          setDownloadProgress((prev) => {
            const newMap = new Map(prev);
            newMap.delete(modelName);
            return newMap;
          });

          localStorage.removeItem('onellama_pulling_' + tag);
          const norm = tag.indexOf(':latest') !== -1 ? tag.split(':latest')[0] : (tag.indexOf(':') === -1 ? tag + ':latest' : null);
          if (norm) localStorage.removeItem('onellama_pulling_' + norm);
          
          if (iframeRef.current && iframeRef.current.contentWindow) {
            iframeRef.current.contentWindow.postMessage({ type: 'pullComplete', tag: tag }, '*');
          }
        }).catch(err => {
          console.error("Failed to pull model:", err);
          setDownloadProgress((prev) => {
            const newMap = new Map(prev);
            newMap.delete(modelName);
            return newMap;
          });
        });
      }
    };
    window.addEventListener('message', handleMessage);
    return () => window.removeEventListener('message', handleMessage);
  }, [setDownloadProgress]);

  return (
    <div className="relative flex-1 min-h-0 overflow-hidden bg-white dark:bg-neutral-900">
      <iframe
        ref={iframeRef}
        key={`webview-${effectiveTheme}`}
        src={webviewSrc}
        className="h-full w-full border-0"
        title="Ollama Model Discovery"
      />
    </div>
  );
}