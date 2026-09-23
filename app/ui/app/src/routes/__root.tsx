import type { QueryClient } from "@tanstack/react-query";
import { createRootRouteWithContext, Outlet } from "@tanstack/react-router";
import { getSettings } from "@/api";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useCloudStatus } from "@/hooks/useCloudStatus";
import { preloadChatData } from "@/lib/chatPreload";
import { preventPageSelectAll } from "@/lib/keyboard";
import { useEffect } from "react";
import GlobalDownloads from "@/components/GlobalDownloads";

function applyTheme(theme: string) {
  const dark =
    theme === "dark" ||
    (theme === "automatic" &&
      window.matchMedia("(prefers-color-scheme: dark)").matches);
  document.documentElement.classList.toggle("dark", dark);
}

function useAppTheme(theme: string | undefined) {
  useEffect(() => {
    if (!theme) return;
    applyTheme(theme);

    if (theme !== "automatic") return;

    const mql = window.matchMedia("(prefers-color-scheme: dark)");
    const onChange = () => applyTheme("automatic");
    mql.addEventListener("change", onChange);
    return () => mql.removeEventListener("change", onChange);
  }, [theme]);
}

function RootComponent() {
  const queryClient = useQueryClient();

  useEffect(() => {
    document.addEventListener("keydown", preventPageSelectAll);
    return () => document.removeEventListener("keydown", preventPageSelectAll);
  }, []);

  useEffect(() => {
    void preloadChatData(queryClient);
  }, [queryClient]);

  // This hook ensures settings are fetched on app startup
  const { data: settingsData } = useQuery({
    queryKey: ["settings"],
    queryFn: getSettings,
  });
  useAppTheme(settingsData?.settings?.Theme);
  // Fetch cloud status on startup (best-effort)
  useCloudStatus();

  return (
    <div>
      <Outlet />
      <GlobalDownloads />
    </div>
  );
}

export const Route = createRootRouteWithContext<{
  queryClient: QueryClient;
}>()({
  component: RootComponent,
});
