import { ArrowDownTrayIcon } from "@heroicons/react/20/solid";

const K = 1024;
const SIZES = ["B", "KB", "MB", "GB", "TB"];

function formatBytes(bytes: number, unit?: string): string {
  let i: number;
  if (unit) {
    i = SIZES.indexOf(unit);
  } else {
    i = bytes === 0 ? 0 : Math.floor(Math.log(bytes) / Math.log(K));
  }

  const decimals = SIZES[i] === "GB" || SIZES[i] === "TB" ? 1 : 0;
  return `${(bytes / Math.pow(K, i)).toFixed(decimals)} ${SIZES[i]}`;
}

export default function Downloading({
  completed,
  total,
}: {
  completed: number;
  total: number;
}) {
  const percentage = total > 0 ? (completed / total) * 100 : 0;
  const unitIndex = total > 0 ? Math.floor(Math.log(total) / Math.log(K)) : 0;
  const unit = SIZES[unitIndex];

  return (
    <div className="flex flex-col gap-2 w-full">
      <div className="flex items-center justify-between gap-2 text-xs font-medium text-neutral-700 dark:text-neutral-300">
        <div className="flex items-center gap-1.5">
          <ArrowDownTrayIcon className="h-4 w-4 shrink-0 text-neutral-500 dark:text-neutral-400" />
          <span>Downloading model</span>
        </div>
        <span className="text-neutral-500 font-mono text-[11px] shrink-0">
          {`${formatBytes(completed, unit)} / ${formatBytes(total, unit)} (${Math.floor(percentage)}%)`}
        </span>
      </div>
      <div className="relative h-1.5 w-full bg-neutral-200 dark:bg-neutral-700 rounded-full overflow-hidden">
        <div
          className="absolute left-0 top-0 h-full bg-neutral-900 dark:bg-neutral-100 rounded-full transition-all duration-150"
          style={{
            width: `${percentage}%`,
          }}
        />
      </div>
    </div>
  );
}

