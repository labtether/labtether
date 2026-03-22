"use client";

import { memo, useMemo, type ReactElement } from "react";
import type { StatusResponse } from "../console/models";
import { isDeviceTier, getVisibilityTier, isAssetHealthy } from "../console/taxonomy";
import { Card } from "./ui/Card";

type NarrativeStatus = Pick<StatusResponse, "timestamp" | "summary" | "assets" | "telemetryOverview" | "endpoints" | "groupReliability">;

type NarrativeSummaryProps = {
  status: NarrativeStatus | null;
  loading?: boolean;
};

/* ---------- tiny helpers ---------- */

function plural(n: number, singular: string, pluralForm?: string): string {
  return n === 1 ? singular : (pluralForm ?? singular + "s");
}

function daysAgo(isoDate: string): number {
  const then = new Date(isoDate).getTime();
  const now = Date.now();
  return Math.max(0, Math.floor((now - then) / 86_400_000));
}

function timeAgo(isoDate: string): string {
  const ms = Date.now() - new Date(isoDate).getTime();
  if (ms < 0) return "just now";
  const minutes = Math.floor(ms / 60_000);
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

/* ---------- styled phrase helpers ---------- */

type Tint = "ok" | "warn" | "bad" | "muted";

function phrase(text: string, tint: Tint): ReactElement {
  const colorClass: Record<Tint, string> = {
    ok: "text-[var(--ok)]",
    warn: "text-[var(--warn)]",
    bad: "text-[var(--bad)]",
    muted: "text-[var(--text-secondary)]",
  };
  return <span className={colorClass[tint]}>{text}</span>;
}

/* ---------- sentence builders ---------- */

type Sentence = {
  key: string;
  node: ReactElement;
};

function buildSentences(status: NarrativeStatus): Sentence[] {
  const sentences: Sentence[] = [];
  const { summary, assets, telemetryOverview, endpoints, groupReliability } = status;

  const deviceAssets = assets.filter(a => isDeviceTier(a));
  const onlineDevices = deviceAssets.filter(a => isAssetHealthy(a));
  const offlineDevices = deviceAssets.filter((a) => a.status === "offline");
  const staleDevices = deviceAssets.filter((a) => a.status === "stale" || a.status === "unresponsive");

  const problemWorkloads = assets.filter(a => {
    const tier = getVisibilityTier(a);
    return (tier === "workload" || tier === "resource") && !isAssetHealthy(a);
  });

  const allHealthy =
    offlineDevices.length === 0 &&
    staleDevices.length === 0 &&
    problemWorkloads.length === 0 &&
    summary.deadLetterCount === 0;

  // --- Overall health headline ---
  if (allHealthy && deviceAssets.length > 0) {
    sentences.push({
      key: "healthy",
      node: (
        <span>
          Your lab is {phrase("healthy", "ok")}.{" "}
          {phrase(`${onlineDevices.length} ${plural(onlineDevices.length, "device")}`, "ok")}{" "}
          online.
        </span>
      ),
    });
  } else if (deviceAssets.length === 0) {
    sentences.push({
      key: "empty",
      node: (
        <span>
          No devices enrolled yet. Add your first device to get started.
        </span>
      ),
    });
  } else {
    // Mixed state
    if (onlineDevices.length > 0) {
      sentences.push({
        key: "online-count",
        node: (
          <span>
            {phrase(`${onlineDevices.length}`, "ok")}{" "}
            of {deviceAssets.length} {plural(deviceAssets.length, "device")} online.
          </span>
        ),
      });
    }
  }

  // --- Offline devices ---
  if (offlineDevices.length > 0) {
    const names = offlineDevices.slice(0, 3).map((a) => a.name);
    const overflow = offlineDevices.length > 3 ? ` +${offlineDevices.length - 3} more` : "";
    sentences.push({
      key: "offline",
      node: (
        <span>
          {phrase(names.join(", ") + overflow, "bad")}{" "}
          {offlineDevices.length === 1 ? "is" : "are"} {phrase("offline", "bad")}.
        </span>
      ),
    });
  }

  // --- Stale devices ---
  if (staleDevices.length > 0) {
    const names = staleDevices.slice(0, 3).map((a) => a.name);
    const overflow = staleDevices.length > 3 ? ` +${staleDevices.length - 3} more` : "";
    sentences.push({
      key: "stale",
      node: (
        <span>
          {phrase(names.join(", ") + overflow, "warn")}{" "}
          {staleDevices.length === 1 ? "hasn't" : "haven't"} reported in —{" "}
          {phrase("possibly stale", "warn")}.
        </span>
      ),
    });
  }

  // --- Problem workloads ---
  if (problemWorkloads.length > 0) {
    const names = problemWorkloads.slice(0, 3).map((a) => a.name);
    const overflow = problemWorkloads.length > 3 ? ` +${problemWorkloads.length - 3} more` : "";
    sentences.push({
      key: "workload-issues",
      node: (
        <span>
          {phrase(`${problemWorkloads.length} ${plural(problemWorkloads.length, "workload")}`, "warn")}{" "}
          need attention: {phrase(names.join(", ") + overflow, "warn")}.
        </span>
      ),
    });
  }

  // --- High disk usage warnings ---
  const highDisk = telemetryOverview.filter(
    (t) => t.metrics.disk_used_percent != null && t.metrics.disk_used_percent >= 85
  );
  if (highDisk.length > 0) {
    for (const entry of highDisk.slice(0, 2)) {
      const pct = Math.round(entry.metrics.disk_used_percent!);
      const tint: Tint = pct >= 95 ? "bad" : "warn";
      sentences.push({
        key: `disk-${entry.asset_id}`,
        node: (
          <span>
            {phrase(entry.name, tint)} disk at{" "}
            {phrase(`${pct}%`, tint)}{" "}
            — {pct >= 95 ? "critically full" : "needs attention"}.
          </span>
        ),
      });
    }
  }

  // --- High temperature warnings ---
  const highTemp = telemetryOverview.filter(
    (t) => t.metrics.temperature_celsius != null && t.metrics.temperature_celsius >= 80
  );
  if (highTemp.length > 0) {
    for (const entry of highTemp.slice(0, 2)) {
      const temp = Math.round(entry.metrics.temperature_celsius!);
      const tint: Tint = temp >= 90 ? "bad" : "warn";
      sentences.push({
        key: `temp-${entry.asset_id}`,
        node: (
          <span>
            {phrase(entry.name, tint)} running hot at{" "}
            {phrase(`${temp}\u00B0C`, tint)}.
          </span>
        ),
      });
    }
  }

  // --- Service endpoints down ---
  const downEndpoints = endpoints.filter((e) => !e.ok);
  if (downEndpoints.length > 0) {
    const names = downEndpoints.slice(0, 3).map((e) => e.name);
    sentences.push({
      key: "endpoints-down",
      node: (
        <span>
          {phrase(names.join(", "), "bad")}{" "}
          {downEndpoints.length === 1 ? "endpoint is" : "endpoints are"}{" "}
          {phrase("down", "bad")}.
        </span>
      ),
    });
  }

  // --- Dead letters ---
  if (summary.deadLetterCount > 0) {
    sentences.push({
      key: "dead-letters",
      node: (
        <span>
          {phrase(`${summary.deadLetterCount} dead ${plural(summary.deadLetterCount, "letter")}`, "warn")}{" "}
          in the queue.
        </span>
      ),
    });
  }

  // --- Group reliability ---
  const poorGroups = groupReliability.filter(
    (sr) => sr.grade === "D" || sr.grade === "F"
  );
  if (poorGroups.length > 0) {
    for (const sr of poorGroups.slice(0, 2)) {
      sentences.push({
        key: `group-${sr.group.id}`,
        node: (
          <span>
            Group {phrase(sr.group.name, "warn")} reliability is{" "}
            {phrase(`grade ${sr.grade}`, sr.grade === "F" ? "bad" : "warn")}{" "}
            ({Math.round(sr.score)}%).
          </span>
        ),
      });
    }
  }

  // --- Sessions ---
  if (summary.sessionCount > 0) {
    sentences.push({
      key: "sessions",
      node: (
        <span>
          {phrase(`${summary.sessionCount} active ${plural(summary.sessionCount, "session")}`, "muted")}.
        </span>
      ),
    });
  }

  // --- Connectors ---
  if (summary.connectorCount > 0 && allHealthy) {
    sentences.push({
      key: "connectors",
      node: (
        <span>
          {phrase(
            `${summary.connectorCount} ${plural(summary.connectorCount, "connector")} reporting.`,
            "muted",
          )}
        </span>
      ),
    });
  }

  // --- Last updated timestamp ---
  if (status.timestamp) {
    sentences.push({
      key: "updated",
      node: (
        <span>
          Updated {phrase(timeAgo(status.timestamp), "muted")}.
        </span>
      ),
    });
  }

  return sentences;
}

/* ---------- main component ---------- */

export const NarrativeSummary = memo(function NarrativeSummary({ status, loading }: NarrativeSummaryProps) {
  const sentences = useMemo(() => {
    if (!status) return [];
    return buildSentences(status);
  }, [status]);

  if (loading || !status) {
    return (
      <Card>
        <div
          className="animate-pulse space-y-2"
          style={{ fontSize: 14, fontWeight: 450, lineHeight: 1.6 }}
        >
          <div className="h-4 w-3/4 rounded bg-[var(--panel-border)]" />
          <div className="h-4 w-1/2 rounded bg-[var(--panel-border)]" />
        </div>
      </Card>
    );
  }

  if (sentences.length === 0) {
    return null;
  }

  const isHealthy = sentences.length > 0 && sentences[0].key === "healthy";
  const orbColor = isHealthy ? "var(--ok)" : "var(--warn)";

  return (
    <Card highlight>
      <div className="flex items-center gap-3">
        {/* Health orb */}
        <div className="relative w-3.5 h-3.5 shrink-0">
          <div
            className="absolute -inset-1 rounded-full"
            style={{ background: `radial-gradient(circle, color-mix(in srgb, ${orbColor} 25%, transparent), transparent 70%)` }}
          />
          <div
            className="w-3.5 h-3.5 rounded-full relative"
            style={{
              background: `radial-gradient(circle at 35% 35%, ${orbColor}, color-mix(in srgb, ${orbColor} 60%, transparent))`,
              boxShadow: `0 0 6px color-mix(in srgb, ${orbColor} 40%, transparent), 0 0 16px color-mix(in srgb, ${orbColor} 15%, transparent)`,
            }}
          />
        </div>
        <p
          className="text-[var(--text)] flex-1"
          style={{ fontSize: 14, fontWeight: 450, lineHeight: 1.6 }}
        >
          {sentences.map((s, i) => (
            <span key={s.key}>
              {i > 0 && " "}
              {s.node}
            </span>
          ))}
        </p>
        {/* Blinking cursor accent */}
        <span
          className="w-0.5 h-3.5 rounded-sm shrink-0 self-center"
          style={{
            background: "var(--accent)",
            animation: "cursor-blink 1s step-end infinite",
            opacity: 0.6,
          }}
        />
      </div>
    </Card>
  );
});
