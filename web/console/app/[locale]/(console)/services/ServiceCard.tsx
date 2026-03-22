import { memo } from "react";
import { ExternalLink, Info, Pencil, GripVertical, X } from "lucide-react";
import { ServiceIcon } from "../../../components/ServiceIcon";
import type { WebService } from "../../../hooks/useWebServices";
import { ServiceHealthDetails } from "./ServiceHealthDetails";
import {
  compatConnectorLabel,
  extractDomain,
  formatCompatConfidence,
  formatResponseTime,
  proxyProviderLabel,
  serviceLayoutKey,
  statusDotColor,
} from "./servicesPageHelpers";

function hostHue(id: string): number {
  let hash = 0;
  for (let i = 0; i < id.length; i++) {
    hash = (hash * 31 + id.charCodeAt(i)) | 0;
  }
  return ((hash % 360) + 360) % 360;
}

interface ServiceCardProps {
  service: WebService;
  hostName: string;
  expanded: boolean;
  layoutMode: boolean;
  selectionMode: boolean;
  selected: boolean;
  dragging: boolean;
  dragOver: boolean;
  onToggleExpand: (service: WebService) => void;
  onToggleSelect: (service: WebService) => void;
  onEdit: (service: WebService) => void;
  onRename: (service: WebService) => void;
  onToggleHidden: (service: WebService) => void;
  onDeleteManual: (service: WebService) => void;
  onDragStart: (service: WebService) => void;
  onDragOver: (service: WebService) => void;
  onDrop: (service: WebService) => void;
  onDragEnd: () => void;
  onFilterHost?: (hostAssetID: string) => void;
}

function areServiceCardPropsEqual(left: ServiceCardProps, right: ServiceCardProps): boolean {
  return (
    left.service === right.service
    && left.hostName === right.hostName
    && left.expanded === right.expanded
    && left.layoutMode === right.layoutMode
    && left.selectionMode === right.selectionMode
    && left.selected === right.selected
    && left.dragging === right.dragging
    && left.dragOver === right.dragOver
  );
}

export const ServiceCard = memo(function ServiceCard({
  service,
  hostName,
  expanded,
  layoutMode,
  selectionMode,
  selected,
  dragging,
  dragOver,
  onToggleExpand,
  onToggleSelect,
  onEdit,
  onRename,
  onToggleHidden,
  onDeleteManual,
  onDragStart,
  onDragOver,
  onDrop,
  onDragEnd,
  onFilterHost,
}: ServiceCardProps) {
  const isDown = service.status === "down";
  const isHidden = service.metadata?.hidden === "true";
  const sourceLabel = service.source.replace(/_/g, " ");
  const compatConnector = service.metadata?.compat_connector ?? "";
  const compatLabel = compatConnectorLabel(compatConnector);
  const compatConfidence = formatCompatConfidence(service.metadata?.compat_confidence);
  const hasCompat = compatLabel.trim() !== "";
  const domain = extractDomain(service.url);
  const hostColorHue = hostName ? hostHue(service.host_asset_id) : null;
  const health = service.health;
  const hasHealth = Boolean(health && health.checks > 0);
  const uptimeSummary = hasHealth
    ? `${health?.uptime_percent.toFixed(1)}% uptime`
    : "";
  const userTags = service.metadata?.user_tags
    ? service.metadata.user_tags.split(",").map((tag) => tag.trim()).filter(Boolean)
    : [];

  return (
    <div
      draggable={layoutMode}
      onDragStart={(event) => {
        if (!layoutMode) return;
        event.dataTransfer.effectAllowed = "move";
        event.dataTransfer.setData("text/plain", serviceLayoutKey(service));
        onDragStart(service);
      }}
      onDragOver={(event) => {
        if (!layoutMode) return;
        event.preventDefault();
        event.dataTransfer.dropEffect = "move";
        onDragOver(service);
      }}
      onDrop={(event) => {
        if (!layoutMode) return;
        event.preventDefault();
        onDrop(service);
      }}
      onDragEnd={onDragEnd}
      className={`group relative bg-[var(--panel-glass)] border rounded-lg px-3 py-2.5 transition-[border-color,box-shadow,transform,opacity] duration-[var(--dur-fast)] hover:-translate-y-px ${
        layoutMode
          ? "cursor-grab border-[var(--line)]"
          : "border-[var(--panel-border)] hover:border-[var(--line)]"
      } ${dragging ? "opacity-55 scale-[0.98]" : ""} ${dragOver ? "ring-2 ring-[var(--accent)]/50" : ""} ${
        selected ? "ring-2 ring-[var(--accent)]/70" : ""
      } ${
        isDown ? "opacity-60" : ""
      }`}
      style={{
        contain: "layout paint style",
        contentVisibility: "auto",
        containIntrinsicSize: expanded ? "360px 420px" : "320px 184px",
        backdropFilter: "blur(var(--blur-md))",
        WebkitBackdropFilter: "blur(var(--blur-md))",
      }}
    >
      {/* Layout mode grip */}
      {layoutMode && (
        <div className="absolute top-2 left-4 p-1 rounded bg-[var(--surface)]/80 border border-[var(--line)]">
          <GripVertical size={10} className="text-[var(--muted)]" />
        </div>
      )}

      {/* Selection checkbox */}
      {selectionMode && !layoutMode && (
        <label className="absolute top-2 left-4 h-5 px-1.5 rounded border border-[var(--line)] bg-[var(--panel)] text-[10px] text-[var(--text)] inline-flex items-center gap-1 cursor-pointer">
          <input
            type="checkbox"
            checked={selected}
            onChange={() => onToggleSelect(service)}
            onClick={(event) => event.stopPropagation()}
            className="accent-[var(--accent)]"
          />
          Sel
        </label>
      )}

      {/* Hover action buttons */}
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation();
          e.preventDefault();
          onEdit(service);
        }}
        className="absolute top-2 right-8 p-1 rounded opacity-0 group-hover:opacity-100 hover:bg-[var(--hover)] transition-[opacity,background-color] duration-[var(--dur-fast)] cursor-pointer"
        title="Edit service"
      >
        <Pencil size={12} className="text-[var(--muted)]" />
      </button>

      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation();
          onToggleExpand(service);
        }}
        className="absolute top-2 right-2 p-1 rounded opacity-0 group-hover:opacity-100 hover:bg-[var(--hover)] transition-[opacity,background-color] duration-[var(--dur-fast)] cursor-pointer"
        title="Details"
      >
        {expanded ? (
          <X size={12} className="text-[var(--muted)]" />
        ) : (
          <Info size={12} className="text-[var(--muted)]" />
        )}
      </button>

      {/* Card content */}
      <a
        href={service.url}
        target="_blank"
        rel="noopener noreferrer"
        className="block"
        onClick={(event) => {
          if (selectionMode && !layoutMode) {
            event.preventDefault();
            event.stopPropagation();
            onToggleSelect(service);
            return;
          }
          if (layoutMode) {
            event.preventDefault();
            event.stopPropagation();
          }
        }}
      >
        <div className="flex items-start gap-2.5">
          <ServiceIcon iconKey={service.icon_key} size={30} />
          <div className="flex-1 min-w-0 pr-10">
            {/* Row 1: Name + external link */}
            <div className="flex items-center gap-1.5">
              <span
                data-service-name={service.name}
                className="text-[13px] leading-tight font-semibold text-[var(--text)] truncate"
                title={service.name}
              >
                {service.name}
              </span>
              {isHidden && (
                <span className="text-[10px] px-1 py-px rounded bg-[var(--surface)] text-[var(--muted)] flex-shrink-0 uppercase tracking-wider">
                  Hidden
                </span>
              )}
              <ExternalLink
                size={10}
                className="text-[var(--muted)] opacity-0 group-hover:opacity-100 flex-shrink-0 transition-opacity duration-[var(--dur-fast)]"
              />
            </div>

            {/* Row 2: Domain / address */}
            <div
              className="text-xs leading-snug text-[var(--muted)] truncate mt-0.5 font-mono"
              title={service.url}
            >
              {domain}
            </div>

            {/* Row 3: Status bar */}
            <div className="flex items-center gap-1.5 mt-1.5">
              <span
                className={`w-[5px] h-[5px] rounded-full flex-shrink-0 ${statusDotColor(service.status)}`}
                style={
                  service.status === "up"
                    ? { boxShadow: "0 0 6px 1px var(--ok-glow)" }
                    : service.status === "down"
                      ? { boxShadow: "0 0 6px 1px var(--bad-glow)" }
                      : undefined
                }
              />
              <span className="text-[10px] text-[var(--muted)] capitalize">
                {service.status}
              </span>
              {service.response_ms > 0 && (
                <span className="text-[10px] text-[var(--muted)] tabular-nums font-mono">
                  {formatResponseTime(service.response_ms)}
                </span>
              )}
              {hasHealth && (
                <>
                  <span className="text-[10px] text-[var(--muted)] opacity-50">·</span>
                  <span className="text-[10px] text-[var(--muted)] tabular-nums">
                    {health?.uptime_percent.toFixed(1)}%
                  </span>
                </>
              )}
            </div>

            {/* Row 4: Host tag */}
            {hostName && (
              <button
                type="button"
                onClick={(e) => {
                  e.preventDefault();
                  e.stopPropagation();
                  onFilterHost?.(service.host_asset_id);
                }}
                className="mt-1 max-w-full truncate rounded-full px-1.5 py-px text-[10px] font-medium cursor-pointer transition-opacity duration-[var(--dur-fast)] hover:opacity-100 opacity-60"
                style={{
                  backgroundColor: `hsla(${hostColorHue ?? 0}, 70%, 65%, 0.1)`,
                  color: `hsl(${hostColorHue ?? 0}, 70%, 72%)`,
                  border: `1px solid hsla(${hostColorHue ?? 0}, 70%, 65%, 0.18)`,
                }}
                title={`Filter by host: ${hostName}`}
              >
                {hostName}
              </button>
            )}
            {userTags.length > 0 && (
              <div className="flex flex-wrap gap-1 mt-0.5">
                {userTags.map((tag) => (
                  <span
                    key={tag}
                    className="rounded-full px-1.5 py-px text-[10px] font-medium bg-[var(--accent)]/10 text-[var(--accent)] border border-[var(--accent)]/20"
                  >
                    {tag}
                  </span>
                ))}
              </div>
            )}

          </div>
        </div>
      </a>

      {/* Expanded detail view */}
      {expanded && (
        <div className="mt-2.5 pt-2.5 border-t border-[var(--line)] space-y-1">
          <div className="text-xs text-[var(--muted)] truncate" title={service.url}>
            <span className="text-[var(--text)] font-medium">URL: </span>
            {service.url}
          </div>
          <div className="text-xs text-[var(--muted)]">
            <span className="text-[var(--text)] font-medium">Source: </span>
            {sourceLabel}
          </div>
          {service.metadata?.proxy_provider && (
            <div className="text-xs text-[var(--muted)]">
              <span className="text-[var(--text)] font-medium">Proxy: </span>
              {proxyProviderLabel(service.metadata.proxy_provider)}
            </div>
          )}
          {service.source === "docker" && service.container_id && (
            <div className="text-xs text-[var(--muted)] truncate">
              <span className="text-[var(--text)] font-medium">Container: </span>
              {service.container_id.slice(0, 12)}
            </div>
          )}
          {service.metadata?.image && (
            <div className="text-xs text-[var(--muted)] truncate">
              <span className="text-[var(--text)] font-medium">Image: </span>
              {service.metadata.image}
            </div>
          )}
          {service.service_unit && (
            <div className="text-xs text-[var(--muted)] truncate">
              <span className="text-[var(--text)] font-medium">Unit: </span>
              {service.service_unit}
            </div>
          )}
          {service.metadata?.scan_scope && (
            <div className="text-xs text-[var(--muted)]">
              <span className="text-[var(--text)] font-medium">Scan scope: </span>
              {service.metadata.scan_scope}
            </div>
          )}
          {service.metadata?.scan_target_host && (
            <div className="text-xs text-[var(--muted)] truncate" title={service.metadata.scan_target_host}>
              <span className="text-[var(--text)] font-medium">Scanned host: </span>
              {service.metadata.scan_target_host}
            </div>
          )}
          {service.metadata?.router_name && (
            <div className="text-xs text-[var(--muted)] truncate">
              <span className="text-[var(--text)] font-medium">Router: </span>
              {service.metadata.router_name}
            </div>
          )}
          {service.metadata?.backend_url && (
            <div className="text-xs text-[var(--muted)] truncate" title={service.metadata.backend_url}>
              <span className="text-[var(--text)] font-medium">Backend: </span>
              {service.metadata.backend_url}
            </div>
          )}
          {service.metadata?.raw_url && (
            <div className="text-xs text-[var(--muted)] truncate" title={service.metadata.raw_url}>
              <span className="text-[var(--text)] font-medium">Direct: </span>
              {service.metadata.raw_url}
            </div>
          )}
          {service.metadata?.alt_urls && (
            <div className="text-xs text-[var(--muted)] truncate" title={service.metadata.alt_urls}>
              <span className="text-[var(--text)] font-medium">Alt URLs: </span>
              {service.metadata.alt_urls}
            </div>
          )}
          {hasCompat && (
            <div className="text-xs text-[var(--muted)] truncate">
              <span className="text-[var(--text)] font-medium">Compatible API: </span>
              {compatLabel}
              {compatConfidence ? ` (${compatConfidence})` : ""}
            </div>
          )}
          {service.metadata?.compat_auth_hint && (
            <div className="text-xs text-[var(--muted)] truncate">
              <span className="text-[var(--text)] font-medium">Auth hint: </span>
              {service.metadata.compat_auth_hint}
            </div>
          )}
          {service.metadata?.compat_profile && (
            <div className="text-xs text-[var(--muted)] truncate" title={service.metadata.compat_profile}>
              <span className="text-[var(--text)] font-medium">Detection profile: </span>
              {service.metadata.compat_profile}
            </div>
          )}
          {service.metadata?.compat_evidence && (
            <div className="text-xs text-[var(--muted)] truncate" title={service.metadata.compat_evidence}>
              <span className="text-[var(--text)] font-medium">Detection evidence: </span>
              {service.metadata.compat_evidence}
            </div>
          )}
          {hasHealth && <ServiceHealthDetails health={health} currentStatus={service.status} />}
          <div className="pt-1 flex items-center gap-1.5 flex-wrap">
            <button
              type="button"
              onClick={() => onRename(service)}
              className="h-6 px-2 rounded border border-[var(--line)] text-[10px] text-[var(--text)] hover:bg-[var(--hover)] transition-colors duration-[var(--dur-fast)] cursor-pointer"
            >
              Rename
            </button>
            <button
              type="button"
              onClick={() => onToggleHidden(service)}
              className="h-6 px-2 rounded border border-[var(--line)] text-[10px] text-[var(--text)] hover:bg-[var(--hover)] transition-colors duration-[var(--dur-fast)] cursor-pointer"
            >
              {isHidden ? "Unhide" : "Hide"}
            </button>
            {service.source === "manual" && (
              <button
                type="button"
                onClick={() => onDeleteManual(service)}
                className="h-6 px-2 rounded border border-[var(--bad)]/40 text-[10px] text-[var(--bad)] hover:bg-[var(--bad)]/10 transition-colors duration-[var(--dur-fast)] cursor-pointer"
              >
                Delete
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  );
}, areServiceCardPropsEqual);
