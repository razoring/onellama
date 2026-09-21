import { createFileRoute } from "@tanstack/react-router";
import { AppSidebar } from "@/components/AppSidebar";
import { ModelsScreen } from "@/components/ModelsScreen";

export const Route = createFileRoute("/models")({
  component: ModelsRoute,
});

function ModelsRoute() {
  return (
    <div className="flex h-screen w-full flex-col bg-white dark:bg-neutral-900">
      <div className="flex flex-1 overflow-hidden">
        <div className="flex h-full w-64 flex-col border-r border-neutral-200 dark:border-neutral-800">
          <div className="flex h-14 items-center px-4">
            <h2 className="text-sm font-semibold text-neutral-800 dark:text-neutral-200">
              Ollama
            </h2>
          </div>
          <AppSidebar current="models" />
        </div>
        <div className="flex flex-1 flex-col overflow-hidden">
          <ModelsScreen />
        </div>
      </div>
    </div>
  );
}
