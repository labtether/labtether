import { describe, expect, it } from "vitest";

import {
  buildMcpConnectionSnippets,
  MCP_TOOL_GROUPS,
  TOTAL_TOOLS,
} from "../McpConnectionCard";

describe("McpConnectionCard", () => {
  it("generates authenticated HTTPS client configurations without embedding a real key", () => {
    const snippets = buildMcpConnectionSnippets("https://labtether.example/mcp");

    expect(snippets.claude).toBe(
      'claude mcp add --transport http labtether https://labtether.example/mcp --header "Authorization: Bearer <LABTETHER_API_KEY>"',
    );
    expect(JSON.parse(snippets.generic)).toEqual({
      mcpServers: {
        labtether: {
          url: "https://labtether.example/mcp",
          transport: "streamable-http",
          headers: { Authorization: "Bearer <LABTETHER_API_KEY>" },
        },
      },
    });
  });

  it("keeps the rendered inventory synchronized with the 32 registered tools", () => {
    const tools = MCP_TOOL_GROUPS.flatMap((group) => group.tools);

    expect(TOTAL_TOOLS).toBe(32);
    expect(new Set(tools).size).toBe(TOTAL_TOOLS);
    expect(tools).toContain("whoami");
    expect(tools).toContain("groups_list");
    expect(tools).toContain("docker_container_stats");
    expect(tools).not.toContain("groups");
  });
});
