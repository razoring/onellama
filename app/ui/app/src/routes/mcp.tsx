import { AppSidebar } from "@/components/AppSidebar";
import { SidebarLayout } from "@/components/layout/layout";
import { createFileRoute } from "@tanstack/react-router";
import { MCPServers } from "@/components/MCPServers";

export const Route = createFileRoute("/mcp")({
  component: MCPRoute,
});

function MCPRoute() {
  return (
    <SidebarLayout title="MCPs" sidebar={<AppSidebar current="mcp" />}>
      <MCPServers />
    </SidebarLayout>
  );
}
