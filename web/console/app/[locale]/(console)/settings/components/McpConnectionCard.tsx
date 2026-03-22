"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Copy, Check, ChevronDown, ChevronRight } from "lucide-react";
import { Card } from "../../../../components/ui/Card";

const MCP_TOOL_GROUPS = [
  { name: "Asset Management", tools: "assets_list, assets_get" },
  { name: "Command Execution", tools: "exec, exec_multi" },
  { name: "Service Management", tools: "services_list, services_restart" },
  { name: "File Operations", tools: "files_list, files_read" },
  { name: "Docker", tools: "docker_hosts, docker_containers, docker_container_restart, docker_container_logs, docker_container_stats" },
  { name: "System Info", tools: "system_processes, system_network, system_disks, system_packages" },
  { name: "Alerts", tools: "alerts_list, alerts_acknowledge" },
  { name: "Power Management", tools: "asset_reboot, asset_shutdown, asset_wake" },
  { name: "Other", tools: "groups, metrics, schedules, webhooks, saved_actions, credentials, topology, updates, connectors" },
] as const;

const TOTAL_TOOLS = 23;

function CopyButton({ text, label }: { text: string; label: string }) {
  const [copied, setCopied] = useState(false);
  const t = useTranslations("settings");

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // clipboard unavailable in non-secure context
    }
  };

  return (
    <button
      type="button"
      onClick={() => { void handleCopy(); }}
      className="inline-flex items-center gap-1 px-2 py-1 text-[10px] font-mono text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] rounded transition-colors cursor-pointer bg-transparent border-none"
      aria-label={label}
    >
      {copied ? <Check size={12} className="text-[var(--good)]" /> : <Copy size={12} />}
      {copied ? t("mcp.copied") : t("mcp.copy")}
    </button>
  );
}

export function McpConnectionCard() {
  const t = useTranslations("settings");
  const [toolsOpen, setToolsOpen] = useState(false);
  const [hubUrl, setHubUrl] = useState("");
  const [hubLoading, setHubLoading] = useState(true);

  const fetchHubUrl = useCallback(async () => {
    try {
      const res = await fetch("/api/settings/enrollment", { cache: "no-store", signal: AbortSignal.timeout(10_000) });
      if (!res.ok) throw new Error();
      const data = (await res.json().catch(() => null)) as Record<string, unknown> | null;
      const url = typeof data?.hub_url === "string" ? data.hub_url.trim() : "";
      setHubUrl(url || window.location.origin);
    } catch {
      // Fallback to browser origin if enrollment endpoint unavailable
      setHubUrl(window.location.origin);
    } finally {
      setHubLoading(false);
    }
  }, []);

  useEffect(() => { void fetchHubUrl(); }, [fetchHubUrl]);

  const mcpUrl = hubUrl ? `${hubUrl}/mcp` : "";

  const claudeSnippet = `claude mcp add labtether ${mcpUrl}`;
  const genericSnippet = JSON.stringify(
    { mcpServers: { labtether: { url: mcpUrl, transport: "streamable-http" } } },
    null,
    2,
  );

  return (
    <Card className="mb-6">
      <p className="text-xs font-mono uppercase tracking-wider text-[var(--muted)] mb-1">
        {t("mcp.heading")}
      </p>
      <p className="text-xs text-[var(--muted)] mb-4">{t("mcp.description")}</p>

      {hubLoading && <p className="text-xs text-[var(--muted)] py-2">&nbsp;</p>}

      {!hubLoading && <>
      {/* Endpoint */}
      <div className="flex items-center gap-2 mb-4">
        <span className="text-xs text-[var(--muted)]">{t("mcp.endpoint")}:</span>
        <code className="text-xs font-mono text-[var(--text)] bg-[var(--surface)] px-2 py-1 rounded">
          {mcpUrl}
        </code>
        <CopyButton text={mcpUrl} label="Copy MCP URL" />
        <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-blue-500/10 text-blue-400 border border-blue-500/20">
          {t("mcp.transport")}
        </span>
      </div>

      {/* Claude Code snippet */}
      <div className="mb-3">
        <div className="flex items-center justify-between mb-1">
          <span className="text-[10px] font-mono uppercase tracking-wider text-[var(--muted)]">
            {t("mcp.claudeCode")}
          </span>
          <CopyButton text={claudeSnippet} label="Copy Claude Code command" />
        </div>
        <pre className="text-xs font-mono text-[var(--text)] bg-[var(--surface)] border border-[var(--panel-border)] rounded-lg p-3 overflow-x-auto">
          {claudeSnippet}
        </pre>
      </div>

      {/* Generic config snippet */}
      <div className="mb-4">
        <div className="flex items-center justify-between mb-1">
          <span className="text-[10px] font-mono uppercase tracking-wider text-[var(--muted)]">
            {t("mcp.genericConfig")}
          </span>
          <CopyButton text={genericSnippet} label="Copy MCP config" />
        </div>
        <pre className="text-xs font-mono text-[var(--text)] bg-[var(--surface)] border border-[var(--panel-border)] rounded-lg p-3 overflow-x-auto">
          {genericSnippet}
        </pre>
      </div>

      {/* Auth note */}
      <p className="text-[10px] text-[var(--muted)] mb-4 italic">{t("mcp.authNote")}</p>

      {/* Collapsible tools list */}
      <button
        type="button"
        onClick={() => setToolsOpen((prev) => !prev)}
        className="flex items-center gap-1.5 text-xs text-[var(--muted)] hover:text-[var(--text)] transition-colors cursor-pointer bg-transparent border-none p-0"
      >
        {toolsOpen ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        {t("mcp.toolsToggle", { count: TOTAL_TOOLS })}
      </button>

      {toolsOpen && (
        <div className="mt-2 space-y-1.5 ml-5">
          {MCP_TOOL_GROUPS.map((group) => (
            <div key={group.name}>
              <span className="text-xs text-[var(--text)]">{group.name}</span>
              <span className="text-[10px] font-mono text-[var(--muted)] ml-2">{group.tools}</span>
            </div>
          ))}
        </div>
      )}
      </>}
    </Card>
  );
}
