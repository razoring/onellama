import React, { useState } from "react";

export interface PendingPrompt {
  id: string;
  type: "permission" | "question";
  tool_name?: string;
  title: string;
  message: string;
  options?: string[];
}

interface InteractivePromptOverlayProps {
  prompt: PendingPrompt;
  onRespond: (id: string, response: string) => void;
}

export const InteractivePromptOverlay: React.FC<InteractivePromptOverlayProps> = ({
  prompt,
  onRespond,
}) => {
  const [selectedOption, setSelectedOption] = useState<string>("");
  const [otherText, setOtherText] = useState<string>("");

  const defaultPermissionOptions = ["Allow", "Allow for this chat", "Deny"];
  const displayOptions =
    prompt.options && prompt.options.length > 0
      ? prompt.options
      : prompt.type === "permission"
      ? defaultPermissionOptions
      : [];

  const handleSelect = (option: string) => {
    setSelectedOption(option);
    if (option !== "Other") {
      onRespond(prompt.id, option);
    }
  };

  const handleOtherSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (otherText.trim()) {
      onRespond(prompt.id, otherText.trim());
    }
  };

  return (
    <div className="absolute inset-0 z-50 flex flex-col justify-between bg-neutral-900/95 backdrop-blur-md rounded-2xl p-4 border border-neutral-700 shadow-2xl transition-all duration-200">
      <div className="flex-1 overflow-y-auto space-y-2 pr-1">
        <div className="flex items-center justify-between">
          <span className="text-xs font-semibold uppercase tracking-wider text-amber-400">
            {prompt.type === "permission" ? "⚠️ Permission Required" : "❓ Question from Model"}
          </span>
          {prompt.tool_name && (
            <span className="text-xs px-2 py-0.5 rounded bg-neutral-800 text-neutral-400 font-mono">
              {prompt.tool_name}
            </span>
          )}
        </div>

        <h4 className="text-sm font-semibold text-neutral-100">{prompt.title}</h4>
        <p className="text-xs text-neutral-300 font-mono bg-neutral-950/80 p-2.5 rounded-lg border border-neutral-800 whitespace-pre-wrap leading-relaxed">
          {prompt.message}
        </p>
      </div>

      <div className="mt-3 space-y-2 pt-2 border-t border-neutral-800">
        <div className="flex flex-wrap gap-2">
          {displayOptions.map((opt) => (
            <button
              key={opt}
              type="button"
              onClick={() => handleSelect(opt)}
              className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
                selectedOption === opt
                  ? "bg-amber-500 text-neutral-950 font-semibold"
                  : opt.toLowerCase().includes("deny")
                  ? "bg-red-500/20 text-red-300 hover:bg-red-500/30 border border-red-500/30"
                  : "bg-neutral-800 text-neutral-200 hover:bg-neutral-700 border border-neutral-700"
              }`}
            >
              {opt}
            </button>
          ))}

          <button
            type="button"
            onClick={() => setSelectedOption("Other")}
            className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
              selectedOption === "Other"
                ? "bg-amber-500 text-neutral-950 font-semibold"
                : "bg-neutral-800 text-neutral-300 hover:bg-neutral-700 border border-neutral-700"
            }`}
          >
            Other
          </button>
        </div>

        {selectedOption === "Other" && (
          <form onSubmit={handleOtherSubmit} className="flex gap-2 pt-1">
            <input
              type="text"
              autoFocus
              value={otherText}
              onChange={(e) => setOtherText(e.target.value)}
              placeholder="Enter custom response or reason..."
              className="flex-1 bg-neutral-950 border border-neutral-700 rounded-lg px-3 py-1.5 text-xs text-neutral-100 placeholder-neutral-500 focus:outline-none focus:border-amber-500"
            />
            <button
              type="submit"
              disabled={!otherText.trim()}
              className="px-4 py-1.5 bg-amber-500 hover:bg-amber-400 disabled:opacity-50 text-neutral-950 text-xs font-semibold rounded-lg transition-colors"
            >
              Submit
            </button>
          </form>
        )}
      </div>
    </div>
  );
};
