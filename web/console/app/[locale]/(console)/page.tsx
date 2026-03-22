"use client";

import dynamic from "next/dynamic";
import { Link } from "../../../i18n/navigation";
import { memo, useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { PageHeader } from "../../components/PageHeader";
import { RoutePerfBoundary } from "../../components/RoutePerfBoundary";
import { Card } from "../../components/ui/Card";
import { Badge } from "../../components/ui/Badge";
import { Button } from "../../components/ui/Button";
import { DataValue } from "../../components/ui/DataValue";
import { NarrativeSummary } from "../../components/NarrativeSummary";
import { ResourceRanking } from "../../components/ResourceRanking";
import { AddDeviceModal } from "../../components/AddDeviceModal";
import { Sparkline } from "../../console/Sparkline";
import { useFastStatus, useSlowStatus, useStatusControls } from "../../contexts/StatusContext";
import { downloadJSON } from "../../lib/export";
import { useSparklineHistory } from "./useSparklineHistory";
import type { TelemetryOverviewAsset } from "../../console/models";
import { isDeviceTier, isAssetHealthy, getVisibilityTier } from "../../console/taxonomy";
import { DashboardProblemWorkloadsBanner } from "./DashboardProblemWorkloadsBanner";
import { DashboardFirstRunChecklistCard } from "./DashboardFirstRunChecklistCard";
import { useDashboardFleetFocus } from "./useDashboardFleetFocus";
import { fleetAvg, fmtPct, timeAgo, topMetricLabel, topMetricValue } from "./dashboardPageUtils";

const TopologyHero = dynamic(
  () => import("../../components/TopologyHero").then((module) => module.TopologyHero),
  {
    ssr: false,
    loading: () => (
      <Card className="min-h-[280px] border-[var(--panel-border)]">
        <div className="flex h-[280px] items-center justify-center">
          <p className="text-sm text-[var(--muted)]">Loading topology overview...</p>
        </div>
      </Card>
    ),
  },
);

type HotspotTab = "cpu" | "memory" | "disk";

/* ---------- main page ---------- */

export default function DashboardPage() {
  const t = useTranslations('dashboard');
  const status = useFastStatus();
  const slowStatus = useSlowStatus();
  const { loading, error } = useStatusControls();
  const [addDeviceOpen, setAddDeviceOpen] = useState(false);
  const [hotspotTab, setHotspotTab] = useState<HotspotTab>("cpu");

  const telemetry = useMemo(
    () => status?.telemetryOverview ?? [],
    [status?.telemetryOverview],
  );
  const allAssets = useMemo(() => status?.assets ?? [], [status?.assets]);

  // Device-tier counts for KPIs
  const { devices, devicesOnline, deviceCount, problemWorkloads } = useMemo(() => {
    const devs = allAssets.filter(a => isDeviceTier(a));
    const online = devs.filter(a => isAssetHealthy(a)).length;
    const problems = allAssets.filter(a => {
      const tier = getVisibilityTier(a);
      return (tier === "workload" || tier === "resource") && !isAssetHealthy(a);
    });
    return { devices: devs, devicesOnline: online, deviceCount: devs.length, problemWorkloads: problems };
  }, [allAssets]);
  const connectorCount = slowStatus?.connectors?.length ?? 0;
  const recentCommandCount = slowStatus?.recentCommands?.length ?? 0;
  const recentLogCount = slowStatus?.recentLogs?.length ?? 0;
  const assetCount = allAssets.length;
  const telemetryCount = telemetry.length;

  const deviceTelemetry = useMemo(
    () => telemetry.filter(t => isDeviceTier({ type: t.type, source: t.source })),
    [telemetry],
  );
  const narrativeStatus = useMemo(
    () => (
      status && slowStatus
        ? {
            timestamp: status.timestamp,
            summary: {
              ...slowStatus.summary,
              ...status.summary,
            },
            assets: status.assets,
            telemetryOverview: status.telemetryOverview,
            endpoints: status.endpoints,
            groupReliability: slowStatus.groupReliability,
          }
        : null
    ),
    [slowStatus, status],
  );

  const fleetCpu = fleetAvg(deviceTelemetry, (m) => m.cpu_used_percent);
  const fleetMem = fleetAvg(deviceTelemetry, (m) => m.memory_used_percent);
  // Rolling sparkline history — one hook call per KPI metric.
  // Values are numeric so the hook can accumulate a meaningful trend line.
  // Devices KPI uses devicesOnline count; Issues uses problem count.
  const devicesOnlineHistory = useSparklineHistory(status ? devicesOnline : undefined);
  const fleetCpuHistory = useSparklineHistory(fleetCpu ?? undefined);
  const fleetMemHistory = useSparklineHistory(fleetMem ?? undefined);
  const issuesHistory = useSparklineHistory(status ? problemWorkloads.length : undefined);

  const { fleetExpanded, setFleetExpanded, healthyNodes, hasFleetIssues, visibleFleet } =
    useDashboardFleetFocus(deviceTelemetry);

  /* Resource hotspot rankings */
  const hotspotItems = useMemo(() => {
    const getter = (node: TelemetryOverviewAsset) => {
      if (hotspotTab === "cpu") return node.metrics.cpu_used_percent;
      if (hotspotTab === "memory") return node.metrics.memory_used_percent;
      return node.metrics.disk_used_percent;
    };
    return [...deviceTelemetry]
      .filter((n) => getter(n) != null)
      .sort((a, b) => (getter(b) ?? 0) - (getter(a) ?? 0))
      .slice(0, 5)
      .map((n) => ({ id: n.asset_id, name: n.name, value: getter(n) ?? 0 }));
  }, [deviceTelemetry, hotspotTab]);

  /* Activity timeline */
  const timeline = useMemo(() => {
    const commands = (slowStatus?.recentCommands ?? []).map((c) => ({
      id: c.id,
      ts: c.updated_at,
      kind: "command" as const,
      label: c.body,
      status: c.status,
    }));
    const audit = (slowStatus?.recentAudit ?? []).map((e) => ({
      id: e.id,
      ts: e.timestamp,
      kind: "event" as const,
      label: e.type,
      status: e.decision || "n/a",
    }));
    return [...commands, ...audit]
      .sort((a, b) => new Date(b.ts).getTime() - new Date(a.ts).getTime())
      .slice(0, 10);
  }, [slowStatus?.recentCommands, slowStatus?.recentAudit]);

  return (
    <RoutePerfBoundary
      route="dashboard"
      sampleSize={assetCount}
      metadata={{
        assets: assetCount,
        telemetry: telemetryCount,
        connectors: connectorCount,
        problem_workloads: problemWorkloads.length,
      }}
    >
      <div className="animate-fade-in">
      {/* 1. Page Header */}
      <PageHeader
        title={t('title')}
        subtitle={t('subtitle')}
        action={
          <div className="flex items-center gap-2">
            <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              const snapshot = {
                exported_at: new Date().toISOString(),
                kpis: {
                  devices_online: devicesOnline,
                  device_count: deviceCount,
                  fleet_cpu_pct: fleetCpu,
                  fleet_mem_pct: fleetMem,
                  problem_workload_count: problemWorkloads.length,
                },
                assets: allAssets,
                problem_workloads: problemWorkloads,
                telemetry_overview: telemetry,
              };
              downloadJSON(snapshot, `labtether-dashboard-${new Date().toISOString().slice(0, 10)}.json`);
            }}
          >
            {t('export')}
          </Button>
            <Button variant="primary" size="sm" onClick={() => setAddDeviceOpen(true)}>
              {t('addDevice')}
            </Button>
          </div>
        }
      />

      {/* Error banner */}
      {error ? (
        <Card className="flex items-start gap-3 mb-4 border-[var(--bad)]/20">
          <span className="text-[var(--bad)] text-sm font-medium">!</span>
          <div>
            <p className="text-sm text-[var(--bad)]">{error}</p>
            <p className="text-xs text-[var(--muted)]">{t('errorHint')}</p>
          </div>
        </Card>
      ) : null}

      {/* 2. Narrative Summary */}
      <div className="mb-4">
        <NarrativeSummary status={narrativeStatus} loading={loading && !status} />
      </div>

      {/* 2.5 First-Run Checklist */}
      <DashboardFirstRunChecklistCard
        loading={loading && !status}
        hasError={Boolean(error)}
        deviceCount={deviceCount}
        connectorCount={connectorCount}
        recentCommandCount={recentCommandCount}
        recentLogCount={recentLogCount}
        onAddDevice={() => setAddDeviceOpen(true)}
      />

      {/* 3. Topology Hero */}
      <div className="mb-4">
        <TopologyHero />
      </div>

      {/* 3.5. Problem Workloads Banner */}
      <DashboardProblemWorkloadsBanner
        problemWorkloads={problemWorkloads}
        allAssets={allAssets}
      />

      {/* 4. KPI Cards */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-2.5 mb-4">
        {([
          { label: t('kpi.online'), value: status ? `${devicesOnline}/${deviceCount}` : "--", history: devicesOnlineHistory, color: "var(--ok)", rgb: "var(--ok)", isHero: true },
          { label: t('kpi.cpu'), value: fmtPct(fleetCpu), history: fleetCpuHistory, color: "var(--accent)", rgb: "var(--accent)", isHero: false },
          { label: t('kpi.memory'), value: fmtPct(fleetMem), history: fleetMemHistory, color: "var(--warn)", rgb: "var(--warn)", isHero: false },
          { label: t('kpi.failures'), value: status ? String(problemWorkloads.length) : "--", history: issuesHistory, color: "var(--text-secondary)", rgb: "var(--text-secondary)", isHero: false },
        ]).map((kpi, index) => (
          <div
            key={kpi.label}
            style={{ animation: `slide-in 300ms ease-out ${index * 60}ms backwards` }}
          >
            <KpiCard {...kpi} />
          </div>
        ))}
      </div>

      {/* 5. Two-column grid: Fleet Focus + Activity */}
      <div className="grid grid-cols-1 xl:grid-cols-3 gap-4 mb-4">
        {/* Fleet Focus */}
        <Card className="xl:col-span-2">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-xs font-mono uppercase tracking-wider text-[var(--muted)]">
              <span aria-hidden="true">// </span>
              {t('fleetFocus.label')}
            </h2>
            <Link href="/nodes" className="text-xs text-[var(--accent)] hover:underline">
              {t('fleetFocus.devicesLink')} &rarr;
            </Link>
          </div>

          {visibleFleet.length > 0 ? (
            <div className="divide-y divide-[var(--panel-border)]">
              {visibleFleet.map((node) => (
                <div key={node.asset_id} className="flex items-center gap-3 py-2 text-sm rounded hover:bg-[var(--hover)] transition-colors duration-[var(--dur-instant)] px-1 -mx-1">
                  <Badge status={node.normalizedStatus} dot size="sm" />
                  <Link
                    href={`/nodes/${node.asset_id}`}
                    className="text-[var(--text)] hover:underline truncate flex-1"
                  >
                    {node.name}
                  </Link>
                  <span className="text-xs tabular-nums text-[var(--muted)] hidden sm:inline">
                    {topMetricLabel(node)}
                  </span>
                  <div className="w-10 h-1 rounded-full bg-[var(--surface)] overflow-hidden hidden sm:block">
                    <div
                      className="h-full rounded-full"
                      style={{
                        width: `${Math.min(100, topMetricValue(node))}%`,
                        background: topMetricValue(node) >= 80
                          ? "linear-gradient(90deg, var(--color-warn), var(--color-bad))"
                          : "linear-gradient(90deg, var(--accent), var(--accent-text))",
                      }}
                    />
                  </div>
                  <span className="text-xs tabular-nums text-[var(--muted)] w-14 text-right shrink-0">
                    {timeAgo(node.last_seen_at)}
                  </span>
                </div>
              ))}
            </div>
          ) : (
            <p className="text-sm text-[var(--muted)] text-center py-6">
              {loading ? t('fleetFocus.loading') : t('fleetFocus.empty')}
            </p>
          )}

          {hasFleetIssues && healthyNodes.length > 0 ? (
            <button
              type="button"
              onClick={() => setFleetExpanded((prev) => !prev)}
              className="mt-2 text-xs text-[var(--accent)] hover:underline"
            >
              {fleetExpanded ? t('fleetFocus.showIssues') : t('fleetFocus.showAll')}
            </button>
          ) : null}

        </Card>

        {/* Activity */}
        <Card>
          <h2 className="text-xs font-mono uppercase tracking-wider text-[var(--muted)] mb-3">
            <span aria-hidden="true">// </span>
            {t('activity.label')}
          </h2>
          {timeline.length > 0 ? (
            <div className="divide-y divide-[var(--panel-border)]">
              {timeline.map((item) => (
                <div key={item.id} className="flex items-center gap-2 py-2 text-xs rounded hover:bg-[var(--hover)] transition-colors duration-[var(--dur-instant)] px-1 -mx-1">
                  <span className="text-[var(--muted)] tabular-nums w-14 shrink-0">{timeAgo(item.ts)}</span>
                  <span
                    className="text-[10px] font-mono font-bold uppercase tracking-wider px-1.5 py-0.5 rounded"
                    style={{
                      color: item.kind === "command"
                        ? "var(--accent)"
                        : item.status === "failed" ? "var(--bad)" : "var(--text-secondary)",
                      background: item.kind === "command"
                        ? "rgba(var(--accent-rgb),0.1)"
                        : item.status === "failed" ? "var(--bad-glow)" : "rgba(255,255,255,0.04)",
                      boxShadow: item.kind === "command"
                        ? "0 0 8px rgba(var(--accent-rgb),0.05)"
                        : "none",
                    }}
                  >
                    {item.kind === "command" ? t('activity.command') : item.kind === "event" ? t('activity.event') : t('activity.heartbeat')}
                  </span>
                  <span className="text-[var(--text)] truncate flex-1">{item.label}</span>
                </div>
              ))}
            </div>
          ) : (
            <p className="text-xs text-[var(--muted)] text-center py-6">
              {t('activity.empty')}
            </p>
          )}
        </Card>
      </div>

      {/* 6. Resource Hotspots */}
      <Card className="mb-4">
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-xs font-mono uppercase tracking-wider text-[var(--muted)]">
            <span aria-hidden="true">// </span>
            {t('hotspots.label')}
          </h2>
          <div className="flex gap-1">
            {(["cpu", "memory", "disk"] as const).map((tab) => (
              <button
                key={tab}
                type="button"
                onClick={() => setHotspotTab(tab)}
                className={`px-2.5 py-1 rounded text-xs transition-colors ${
                  hotspotTab === tab
                    ? "bg-[var(--accent)]/15 text-[var(--accent-text)]"
                    : "text-[var(--muted)] hover:text-[var(--text)]"
                }`}
              >
                {tab === "cpu" ? t('hotspots.cpu') : tab === "memory" ? t('hotspots.memory') : t('hotspots.disk')}
              </button>
            ))}
          </div>
        </div>
        <ResourceRanking items={hotspotItems} />
      </Card>

      {/* Add Device Modal */}
      <AddDeviceModal
        open={addDeviceOpen}
        onClose={() => setAddDeviceOpen(false)}
      />
      </div>
    </RoutePerfBoundary>
  );
}

/* ---------- sub-components ---------- */

const KpiCard = memo(function KpiCard({
  label,
  value,
  history,
  color,
  isHero,
}: {
  label: string;
  value: string;
  history?: Array<{ value: number }>;
  color: string;
  rgb: string;
  isHero: boolean;
}) {
  return (
    <div
      className="rounded-[11px] p-px hover-lift"
      style={{
        background: isHero
          ? "linear-gradient(135deg, rgba(var(--accent-rgb),0.3), rgba(var(--accent-rgb),0.05) 40%, rgba(var(--accent-rgb),0.15) 60%, rgba(var(--accent-rgb),0.03))"
          : "linear-gradient(180deg, rgba(255,255,255,0.06) 0%, rgba(255,255,255,0.015) 100%)",
      }}
    >
      <div
        className="relative overflow-hidden rounded-[10px] p-3.5"
        style={{
          background: "var(--panel-glass)",
          backdropFilter: "blur(8px) saturate(1.4)",
          WebkitBackdropFilter: "blur(8px) saturate(1.4)",
        }}
      >
        {/* Top-edge specular */}
        <div
          className="absolute top-0 left-[15%] right-[15%] h-px pointer-events-none"
          style={{ background: `linear-gradient(90deg, transparent, color-mix(in srgb, ${color} 25%, transparent), transparent)` }}
        />
        <span style={{ color, lineHeight: 1 }}>
          <DataValue value={value} className="text-2xl font-mono tabular-nums font-bold tracking-tight" />
        </span>
        <p className="text-[10px] font-mono font-bold uppercase tracking-wider text-[var(--muted)] mt-1.5">
          {label}
        </p>
        {history && history.length >= 2 && (
          <div className="w-full mt-2" style={{ height: 14 }}>
            <Sparkline points={history} />
          </div>
        )}
      </div>
    </div>
  );
});
