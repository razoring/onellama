import { AppSidebar } from "@/components/AppSidebar";
import { SidebarLayout } from "@/components/layout/layout";
import { createFileRoute } from "@tanstack/react-router";
import { ScheduledTasks } from "@/components/ScheduledTasks";

export const Route = createFileRoute("/scheduled")({
  component: ScheduledRoute,
});

function ScheduledRoute() {
  return (
    <SidebarLayout title="Scheduled Tasks" sidebar={<AppSidebar current="scheduled" />}>
      <ScheduledTasks />
    </SidebarLayout>
  );
}
