import { useStreamingContext } from "@/contexts/StreamingContext";
import Downloading from "./Downloading";

export default function GlobalDownloads() {
  const { downloadProgress } = useStreamingContext();

  const activeDownloads = Array.from(downloadProgress.entries())
    .map(([modelName, info]) => ({
      modelName,
      ...info,
    }))
    .filter((item) => !item.event.done && item.event.total > 1024)
    // Sort descending by timestamp so oldest is at the end of the array (bottom of list)
    .sort((a, b) => b.timestamp - a.timestamp);

  if (activeDownloads.length === 0) {
    return null;
  }

  return (
    <div
      className="fixed bottom-4 right-4 z-50 flex flex-col gap-2 pointer-events-none max-w-sm w-full"
      data-role="global-downloads"
    >
      {activeDownloads.map(({ modelName, event }) => (
        <div
          key={modelName}
          className="pointer-events-auto bg-white/95 dark:bg-neutral-900/95 backdrop-blur-md border border-neutral-200 dark:border-neutral-800 shadow-xl rounded-2xl p-4 transition-all duration-200"
        >
          <div className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 mb-1 truncate">
            {modelName}
          </div>
          <Downloading completed={event.completed} total={event.total} />
        </div>
      ))}
    </div>
  );
}
