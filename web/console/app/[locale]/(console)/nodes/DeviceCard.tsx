"use client";

import type { KeyboardEvent, MouseEvent } from "react";
import { useRouter, Link } from "../../../../i18n/navigation";
import { Box, ChevronRight } from "lucide-react";
import { Badge } from "../../../components/ui/Badge";
import { MiniBar } from "../../../components/ui/MiniBar";
import { formatAge } from "../../../console/formatters";
import { friendlySourceLabel, friendlyTypeLabel, sourceIcon } from "../../../console/taxonomy";
import type { DeviceCardData, HAHubSummary } from "./nodesPageUtils";

type DeviceCardProps = {
  card: DeviceCardData;
  /** Composite facet metadata — when present, renders source pills beside the device name. */
  facets?: Array<{ asset_id: string; source: string; type: string }>;
};

function isInteractiveDescendant(
  target: EventTarget | null,
  container: HTMLDivElement,
): boolean {
  if (!(target instanceof HTMLElement)) return false;
  const interactive = target.closest(
    "a, button, input, select, textarea, summary, [role='button'], [role='link']",
  );
  return Boolean(interactive && interactive !== container);
}

function WorkloadSummaryFooter({ card }: { card: DeviceCardData }) {
  const { workloads, dockerHost } = card;
  const parts: string[] = [];
  if (workloads.vms > 0) parts.push(`${workloads.vms} VM${workloads.vms !== 1 ? "s" : ""}`);
  if (workloads.containers > 0) parts.push(`${workloads.containers} container${workloads.containers !== 1 ? "s" : ""}`);
  if (workloads.stacks > 0) parts.push(`${workloads.stacks} stack${workloads.stacks !== 1 ? "s" : ""}`);
  if (workloads.datastores > 0) parts.push(`${workloads.datastores} datastore${workloads.datastores !== 1 ? "s" : ""}`);
  if (workloads.other > 0) parts.push(`${workloads.other} other`);
  if (parts.length === 0 && !dockerHost) return null;

  const summary = parts.join(" \u00b7 ");

  return (
    <div className="border-t border-[var(--line)] px-3 py-2 flex items-center justify-between gap-2">
      {summary ? (
        <span className="text-xs tabular-nums text-[var(--muted)]">{summary}</span>
      ) : null}
      {dockerHost ? (
        <Link
          href={`/nodes/${dockerHost.id}`}
          className="inline-flex items-center gap-1 text-xs text-[var(--muted)] hover:text-[var(--accent)] transition-colors"
          style={{ transitionDuration: "var(--dur-instant)" }}
          title={`View Docker host: ${dockerHost.name}`}
        >
          <Box size={11} className="shrink-0" />
          <span>Docker</span>
        </Link>
      ) : null}
    </div>
  );
}

function HAHubCardBody({ summary }: { summary: HAHubSummary }) {
  return (
    <div className="space-y-0.5 pl-[36px]">
      <div className="text-xs tabular-nums text-[var(--muted)]">
        <span>{summary.entityCount} {summary.entityCount === 1 ? "entity" : "entities"}</span>
        {summary.unavailableCount > 0 ? (
          <span className="text-[var(--warn)]"> &middot; {summary.unavailableCount} unavailable</span>
        ) : null}
      </div>
      {summary.automationCount > 0 ? (
        <div className="text-xs tabular-nums text-[var(--muted)]">
          <span>{summary.automationCount} {summary.automationCount === 1 ? "automation" : "automations"}</span>
          {summary.automationsDisabled > 0 ? (
            <span className="text-[var(--warn)]"> &middot; {summary.automationsDisabled} disabled</span>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

function HADomainFooter({ summary }: { summary: HAHubSummary }) {
  if (summary.domains.length === 0) return null;
  const overflow = summary.totalDomains - summary.domains.length;
  const parts = summary.domains.map(d => `${d.domain}: ${d.count}`);
  if (overflow > 0) parts.push(`+${overflow} more`);

  return (
    <div className="border-t border-[var(--line)] px-3 py-2">
      <span className="text-xs tabular-nums text-[var(--muted)]">{parts.join("  ")}</span>
    </div>
  );
}

export function DeviceCard({ card, facets }: DeviceCardProps) {
  const router = useRouter();
  const { asset, freshness, cpu, mem, disk, hostedOn } = card;
  const isOffline = freshness === "offline" || freshness === "unknown";
  const Icon = sourceIcon(asset.source);
  const assetKindLabel = friendlyTypeLabel(asset.resource_kind ?? asset.type);
  const detailHref = `/nodes/${asset.id}`;

  const navigateToNode = () => {
    router.push(detailHref);
  };

  const handleCardClick = (event: MouseEvent<HTMLDivElement>) => {
    if (isInteractiveDescendant(event.target, event.currentTarget)) return;
    if (event.metaKey || event.ctrlKey) {
      window.open(detailHref, "_blank", "noopener,noreferrer");
      return;
    }
    navigateToNode();
  };

  const handleCardKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (isInteractiveDescendant(event.target, event.currentTarget)) return;
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      navigateToNode();
    }
  };

  return (
    <div
      role="link"
      tabIndex={0}
      onClick={handleCardClick}
      onKeyDown={handleCardKeyDown}
      className="group flex flex-col rounded-lg border border-[var(--line)] bg-[var(--panel-glass)]
        transition-[border-color,box-shadow] hover:border-[var(--accent)]/40 hover:shadow-[0_0_12px_var(--accent-glow)] cursor-pointer
        focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--control-focus-ring)]"
      style={{ transitionDuration: "var(--dur-fast)" }}
    >
      {/* Header */}
      <div className="px-3 py-2.5 space-y-1.5 flex-1">
        {/* Row 1: Icon pill + Name + Facets + Badge + Chevron */}
        <div className="flex items-center gap-2">
          <span className="flex items-center justify-center h-7 w-7 rounded-md bg-[var(--accent-subtle)] shrink-0">
            <Icon size={14} className="text-[var(--accent-text)] group-hover:text-[var(--accent)] transition-colors" />
          </span>
          <span className="text-sm font-medium text-[var(--text)] truncate flex-1">
            {asset.name}
          </span>
          {facets && facets.length > 0 ? (
            <span className="inline-flex items-center gap-1 shrink-0">
              {facets.map((f) => (
                <span
                  key={f.asset_id}
                  className="rounded border border-[var(--accent)]/30 bg-[var(--accent)]/8 px-1.5 py-0.5 text-[10px] text-[var(--accent-text)]"
                  title={`Facet: ${friendlySourceLabel(f.source)} (${friendlyTypeLabel(f.type)})`}
                >
                  {friendlySourceLabel(f.source)}
                </span>
              ))}
            </span>
          ) : null}
          <Badge status={freshness} size="sm" />
          <ChevronRight
            size={14}
            className="text-[var(--muted)] opacity-0 group-hover:opacity-100 transition-opacity shrink-0"
          />
        </div>

        {/* Row 2: Source label + type + age */}
        <div className="flex items-center gap-2 text-xs tabular-nums text-[var(--muted)] pl-[36px]">
          <span>{friendlySourceLabel(asset.source)}</span>
          <span>&middot;</span>
          <span>{card.haHub && asset.metadata?.ha_version
            ? `v${asset.metadata.ha_version}`
            : assetKindLabel}</span>
          <span>&middot;</span>
          {isOffline ? (
            <span className="text-[var(--bad)]">
              {freshness === "offline" ? "Offline" : "Unknown"} &middot; {formatAge(asset.last_seen_at)}
            </span>
          ) : (
            <span>{formatAge(asset.last_seen_at)}</span>
          )}
        </div>

        {/* Row 3: Metric bars (only when online) */}
        {!isOffline && (cpu != null || mem != null || disk != null) ? (
          <div className="flex items-center gap-3 flex-wrap pl-[36px]">
            {cpu != null ? <MiniBar value={cpu} label={`CPU ${Math.round(cpu)}%`} /> : null}
            {mem != null ? <MiniBar value={mem} label={`MEM ${Math.round(mem)}%`} /> : null}
            {disk != null ? <MiniBar value={disk} label={`DSK ${Math.round(disk)}%`} /> : null}
          </div>
        ) : null}

        {/* HA hub summary (entity health + automations) */}
        {card.haHub ? <HAHubCardBody summary={card.haHub} /> : null}

        {/* Hosted-on badge */}
        {hostedOn ? (
          <div className="pl-[36px]">
            <span
              className="inline-flex items-center gap-1 rounded border border-[var(--line)] bg-[var(--surface)]/60 px-1.5 py-0.5 text-[10px] text-[var(--muted)]"
              title={`Hosted on ${hostedOn.name}`}
            >
              on {hostedOn.name}
            </span>
          </div>
        ) : null}
      </div>

      {/* Footer: HA domain breakdown or workload summary */}
      {card.haHub ? <HADomainFooter summary={card.haHub} /> : <WorkloadSummaryFooter card={card} />}
    </div>
  );
}
