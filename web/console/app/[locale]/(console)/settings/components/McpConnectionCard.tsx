"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import { CodeBlock, CopyButton, Facts, Row, Section, Sheet } from "./SettingsKit";
import { useHubUrl } from "./useHubUrl";

export const MCP_TOOL_GROUPS = [
  { name: "Identity", tools: ["whoami"] },
  { name: "Asset Management", tools: ["assets_list", "assets_get"] },
  { name: "Command Execution", tools: ["exec", "exec_multi"] },
  { name: "Service Management", tools: ["services_list", "services_restart"] },
  { name: "File Operations", tools: ["files_list", "files_read"] },
  { name: "Docker", tools: ["docker_hosts", "docker_containers", "docker_container_restart", "docker_container_logs", "docker_container_stats"] },
  { name: "System Info", tools: ["system_processes", "system_network", "system_disks", "system_packages"] },
  { name: "Alerts", tools: ["alerts_list", "alerts_acknowledge"] },
  { name: "Power Management", tools: ["asset_reboot", "asset_shutdown", "asset_wake"] },
  { name: "Other", tools: ["groups_list", "metrics_overview", "schedules_list", "webhooks_list", "saved_actions_list", "credentials_list", "topology_edges", "updates_list_plans", "connectors_health"] },
] as const;

export const TOTAL_TOOLS = MCP_TOOL_GROUPS.reduce((total, group) => total + group.tools.length, 0);

const MCP_API_KEY_PLACEHOLDER = "<LABTETHER_API_KEY>";

export function buildMcpConnectionSnippets(mcpUrl: string) {
  return {
    claude: `claude mcp add --transport http labtether ${mcpUrl} --header "Authorization: Bearer ${MCP_API_KEY_PLACEHOLDER}"`,
    generic: JSON.stringify(
      {
        mcpServers: {
          labtether: {
            url: mcpUrl,
            transport: "streamable-http",
            headers: { Authorization: `Bearer ${MCP_API_KEY_PLACEHOLDER}` },
          },
        },
      },
      null,
      2,
    ),
  };
}

export function McpConnectionCard() {
  const t = useTranslations("settings");
  const [open, setOpen] = useState(false);
  const [toolsOpen, setToolsOpen] = useState(false);
  const { hubUrl, loading: hubLoading } = useHubUrl();

  const mcpUrl = hubUrl ? `${hubUrl}/mcp` : "";

  const snippets = buildMcpConnectionSnippets(mcpUrl);

  return (
    <Section title={t("mcp.heading")} id="mcp">
      <Sheet className={hubLoading ? "st-loading" : ""}>
        {hubLoading ? null : (
          <>
            <Row
              label={t("mcp.endpoint")}
              hint={(
                <span className="st-cv">
                  <code className="st-mono">{mcpUrl}</code>
                  <CopyButton text={mcpUrl} label={t("mcp.copyUrl")} />
                  <span>{t("mcp.transport")}</span>
                </span>
              )}
            >
              <Button size="sm" onClick={() => setOpen((value) => !value)} aria-expanded={open} aria-controls="st-mcp-details">
                {open ? t("mcp.hide") : t("mcp.show")}
              </Button>
            </Row>
            {open ? (
              <div className="st-x" id="st-mcp-details">
                <div className="st-field">
                  <span className="st-lbl">{t("mcp.claudeCode")}</span>
                  <CodeBlock code={snippets.claude} copyLabel={t("mcp.copyClaude")} />
                </div>
                <div className="st-field">
                  <span className="st-lbl">{t("mcp.genericConfig")}</span>
                  <CodeBlock code={snippets.generic} copyLabel={t("mcp.copyConfig")} />
                </div>
                <p className="st-g">{t("mcp.authNote")}</p>
                <div className="st-field">
                  <button type="button" className="st-link st-left" onClick={() => setToolsOpen((prev) => !prev)} aria-expanded={toolsOpen}>
                    {toolsOpen ? t("mcp.toolsHide") : t("mcp.toolsToggle", { count: TOTAL_TOOLS })}
                  </button>
                  {toolsOpen ? (
                    <Facts items={MCP_TOOL_GROUPS.map((group) => [group.name, <code key={group.name} className="st-mono">{group.tools.join(", ")}</code>] as const)} />
                  ) : null}
                </div>
              </div>
            ) : null}
          </>
        )}
      </Sheet>
    </Section>
  );
}
