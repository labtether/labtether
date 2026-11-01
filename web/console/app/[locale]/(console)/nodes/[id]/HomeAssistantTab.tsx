"use client";

import { useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { ExternalLink, RefreshCw } from "lucide-react";

import { Button } from "../../../../components/ui/Button";
import type { Asset } from "../../../../console/models";
import {
  childParentKey,
  hostParentKey,
  isHomeAssistantEntityAsset,
  isHomeAssistantHubAsset,
} from "../../../../console/taxonomy";
import { useFastStatus } from "../../../../contexts/StatusContext";
import { Link } from "../../../../../i18n/navigation";
import { DiFilter, DiPick } from "./proxmox/DiControls";
import { DiCols, DiEmpty, DiFoot, DiKV, DiList, DiMain, DiNote, DiPage, DiRow, DiSection, DiSheet, DiTime } from "./proxmox/DiList";
import { whenText } from "./proxmox/diFormat";
import {
  metadataValue, otherMetadata, stateText, UNAVAILABLE_STATES, useHomeAssistantSync, type SyncFeedback,
} from "./proxmox/useHomeAssistantSync";

type HomeAssistantTabProps = {
  asset: Asset;
};

const ENTITY_KEYS = new Set([
  "entity_id", "domain", "state", "friendly_name", "original_name", "unit_of_measurement", "device_class", "state_class",
  "entity_category", "supported_features", "assumed_state", "icon", "last_changed", "last_updated", "collector_id",
  "collector_base_url", "connector_type",
]);
const HUB_KEYS = new Set(["collector_id", "collector_base_url", "base_url", "connector_type", "discovered"]);
const PREVIEW = 25;

function Feedback({ feedback }: { feedback: SyncFeedback | null }) {
  if (!feedback) return null;
  return <DiNote tone={feedback.kind === "error" ? "bad" : feedback.kind === "warning" ? "warn" : ""}>{feedback.message}</DiNote>;
}

function SyncButton({ syncing, onSync }: { syncing: boolean; onSync: () => void }) {
  const t = useTranslations("devices-proxmox.ha");
  return (
    <Button size="sm" disabled={syncing} onClick={onSync}>
      <RefreshCw size={14} className={syncing ? "animate-spin" : undefined} aria-hidden="true" />
      {syncing ? t("syncing") : t("syncNow")}
    </Button>
  );
}

/** When the entity last changed state (or was last seen), newest first in the list. */
function changedAt(entity: Asset): number {
  const ms = new Date(entity.metadata?.last_changed || entity.last_seen_at).getTime();
  return Number.isFinite(ms) ? ms : 0;
}

function isUnavailable(entity: Asset): boolean {
  const state = entity.metadata?.state?.trim().toLowerCase();
  return state ? UNAVAILABLE_STATES.has(state) : true;
}

/** A Home Assistant instance: its connection, a searchable entity list with a domain menu, and the rest of its details. */
function HomeAssistantHubTab({ asset }: { asset: Asset }) {
  const t = useTranslations("devices-proxmox.ha");
  const kit = useTranslations("devices-proxmox.kit");
  const locale = useLocale();
  const status = useFastStatus();
  const { feedback, sync, syncing } = useHomeAssistantSync(asset);
  const [query, setQuery] = useState("");
  const [domain, setDomain] = useState("all");
  const [showAll, setShowAll] = useState(false);
  const hubKey = useMemo(() => hostParentKey(asset), [asset]);

  const entities = useMemo(() => (status?.assets ?? [])
    .filter((candidate) => candidate.id !== asset.id
      && candidate.source === "homeassistant"
      && isHomeAssistantEntityAsset(candidate)
      && childParentKey(candidate) === hubKey)
    .sort((left, right) => changedAt(right) - changedAt(left) || left.name.localeCompare(right.name)),
  [asset.id, hubKey, status?.assets]);

  const domains = useMemo(() => Array.from(new Set(entities.map((entity) => entity.metadata?.domain?.trim() || "other"))).sort(), [entities]);
  const unavailable = entities.filter(isUnavailable).length;
  const scope = domain === "all" || domain === "unavailable" || domains.includes(domain) ? domain : "all";
  const needle = query.trim().toLowerCase();
  const hits = entities.filter((entity) => {
    if (scope === "unavailable" && !isUnavailable(entity)) return false;
    if (scope !== "all" && scope !== "unavailable" && (entity.metadata?.domain?.trim() || "other") !== scope) return false;
    if (!needle) return true;
    return [entity.name, entity.metadata?.entity_id, entity.metadata?.state].some((value) => value?.toLowerCase().includes(needle));
  });
  const shown = showAll ? hits : hits.slice(0, PREVIEW);
  const filtered = needle !== "" || scope !== "all";
  const baseURL = metadataValue(asset.metadata, "collector_base_url") ?? metadataValue(asset.metadata, "base_url");
  const safeURL = baseURL && /^https?:\/\//i.test(baseURL) ? baseURL : null;

  return (
    <DiPage className="di-ha">
      <DiSection
        id="ha-connection"
        title={t("connection")}
        right={
          <>
            <SyncButton syncing={syncing} onSync={() => { void sync(); }} />
            {safeURL ? (
              <a className="di-btn" href={safeURL} target="_blank" rel="noopener noreferrer">
                <ExternalLink aria-hidden="true" />
                {t("open")}
              </a>
            ) : null}
          </>
        }
      >
        <Feedback feedback={feedback} />
        <DiSheet kind="tight" className="di-ha-kv">
          <DiKV
            items={[
              [t("baseUrl"), baseURL ? baseURL.replace(/^https?:\/\//i, "").replace(/\/$/, "") : null],
              [t("lastSeen"), whenText(asset.last_seen_at, locale) || null],
              [t("entities"), t("entitiesValue", { count: entities.length, domains: domains.length })],
              [t("unavailable"), unavailable > 0 ? <span key="u" className="di-warn">{unavailable}</span> : "0"],
            ]}
          />
        </DiSheet>
      </DiSection>

      <DiSection id="ha-entities" title={t("entitiesTitle")}>
        <div className="di-tools">
          <DiFilter value={query} onChange={(value) => { setQuery(value); setShowAll(false); }} placeholder={t("filter")} />
          <span className="sp" />
          <DiPick
            label={t("domainLabel")}
            options={[
              { id: "all", label: t("allDomains") },
              ...(unavailable > 0 ? [{ id: "unavailable", label: t("unavailable") }] : []),
              ...domains.map((id) => ({ id, label: id })),
            ]}
            value={scope}
            onChange={(next) => { setDomain(next); setShowAll(false); }}
          />
        </div>
        <DiList cols="minmax(0, 1.6fr) minmax(96px, 0.7fr) 88px" colsS="minmax(0, 1fr) auto" label={t("entitiesTitle")}>
          <DiCols>
            <span>{t("entity")}</span>
            <span>{t("state")}</span>
            <span className="r di-s">{t("lastChanged")}</span>
          </DiCols>
          {shown.length === 0 ? (
            <DiEmpty title={entities.length === 0 ? t("noEntities") : t("noMatch")} line={entities.length === 0 ? t("noEntitiesLine") : undefined} />
          ) : null}
          {shown.map((entity) => {
            const state = stateText(entity.metadata);
            return (
              <DiRow key={entity.id}>
                <DiMain name={entity.name} href={`/nodes/${entity.id}`} grey={<span className="di-mono">{entity.metadata?.entity_id ?? entity.id}</span>} />
                <span className={`di-ha-state${isUnavailable(entity) ? " di-warn" : ""}`}>{state ?? t("unknown")}</span>
                <DiTime className="di-s">{whenText(entity.metadata?.last_changed || entity.last_seen_at, locale)}</DiTime>
              </DiRow>
            );
          })}
          {hits.length > PREVIEW || filtered ? (
            <DiFoot
              left={kit("shown", { shown: shown.length, total: filtered ? entities.length : hits.length })}
              right={
                <span className="di-acts">
                  {hits.length > PREVIEW ? (
                    <button type="button" onClick={() => setShowAll((value) => !value)}>{showAll ? kit("showFewer") : kit("showAll")}</button>
                  ) : null}
                  {filtered ? (
                    <button type="button" onClick={() => { setQuery(""); setDomain("all"); setShowAll(false); }}>{kit("clearFilter")}</button>
                  ) : null}
                </span>
              }
            />
          ) : null}
        </DiList>
      </DiSection>

      <MoreDetails metadata={asset.metadata} shown={HUB_KEYS} />
    </DiPage>
  );
}

function MoreDetails({ metadata, shown }: { metadata: Record<string, string> | undefined; shown: Set<string> }) {
  const t = useTranslations("devices-proxmox.ha");
  const rows = otherMetadata(metadata, shown);
  if (rows.length === 0) return null;
  return (
    <DiSection title={t("moreDetails")}>
      <DiSheet kind="tight">
        <DiKV columns={2} items={rows} />
      </DiSheet>
    </DiSection>
  );
}

/** One Home Assistant entity: its state, the instance it comes from and its attributes. */
function HomeAssistantEntityTab({ asset }: { asset: Asset }) {
  const t = useTranslations("devices-proxmox.ha");
  const locale = useLocale();
  const status = useFastStatus();
  const { feedback, sync, syncing } = useHomeAssistantSync(asset);
  const parentHub = useMemo(() => {
    const parentKey = childParentKey(asset);
    if (!parentKey) return null;
    return (status?.assets ?? []).find((candidate) => candidate.source === "homeassistant"
      && isHomeAssistantHubAsset(candidate)
      && hostParentKey(candidate) === parentKey) ?? null;
  }, [asset, status?.assets]);
  const time = (key: string) => whenText(asset.metadata?.[key], locale) || metadataValue(asset.metadata, key);

  return (
    <DiPage className="di-ha">
      <DiSection id="ha-state" title={t("stateTitle")} right={<SyncButton syncing={syncing} onSync={() => { void sync(); }} />}>
        <Feedback feedback={feedback} />
        <DiSheet kind="tight">
          <DiKV
            columns={2}
            items={[
              [t("currentState"), <b key="s" className="di-ha-now">{stateText(asset.metadata) ?? t("unknown")}</b>],
              [t("lastSeen"), whenText(asset.last_seen_at, locale) || null],
              [t("instance"), parentHub ? <Link key="h" href={`/nodes/${parentHub.id}`} className="di-ha-link">{parentHub.name}</Link> : null],
              [t("entityId"), metadataValue(asset.metadata, "entity_id") ? <span key="e" className="di-mono">{metadataValue(asset.metadata, "entity_id")}</span> : null],
              [t("domain"), metadataValue(asset.metadata, "domain")],
              [t("deviceClass"), metadataValue(asset.metadata, "device_class")],
              [t("stateClass"), metadataValue(asset.metadata, "state_class")],
              [t("category"), metadataValue(asset.metadata, "entity_category")],
              [t("features"), metadataValue(asset.metadata, "supported_features")],
              [t("assumedState"), metadataValue(asset.metadata, "assumed_state")],
              [t("lastChanged"), time("last_changed")],
              [t("lastUpdated"), time("last_updated")],
            ]}
          />
        </DiSheet>
      </DiSection>
      <MoreDetails metadata={asset.metadata} shown={ENTITY_KEYS} />
    </DiPage>
  );
}

export function HomeAssistantTab({ asset }: HomeAssistantTabProps) {
  const t = useTranslations("devices-proxmox.ha");
  if (isHomeAssistantHubAsset(asset)) return <HomeAssistantHubTab asset={asset} />;
  if (isHomeAssistantEntityAsset(asset)) return <HomeAssistantEntityTab asset={asset} />;
  return (
    <DiPage>
      <DiSheet>
        <DiEmpty title={t("notRecognized")} />
      </DiSheet>
    </DiPage>
  );
}
