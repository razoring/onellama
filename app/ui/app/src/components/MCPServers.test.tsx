import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { MCPServers } from "./MCPServers";

describe("MCPServers Component", () => {
  it("renders MCPServers initial markup without crash", () => {
    const html = renderToStaticMarkup(<MCPServers />);
    expect(html).toContain("Loading MCP configuration");
  });
});
