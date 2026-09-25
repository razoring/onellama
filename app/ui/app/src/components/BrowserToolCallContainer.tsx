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

  const getActionTitle = (): string => {
    const url = toolCall?.arguments?.url;
    const action = toolCall?.arguments?.action;
    if (url) {
      if (action && action !== "navigate") {
        return `${action}: ${url}`;
      }
      return `navigate: ${url}`;
    }
    if (action) {
      return `${action}`;
    }
    return toolCall?.name || "browser_vm";
  };

  return (
    <div className="flex flex-col w-full text-neutral-600 dark:text-neutral-400 relative select-text my-1">
      <div
        onClick={handleOpen}
        className="flex items-center cursor-pointer group/browser self-start relative"
      >
        {/* Globe / Browser icon */}
        <svg
          className="w-4 h-4 absolute left-0 top-1/2 -translate-y-1/2 fill-current text-neutral-500 dark:text-neutral-400"
          viewBox="0 0 18 18"
          xmlns="http://www.w3.org/2000/svg"
        >
          <path d="M17.9297 8.96484C17.9297 13.9131 13.9131 17.9297 8.96484 17.9297C4.0166 17.9297 0 13.9131 0 8.96484C0 4.0166 4.0166 0 8.96484 0C13.9131 0 17.9297 4.0166 17.9297 8.96484ZM8.52608 0.544075C7.9195 0.648681 7.34943 0.980368 6.84016 1.49718C5.65243 1.83219 4.58058 2.44328 3.70023 3.25921C3.62549 3.21723 3.56076 3.16926 3.49805 3.12012C3.24316 2.91797 2.90918 2.8916 2.68066 3.11133C2.46094 3.33105 2.44336 3.68262 2.68945 3.91113C2.76044 3.97523 2.83584 4.03812 2.91591 4.09957C1.95192 5.28662 1.33642 6.76661 1.2196 8.38477H1.05469C0.738281 8.38477 0.474609 8.64844 0.474609 8.96484C0.474609 9.28125 0.738281 9.53613 1.05469 9.53613H1.21922C1.33511 11.1701 1.95962 12.6636 2.93835 13.8571C2.84984 13.9239 2.76702 13.9925 2.68945 14.0625C2.44336 14.2822 2.46094 14.6426 2.68066 14.8623C2.90918 15.082 3.24316 15.0557 3.49805 14.8535C3.56925 14.7972 3.64306 14.7424 3.72762 14.6944C4.60071 15.4974 5.6608 16.0989 6.83417 16.431C7.32996 16.9376 7.88339 17.2682 8.47128 17.3845C8.56963 17.5572 8.75351 17.6748 8.96484 17.6748C9.1767 17.6748 9.36492 17.5566 9.46571 17.3832C10.0509 17.2655 10.6018 16.9355 11.0955 16.431C12.2689 16.0989 13.329 15.4974 14.2021 14.6944C14.2866 14.7424 14.3604 14.7972 14.4316 14.8535C14.6865 15.0557 15.0205 15.082 15.249 14.8623C15.4688 14.6426 15.4863 14.2822 15.2402 14.0625C15.1627 13.9925 15.0798 13.9239 14.9913 13.8571C15.9701 12.6636 16.5946 11.1701 16.7105 9.53613H17.0508C17.3672 9.53613 17.6309 9.28125 17.6309 8.96484C17.6309 8.64844 17.3672 8.38477 17.0508 8.38477H16.7101C16.5933 6.76661 15.9778 5.28661 15.0138 4.09957C15.0939 4.03812 15.1692 3.97523 15.2402 3.91113C15.4863 3.68262 15.4688 3.33105 15.249 3.11133C15.0205 2.8916 14.6865 2.91797 14.4316 3.12012C14.3689 3.16926 14.3042 3.21723 14.2295 3.25921C13.3491 2.44328 12.2773 1.83219 11.0895 1.49718C10.582 0.982111 10.014 0.650921 9.40974 0.545145C9.30275 0.416705 9.14207 0.333984 8.96484 0.333984C8.78813 0.333984 8.63061 0.416228 8.52608 0.544075Z" />
        </svg>
        <span className="ml-6 font-mono text-sm text-neutral-600 dark:text-neutral-400 group-hover/browser:underline group-hover/browser:text-neutral-900 dark:group-hover/browser:text-white transition-all">
          {getActionTitle()}
        </span>
      </div>
    </div>
  );
};

