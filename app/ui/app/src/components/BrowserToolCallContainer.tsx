import React, { useState } from "react";
import { openBrowserWindow, resizeBrowserWindow } from "@/api";

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
  onFocusWindow,
  onTriggerNativeHandoff,
  onReturnControl,
}) => {
  const [isControlled, setIsControlled] = useState(false);
  const [isOpen, setIsOpen] = useState(false);
  const [isHovered, setIsHovered] = useState(false);

  const handleOpen = () => {
    setIsOpen(true);
    if (onFocusWindow) {
      onFocusWindow();
    } else {
      openBrowserWindow();
    }
  };

  const handleTakeControl = () => {
    setIsControlled(true);
    if (onTriggerNativeHandoff) {
      onTriggerNativeHandoff();
    } else {
      resizeBrowserWindow("full");
    }
  };

  const handleReturnControl = () => {
    setIsControlled(false);
    if (onReturnControl) {
      onReturnControl();
    } else {
      resizeBrowserWindow("pip");
    }
  };

  const handleMinimize = () => {
    resizeBrowserWindow("pip");
  };

  const handleMaximize = () => {
    resizeBrowserWindow("full");
  };

  return (
    <div
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
      className="group relative my-2 rounded-xl border border-neutral-800 bg-neutral-950/80 overflow-hidden text-xs font-mono shadow-lg transition-all"
    >
      {/* Custom Title Bar Controls */}
      <div className="w-full flex items-center justify-between px-3 py-2 bg-neutral-900/90 transition-colors border-b border-neutral-800/60">
        <div className="flex items-center gap-2">
          <span className="w-2.5 h-2.5 rounded-full bg-emerald-500 animate-pulse" />
          <button
            type="button"
            onClick={handleOpen}
            className="text-neutral-300 hover:text-white font-medium cursor-pointer transition-colors focus:outline-none"
          >
            {isOpen ? "Frameless QEMU Browser VM" : "Open browser"}
          </button>
        </div>

        {/* Custom Window Control Icons */}
        <div className="flex items-center gap-1.5">
          <button
            type="button"
            title="Minimize to PiP"
            onClick={handleMinimize}
            className="w-5 h-5 flex items-center justify-center rounded hover:bg-neutral-800 text-neutral-400 hover:text-white transition-colors"
          >
            <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M20 12H4" />
            </svg>
          </button>
          <button
            type="button"
            title="Maximize to 1080p"
            onClick={handleMaximize}
            className="w-5 h-5 flex items-center justify-center rounded hover:bg-neutral-800 text-neutral-400 hover:text-white transition-colors"
          >
            <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <rect x="4" y="4" width="16" height="16" rx="2" strokeWidth={2} />
            </svg>
          </button>
          <button
            type="button"
            title="Close / Shrink"
            onClick={handleReturnControl}
            className="w-5 h-5 flex items-center justify-center rounded hover:bg-red-500/20 text-neutral-400 hover:text-red-400 transition-colors"
          >
            <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
      </div>

      {/* Hover-activated Control Panel & Bottom Center Return Button */}
      <div className="p-3 flex flex-col items-center justify-center min-h-[50px] relative">
        {!isControlled ? (
          <div
            className={`transition-all duration-200 ${
              isHovered ? "opacity-100 scale-100" : "opacity-75 scale-95"
            }`}
          >
            <button
              type="button"
              onClick={handleTakeControl}
              className="px-4 py-1.5 rounded-full bg-blue-600 hover:bg-blue-500 text-white font-semibold cursor-pointer shadow-md transition-all flex items-center gap-2"
            >
              <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 15l-2 5L9 9l11 4-5 2zm0 0l5 5" />
              </svg>
              Take Control
            </button>
          </div>
        ) : (
          <div className="w-full flex justify-center my-1">
            <button
              type="button"
              onClick={handleReturnControl}
              className="px-4 py-1.5 rounded-full bg-neutral-800 hover:bg-neutral-700 text-neutral-200 hover:text-white font-semibold cursor-pointer shadow-md border border-neutral-700 transition-all flex items-center gap-2"
            >
              <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 19l-7-7 7-7m8 14l-7-7 7-7" />
              </svg>
              Return to Agent
            </button>
          </div>
        )}
      </div>
    </div>
  );
};
