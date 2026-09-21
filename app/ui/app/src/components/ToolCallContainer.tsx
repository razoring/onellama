import React, { useState } from "react";
import { ChevronDownIcon, ChevronRightIcon, CommandLineIcon } from "@heroicons/react/24/outline";

export interface ToolCallItem {
  id?: string;
  name: string;
  arguments?: Record<string, any> | string;
  output?: string;
  status?: "pending" | "running" | "completed" | "error";
}

interface ToolCallContainerProps {
  toolCall: ToolCallItem;
}

export const ToolCallContainer: React.FC<ToolCallContainerProps> = ({ toolCall }) => {
  const [isOpen, setIsOpen] = useState<boolean>(true);

  const formatArgs = (args: Record<string, any> | string | undefined): string => {
    if (!args) return "";
    if (typeof args === "string") {
      try {
        return JSON.stringify(JSON.parse(args), null, 2);
      } catch {
        return args;
      }
    }
    return JSON.stringify(args, null, 2);
  };

  const getActionTitle = (): string => {
    if (toolCall.name === "terminal_run" && typeof toolCall.arguments === "object" && toolCall.arguments?.command) {
      return `terminal: ${toolCall.arguments.command}`;
    }
    if (toolCall.name === "fs_read" && typeof toolCall.arguments === "object" && toolCall.arguments?.path) {
      return `read: ${toolCall.arguments.path}`;
    }
    if (toolCall.name === "fs_write" && typeof toolCall.arguments === "object" && toolCall.arguments?.path) {
      return `write: ${toolCall.arguments.path}`;
    }
    return toolCall.name;
  };

  return (
    <div className="my-2 rounded-xl border border-neutral-800 bg-neutral-950/60 overflow-hidden text-xs">
      <button
        type="button"
        onClick={() => setIsOpen(!isOpen)}
        className="w-full flex items-center justify-between px-3 py-2 bg-neutral-900/80 hover:bg-neutral-900 transition-colors text-left"
      >
        <div className="flex items-center gap-2 font-mono text-neutral-300 truncate">
          <CommandLineIcon className="w-4 h-4 text-amber-400 shrink-0" />
          <span className="font-semibold text-neutral-200">{getActionTitle()}</span>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <span className="text-[10px] px-1.5 py-0.5 rounded bg-neutral-800 text-neutral-400 font-mono">
            {toolCall.name}
          </span>
          {isOpen ? (
            <ChevronDownIcon className="w-3.5 h-3.5 text-neutral-400" />
          ) : (
            <ChevronRightIcon className="w-3.5 h-3.5 text-neutral-400" />
          )}
        </div>
      </button>

      {isOpen && (
        <div className="p-3 space-y-2 border-t border-neutral-800 font-mono bg-black/40">
          {toolCall.arguments && (
            <div>
              <div className="text-[10px] text-neutral-500 uppercase tracking-wider mb-1">Inputs</div>
              <pre className="text-xs text-neutral-300 bg-neutral-900/90 p-2 rounded border border-neutral-800 overflow-x-auto whitespace-pre-wrap">
                {formatArgs(toolCall.arguments)}
              </pre>
            </div>
          )}

          {toolCall.output && (
            <div>
              <div className="text-[10px] text-neutral-500 uppercase tracking-wider mb-1">Output</div>
              <pre className="text-xs text-neutral-200 bg-neutral-900/90 p-2 rounded border border-neutral-800 overflow-x-auto max-h-60 whitespace-pre-wrap">
                {toolCall.output}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
};
