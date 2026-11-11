"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { PageHeader } from "../../components/PageHeader";
import { Button } from "../../components/ui/Button";
import { RoutePerfBoundary } from "../../components/RoutePerfBoundary";
import { AddDeviceModal } from "../../components/AddDeviceModal";
import { DashSection, NeedsAttention, Running } from "../../components/dashboard/DashSections";
import { ServerList } from "../../components/dashboard/ServerList";
import { ServicesLauncher } from "../../components/dashboard/ServicesLauncher";
import { useFastStatus, useSlowStatus, useStatusControls } from "../../contexts/StatusContext";
import { useAuth } from "../../contexts/AuthContext";
import { useAlerts } from "../../hooks/useAlerts";
import { useWebServices } from "../../hooks/useWebServices";
import { getVisibilityTier, isAssetHealthy, isDeviceTier } from "../../console/taxonomy";
import { hasAdminRole, hasWriteRole } from "../../lib/roles";
import { DashboardFirstRunChecklistCard } from "./DashboardFirstRunChecklistCard";
import { attentionItems, runningItems, serverCards } from "./dashboardModel";
import { buildDeviceCards } from "./nodes/deviceCards";
import { MapPreview } from "./topology/MapPreview";

/**
 * Dashboard (2.0): what needs you (only when something does) beside what is running (only while something runs), then
 * the servers with CPU over the last day, then every web service one click away. Before the first device, the
 * first-run checklist leads. Last, the small map: the Map's tree down to the first tier under the switch.
 */
export default function DashboardPage() {
  const t = useTranslations("dashboard");
  const { user } = useAuth();
  const canWrite = hasWriteRole(user?.role);
  const canAdmin = hasAdminRole(user?.role);
  const status = useFastStatus();
  const slowStatus = useSlowStatus();
  const { loading, error, fetchStatus } = useStatusControls();
  const tCommon = useTranslations("common");
  const [addDeviceOpen, setAddDeviceOpen] = useState(false);
  const { instances, rules } = useAlerts();
  const { services } = useWebServices({ detailLevel: "compact" });

  const allAssets = useMemo(() => status?.assets ?? [], [status?.assets]);
  const deviceCount = useMemo(() => allAssets.filter((a) => isDeviceTier(a)).length, [allAssets]);
  const problemWorkloads = useMemo(
    () => allAssets.filter((a) => {
      const tier = getVisibilityTier(a);
      return (tier === "workload" || tier === "resource") && !isAssetHealthy(a);
    }),
    [allAssets],
  );
  const cards = useMemo(
    () => [...buildDeviceCards(allAssets, status?.telemetryOverview ?? []).cardsByGroupID.values()].flat(),
    [allAssets, status?.telemetryOverview],
  );
  const ruleNames = useMemo(() => new Map(rules.map((r) => [r.id, r.name])), [rules]);
  const attention = useMemo(
    () => attentionItems({ alerts: instances, ruleNames, cards, problemWorkloads, allAssets }),
    [instances, ruleNames, cards, problemWorkloads, allAssets],
  );
  const running = useMemo(
    () => runningItems({
      actionRuns: slowStatus?.actionRuns ?? [],
      updateRuns: slowStatus?.updateRuns ?? [],
      commands: slowStatus?.recentCommands ?? [],
      sessions: slowStatus?.sessions ?? [],
    }),
    [slowStatus?.actionRuns, slowStatus?.updateRuns, slowStatus?.recentCommands, slowStatus?.sessions],
  );
  const servers = useMemo(() => serverCards(cards), [cards]);

  const needs = attention.length > 0 && (
    <DashSection id="db-needs" title={t("sections.attention")} more={{ href: "/alerts", label: t("links.allAlerts") }}>
      <NeedsAttention items={attention} />
    </DashSection>
  );
  const runs = running.length > 0 && (
    <DashSection id="db-run" title={t("sections.running")}>
      <Running items={running} />
    </DashSection>
  );

  return (
    <RoutePerfBoundary
      route="dashboard"
      sampleSize={allAssets.length}
      metadata={{ assets: allAssets.length, problem_workloads: problemWorkloads.length, attention: attention.length }}
    >
      <PageHeader title={t("title")} />

      {error ? (
        <div className="db-error" role="alert">
          <p className="bad">{t("statusFailed")}</p>
          <Button size="sm" onClick={() => void fetchStatus()}>{tCommon("retry")}</Button>
        </div>
      ) : null}

      <DashboardFirstRunChecklistCard
        loading={loading && !status}
        hasError={Boolean(error)}
        deviceCount={deviceCount}
        recentCommandCount={slowStatus?.recentCommands?.length ?? 0}
        recentLogCount={slowStatus?.recentLogs?.length ?? 0}
        canWrite={canWrite}
        canAdmin={canAdmin}
        onAddDevice={() => setAddDeviceOpen(true)}
      />

      <div className="db">
        {needs && runs ? <div className="db-top">{needs}{runs}</div> : needs || runs}
        {servers.length > 0 && (
          <DashSection id="db-servers" title={t("sections.servers")} more={{ href: "/nodes", label: t("links.allDevices") }}>
            <ServerList servers={servers} allCards={cards} />
          </DashSection>
        )}
        {services.length > 0 && (
          <DashSection id="db-apps" title={t("sections.services")} more={{ href: "/services", label: t("links.allServices") }}>
            <ServicesLauncher services={services} assets={allAssets} />
          </DashSection>
        )}
        <MapPreview />
      </div>

      <AddDeviceModal open={addDeviceOpen} onClose={() => setAddDeviceOpen(false)} />
    </RoutePerfBoundary>
  );
}
