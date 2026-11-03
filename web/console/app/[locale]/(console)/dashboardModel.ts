import type { ActionRun, AlertInstance, Asset, Command, Session, UpdateRun } from "../../console/models";
import { alertAssetIds } from "../../console/models/alerts";
import { childParentKey, hostParentKey, isInfraHost } from "../../console/taxonomy";
import { typeSection } from "./nodes/deviceListModel";
import type { DeviceCardData } from "./nodes/nodesPageUtils";

/**
 * One line under Needs attention. title and sub are either plain text (alerts carry their own) or a translation key
 * under dashboard.attention with its values, so the page words devices and workloads in the viewer's language.
 */
export type AttentionItem = {
  key: string;
  tone: "bad" | "warn" | "";
  href: string;
  at: string;
  title: { text: string } | { key: string; values: Record<string, string> };
  sub: { text: string } | { key: string; values: Record<string, string> } | null;
};

const SEVERITY_RANK: Record<AlertInstance["severity"], number> = { critical: 0, high: 1, medium: 2, low: 3 };
const OPEN_ALERT = new Set<AlertInstance["status"]>(["firing", "pending", "acknowledged"]);
/** Critical reads red and high amber; medium and low carry no dot (their word is enough). */
const alertTone = (severity: AlertInstance["severity"]): AttentionItem["tone"] =>
  severity === "critical" ? "bad" : severity === "high" ? "warn" : "";
const DOWN = new Set(["offline", "down", "dead", "failed", "error", "critical"]);

/**
 * Open alerts (most severe first), then devices that are not answering and have no open alert of their own, then
 * containers and services in trouble. Nothing here when all is well; the page then leaves the section out.
 */
export function attentionItems(params: {
  alerts: AlertInstance[];
  ruleNames: Map<string, string>;
  cards: DeviceCardData[];
  problemWorkloads: Asset[];
  allAssets: Asset[];
}): AttentionItem[] {
  const { alerts, ruleNames, cards, problemWorkloads, allAssets } = params;
  const open = alerts
    .filter((a) => OPEN_ALERT.has(a.status))
    .sort((a, b) => SEVERITY_RANK[a.severity] - SEVERITY_RANK[b.severity] || b.started_at.localeCompare(a.started_at));
  const alerted = new Set(open.flatMap((a) => alertAssetIds(a)));

  const fromAlerts: AttentionItem[] = open.map((a) => ({
    key: `alert:${a.id}`,
    tone: alertTone(a.severity),
    href: `/alerts?alert=${encodeURIComponent(a.id)}`,
    at: a.started_at,
    title: { text: a.labels?.rule_name ?? ruleNames.get(a.rule_id) ?? a.rule_id },
    sub: a.annotations?.summary ? { text: a.annotations.summary } : null,
  }));

  const fromDevices: AttentionItem[] = cards
    .filter((c) => c.freshness !== "online" && !alerted.has(c.asset.id))
    .map((c) => ({
      key: `device:${c.asset.id}`,
      tone: c.freshness === "offline" ? "bad" : "warn",
      href: `/nodes/${c.asset.id}`,
      at: c.asset.last_seen_at,
      title: { key: c.freshness === "offline" ? "deviceOffline" : c.freshness === "unknown" ? "deviceNoCheckIn" : "deviceNotResponding", values: { name: c.asset.name } },
      sub: c.freshness === "unknown" ? null : { key: "lastSeen", values: { at: c.asset.last_seen_at } },
    }));

  const hostByKey = new Map<string, Asset>();
  for (const asset of allAssets) if (isInfraHost(asset)) hostByKey.set(hostParentKey(asset), asset);
  const fromWorkloads: AttentionItem[] = problemWorkloads.map((w) => {
    const parentKey = childParentKey(w);
    const host = parentKey ? hostByKey.get(parentKey) : undefined;
    return {
      key: `workload:${w.id}`,
      tone: DOWN.has(w.status) ? "bad" : "warn",
      href: `/nodes/${w.id}`,
      at: w.last_seen_at,
      title: { key: "workloadStatus", values: { name: w.name, status: w.status } },
      sub: host ? { key: "onHost", values: { host: host.name } } : null,
    };
  });

  return [...fromAlerts, ...fromDevices, ...fromWorkloads];
}

/** One job in progress: what runs (and where), when it started, and where to follow it. */
export type RunningItem = { key: string; label: string; target: string; at: string; href: string };
const RUNNING = new Set(["queued", "pending", "running", "in_progress", "started"]);

export function runningItems(params: {
  actionRuns: ActionRun[];
  updateRuns: UpdateRun[];
  commands: Command[];
  sessions: Session[];
}): RunningItem[] {
  const { actionRuns, updateRuns, commands, sessions } = params;
  const targetOf = new Map(sessions.map((s) => [s.id, s.target]));
  return [
    ...actionRuns.filter((r) => RUNNING.has(r.status)).map((r) => ({
      key: `action:${r.id}`, label: r.command || r.action_id || r.type, target: r.target ?? "", at: r.created_at, href: "/actions",
    })),
    ...updateRuns.filter((r) => RUNNING.has(r.status)).map((r) => ({
      key: `update:${r.id}`, label: r.plan_name, target: "", at: r.created_at, href: "/updates",
    })),
    ...commands.filter((c) => RUNNING.has(c.status)).map((c) => ({
      key: `command:${c.id}`, label: c.body, target: targetOf.get(c.session_id) ?? "", at: c.updated_at, href: "/terminal",
    })),
  ].sort((a, b) => b.at.localeCompare(a.at));
}

/** Proxmox hosts, NAS boxes, backup servers and Docker hosts: the machines the rest of the lab runs on. */
export const serverCards = (cards: DeviceCardData[]) =>
  cards
    .filter((c) => ["servers", "storage"].includes(typeSection(c.asset)))
    .sort((a, b) => a.asset.name.localeCompare(b.asset.name));
