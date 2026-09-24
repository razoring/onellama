import { AppSidebar } from "@/components/AppSidebar";
import { SidebarLayout } from "@/components/layout/layout";
import { createFileRoute } from "@tanstack/react-router";
import SystemScreen from "@/components/SystemScreen";

export const Route = createFileRoute("/system")({
  component: SystemRoute,
});

function SystemRoute() {
  return (
    <SidebarLayout title="System" sidebar={<AppSidebar current="system" />}>
      <SystemScreen />
    </SidebarLayout>
  );
}
