"use client";

import { Link } from "../../../../../i18n/navigation";
import { useMemo } from "react";
import { RefreshCw } from "lucide-react";

import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import { formatAge, formatMetadataLabel, formatMetadataValue } from "../../../../console/formatters";
import type { Asset } from "../../../../console/models";
import {
  childParentKey,
  hostParentKey,
  isHomeAssistantEntityAsset,
  isHomeAssistantHubAsset,
} from "../../../../console/taxonomy";
import { useFastStatus, useStatusControls } from "../../../../contexts/StatusContext";

type HomeAssistantTabProps = {
  asset: Asset;
};

type MetadataEntry = {
  key: string;
  label: string;
  value: string;
};

const ENTITY_PRIMARY_METADATA_KEYS = new Set([
  "entity_id",
  "domain",
  "state",
  "friendly_name",
  "original_name",
  "unit_of_measurement",
  "device_class",
  "state_class",
  "entity_category",
  "supported_features",
  "assumed_state",
  "icon",
  "last_changed",
  "last_updated",
  "collector_id",
  "collector_base_url",
  "connector_type",
]);

const HUB_PRIMARY_METADATA_KEYS = new Set([
  "collector_id",
  "collector_base_url",
  "base_url",
  "connector_type",
  "discovered",
]);

const UNAVAILABLE_STATES = new Set(["unknown", "unavailable", "offline"]);

function metadataValue(metadata: Record<string, string> | undefined, key: string): string | null {
  const raw = metadata?.[key];
  if (typeof raw !== "string") {
    return null;
  }
  const trimmed = raw.trim();
  if (!trimmed) {
    return null;
  }
  return formatMetadataValue(key, trimmed);
}

function buildMetadataEntries(
  metadata: Record<string, string> | undefined,
  excludedKeys: Set<string>,
): MetadataEntry[] {
  if (!metadata) {
    return [];
  }

  return Object.entries(metadata)
    .filter(([key, value]) => value.trim() !== "" && !excludedKeys.has(key))
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([key, value]) => ({
      key,
      label: formatMetadataLabel(key),
      value: formatMetadataValue(key, value),
    }));
}

function stateDisplay(metadata: Record<string, string> | undefined): string {
  const state = metadataValue(metadata, "state");
  if (!state) {
    return "Unknown";
  }
  const unit = metadataValue(metadata, "unit_of_measurement");
  return unit ? `${state} ${unit}` : state;
}

function summaryRow(label: string, value: string | null) {
  if (!value) {
    return null;
  }
  return { label, value };
}

function MetadataListCard({ title, entries }: { title: string; entries: MetadataEntry[] }) {
  if (entries.length === 0) {
    return null;
  }

  return (
    <Card className="mb-4">
      <h2 className="mb-3 text-sm font-medium text-[var(--text)]">{title}</h2>
      <dl className="grid grid-cols-1 gap-3 md:grid-cols-2">
        {entries.map((entry) => (
          <div key={entry.key} className="rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2">
            <dt className="text-xs uppercase tracking-wide text-[var(--muted)]">{entry.label}</dt>
            <dd className="mt-1 text-sm text-[var(--text)] break-words">{entry.value}</dd>
          </div>
        ))}
      </dl>
    </Card>
  );
}

function HomeAssistantHubTab({ asset }: { asset: Asset }) {
  const status = useFastStatus();
  const { fetchStatus } = useStatusControls();
  const hubKey = useMemo(() => hostParentKey(asset), [asset]);
  const childEntities = useMemo(() => {
    const assets = status?.assets ?? [];
    return assets
      .filter((candidate) => (
        candidate.id !== asset.id
        && candidate.source === "homeassistant"
        && isHomeAssistantEntityAsset(candidate)
        && childParentKey(candidate) === hubKey
      ))
      .sort((left, right) => {
        const leftDomain = (left.metadata?.domain ?? "").localeCompare(right.metadata?.domain ?? "");
        if (leftDomain !== 0) {
          return leftDomain;
        }
        return left.name.localeCompare(right.name);
      });
  }, [asset.id, hubKey, status?.assets]);

  const domainCounts = useMemo(() => {
    const counts = new Map<string, number>();
    for (const entity of childEntities) {
      const domain = entity.metadata?.domain?.trim() || "other";
      counts.set(domain, (counts.get(domain) ?? 0) + 1);
    }
    return [...counts.entries()].sort((left, right) => {
      if (right[1] !== left[1]) {
        return right[1] - left[1];
      }
      return left[0].localeCompare(right[0]);
    });
  }, [childEntities]);

  const unavailableEntityCount = useMemo(() => {
    return childEntities.filter((entity) => {
      const state = entity.metadata?.state?.trim().toLowerCase();
      return state ? UNAVAILABLE_STATES.has(state) : true;
    }).length;
  }, [childEntities]);

  const connectorRows = [
    summaryRow("Base URL", metadataValue(asset.metadata, "collector_base_url") ?? metadataValue(asset.metadata, "base_url")),
    summaryRow("Collector ID", metadataValue(asset.metadata, "collector_id")),
    summaryRow("Connector Type", metadataValue(asset.metadata, "connector_type")),
    summaryRow("Last Seen", formatAge(asset.last_seen_at)),
  ].filter((row): row is { label: string; value: string } => row !== null);

  const syncRows = [
    { label: "Entities", value: String(childEntities.length) },
    { label: "Domains", value: String(domainCounts.length) },
    { label: "Unavailable", value: String(unavailableEntityCount) },
    summaryRow("Collector Snapshot", metadataValue(asset.metadata, "discovered")),
  ].filter((row): row is { label: string; value: string } => row !== null);

  const additionalMetadata = buildMetadataEntries(asset.metadata, HUB_PRIMARY_METADATA_KEYS);

  return (
    <div className="space-y-4">
      <Card className="mb-4">
        <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-sm font-medium text-[var(--text)]">Home Assistant Hub</h2>
            <p className="mt-1 text-sm text-[var(--muted)]">
              Connector summary for this Home Assistant instance and the entities currently attached to it.
            </p>
          </div>
          <Button
            size="sm"
            variant="secondary"
            onClick={() => {
              void fetchStatus();
            }}
          >
            <RefreshCw size={14} />
            Refresh
          </Button>
        </div>

        <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4">
          {syncRows.map((row) => (
            <div key={row.label} className="rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-3">
              <p className="text-xs uppercase tracking-wide text-[var(--muted)]">{row.label}</p>
              <p className="mt-2 text-lg font-semibold text-[var(--text)]">{row.value}</p>
            </div>
          ))}
        </div>

        {connectorRows.length > 0 ? (
          <dl className="mt-4 grid grid-cols-1 gap-3 md:grid-cols-2">
            {connectorRows.map((row) => (
              <div key={row.label} className="rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2.5">
                <dt className="text-xs uppercase tracking-wide text-[var(--muted)]">{row.label}</dt>
                <dd className="mt-1 text-sm text-[var(--text)] break-words">{row.value}</dd>
              </div>
            ))}
          </dl>
        ) : null}
      </Card>

      <Card className="mb-4">
        <h2 className="mb-3 text-sm font-medium text-[var(--text)]">Domain Breakdown</h2>
        {domainCounts.length === 0 ? (
          <p className="text-sm text-[var(--muted)]">No synced Home Assistant entities are attached to this hub yet.</p>
        ) : (
          <div className="grid grid-cols-1 gap-2 md:grid-cols-2 xl:grid-cols-3">
            {domainCounts.map(([domain, count]) => (
              <div key={domain} className="flex items-center justify-between rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2">
                <span className="text-sm font-medium text-[var(--text)]">{domain}</span>
                <span className="text-xs text-[var(--muted)]">{count} {count === 1 ? "entity" : "entities"}</span>
              </div>
            ))}
          </div>
        )}
      </Card>

      <Card className="mb-4">
        <h2 className="mb-3 text-sm font-medium text-[var(--text)]">Entity Preview</h2>
        {childEntities.length === 0 ? (
          <p className="text-sm text-[var(--muted)]">Run or wait for a Home Assistant sync to populate entity details here.</p>
        ) : (
          <ul className="divide-y divide-[var(--line)]">
            {childEntities.slice(0, 8).map((entity) => (
              <li key={entity.id} className="flex items-center justify-between gap-3 py-2.5">
                <div className="min-w-0">
                  <Link href={`/nodes/${entity.id}`} className="text-sm font-medium text-[var(--accent)] hover:underline">
                    {entity.name}
                  </Link>
                  <p className="mt-0.5 text-xs text-[var(--muted)]">
                    {(entity.metadata?.entity_id ?? entity.id)} · {(entity.metadata?.domain ?? "entity")}
                  </p>
                </div>
                <div className="shrink-0 text-right">
                  <p className="text-sm text-[var(--text)]">{stateDisplay(entity.metadata)}</p>
                  <p className="mt-0.5 text-xs text-[var(--muted)]">{formatAge(entity.last_seen_at)}</p>
                </div>
              </li>
            ))}
          </ul>
        )}
      </Card>

      <MetadataListCard title="Additional Hub Metadata" entries={additionalMetadata} />
    </div>
  );
}

function HomeAssistantEntityTab({ asset }: { asset: Asset }) {
  const status = useFastStatus();
  const { fetchStatus } = useStatusControls();

  const parentHub = useMemo(() => {
    const assets = status?.assets ?? [];
    const parentKey = childParentKey(asset);
    if (!parentKey) {
      return null;
    }
    return assets.find((candidate) => (
      candidate.source === "homeassistant"
      && isHomeAssistantHubAsset(candidate)
      && hostParentKey(candidate) === parentKey
    )) ?? null;
  }, [asset, status?.assets]);

  const summaryRows = [
    summaryRow("Entity ID", metadataValue(asset.metadata, "entity_id")),
    summaryRow("Domain", metadataValue(asset.metadata, "domain")),
    summaryRow("Device Class", metadataValue(asset.metadata, "device_class")),
    summaryRow("State Class", metadataValue(asset.metadata, "state_class")),
    summaryRow("Entity Category", metadataValue(asset.metadata, "entity_category")),
    summaryRow("Supported Features", metadataValue(asset.metadata, "supported_features")),
    summaryRow("Assumed State", metadataValue(asset.metadata, "assumed_state")),
    summaryRow("Last Changed", metadataValue(asset.metadata, "last_changed")),
    summaryRow("Last Updated", metadataValue(asset.metadata, "last_updated")),
    summaryRow("Collector ID", metadataValue(asset.metadata, "collector_id")),
  ].filter((row): row is { label: string; value: string } => row !== null);

  const additionalMetadata = buildMetadataEntries(asset.metadata, ENTITY_PRIMARY_METADATA_KEYS);

  return (
    <div className="space-y-4">
      <Card className="mb-4">
        <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-sm font-medium text-[var(--text)]">Home Assistant Entity</h2>
            <p className="mt-1 text-sm text-[var(--muted)]">
              Current Home Assistant state and synced entity metadata for this asset.
            </p>
          </div>
          <Button
            size="sm"
            variant="secondary"
            onClick={() => {
              void fetchStatus();
            }}
          >
            <RefreshCw size={14} />
            Refresh
          </Button>
        </div>

        <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)] px-4 py-4">
          <p className="text-xs uppercase tracking-wide text-[var(--muted)]">Current State</p>
          <p className="mt-2 text-2xl font-semibold text-[var(--text)]">{stateDisplay(asset.metadata)}</p>
          <p className="mt-1 text-xs text-[var(--muted)]">Last seen {formatAge(asset.last_seen_at)}</p>
        </div>

        {parentHub ? (
          <div className="mt-4 rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2.5">
            <p className="text-xs uppercase tracking-wide text-[var(--muted)]">Source Hub</p>
            <Link href={`/nodes/${parentHub.id}`} className="mt-1 inline-block text-sm font-medium text-[var(--accent)] hover:underline">
              {parentHub.name}
            </Link>
            <p className="mt-0.5 text-xs text-[var(--muted)]">
              {metadataValue(parentHub.metadata, "collector_base_url") ?? metadataValue(parentHub.metadata, "base_url") ?? "Home Assistant"}
            </p>
          </div>
        ) : null}

        {summaryRows.length > 0 ? (
          <dl className="mt-4 grid grid-cols-1 gap-3 md:grid-cols-2">
            {summaryRows.map((row) => (
              <div key={row.label} className="rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2.5">
                <dt className="text-xs uppercase tracking-wide text-[var(--muted)]">{row.label}</dt>
                <dd className="mt-1 text-sm text-[var(--text)] break-words">{row.value}</dd>
              </div>
            ))}
          </dl>
        ) : null}
      </Card>

      <MetadataListCard title="Additional Entity Metadata" entries={additionalMetadata} />
    </div>
  );
}

export function HomeAssistantTab({ asset }: HomeAssistantTabProps) {
  if (isHomeAssistantHubAsset(asset)) {
    return <HomeAssistantHubTab asset={asset} />;
  }

  if (isHomeAssistantEntityAsset(asset)) {
    return <HomeAssistantEntityTab asset={asset} />;
  }

  return (
    <Card className="mb-4">
      <div className="flex flex-col items-center justify-center gap-2 py-12">
        <p className="text-sm font-medium text-[var(--text)]">No Home Assistant details yet</p>
        <p className="max-w-sm text-center text-xs text-[var(--muted)]">
          This asset is not currently recognized as a Home Assistant hub or entity.
        </p>
      </div>
    </Card>
  );
}
