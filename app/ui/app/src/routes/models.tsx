import { createFileRoute } from "@tanstack/react-router";
import { AppSidebar } from "@/components/AppSidebar";
import { ModelsScreen } from "@/components/ModelsScreen";
import { SidebarLayout } from "@/components/layout/layout";

export const Route = createFileRoute("/models")({
  component: ModelsRoute,
});

function ModelsRoute() {
  return (
    <SidebarLayout title="Models" sidebar={<AppSidebar current="models" />}>
      <ModelsScreen />
    </SidebarLayout>
  );
}
