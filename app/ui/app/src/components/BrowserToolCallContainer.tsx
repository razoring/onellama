import React from "react";
import { openBrowserWindow } from "@/api";

interface BrowserToolCallContainerProps {
  toolCall?: {
    id?: string;
    name: string;
    arguments?: Record<string, any>;
    status?: string;
  };
  onFocusWindow?: () => void;
  onTriggerNativeHandoff?: () => void;
  onReturnControl?: () => void;
}

export const BrowserToolCallContainer: React.FC<BrowserToolCallContainerProps> = ({
  toolCall,
  onFocusWindow,
}) => {
  const handleOpen = () => {
    if (onFocusWindow) {
      onFocusWindow();
    } else {
      openBrowserWindow();
    }
  };

  return (
    <div className="relative my-2 rounded-xl border border-neutral-800 bg-neutral-950/70 overflow-hidden text-xs font-mono shadow-sm transition-all">
      <div className="w-full flex items-center justify-between px-3 py-2 bg-neutral-900/60 border-b border-neutral-800/40">
        <div className="flex items-center gap-2">
          <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
          <span className="text-neutral-300 font-medium">Browser VM (QEMU Isolated)</span>
        </div>
        <button
          type="button"
          onClick={handleOpen}
          className="px-2.5 py-1 rounded bg-neutral-800 hover:bg-neutral-700 text-neutral-300 hover:text-white cursor-pointer transition-colors focus:outline-none flex items-center gap-1.5"
        >
          <svg className="w-3 h-3 text-neutral-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
          </svg>
          Show PiP
        </button>
      </div>
      {toolCall?.arguments?.url && (
        <div className="px-3 py-2 text-neutral-400 truncate border-t border-neutral-800/20 bg-neutral-950/40">
          <span className="text-neutral-500 mr-1.5">Target:</span>
          <span className="text-neutral-300">{toolCall.arguments.url}</span>
        </div>
      )}
    </div>
  );
};
