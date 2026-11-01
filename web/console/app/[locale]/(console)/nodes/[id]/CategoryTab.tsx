"use client";

import { Fragment, useMemo, useState } from "react";
import { ChevronDown } from "lucide-react";
import { useTranslations } from "next-intl";
import { Link } from "../../../../../i18n/navigation";
import { useFastStatus } from "../../../../contexts/StatusContext";
import { assetFreshnessLabel } from "../../../../console/formatters";
import { buildNodeMetadataSections } from "../../../../console/models";
import type { Asset, TelemetryOverviewAsset } from "../../../../console/models";
import {
  assetCategory,
  childParentKey,
  friendlyTypeLabel,
  hostParentKey,
  isHiddenAsset,
  sortCategoryAssets,
} from "../../../../console/taxonomy";
import type { CategoryDef } from "../../../../console/taxonomy";
import { resolveAssetPresentationStatus, type AssetPresentationStatus } from "../../../../lib/status";

/** "21:12" today, "1 Nov" before today; empty without a time. */
function sinceTime(iso: string): string {
  const at = new Date(iso);
  if (!iso || !Number.isFinite(at.getTime())) return "";
  if (at.toDateString() === new Date().toDateString()) {
    return at.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hourCycle: "h23" });
  }
  return at.toLocaleDateString([], { month: "short", day: "numeric" });
}

function Pct({ value }: { value: number | undefined }) {
  if (value == null || !Number.isFinite(value)) return <span className="dv-n none dmt-u">—</span>;
  const v = Math.round(value);
  return <span className={`dv-n dmt-u${v >= 85 ? " hot" : ""}`}>{v}<span className="u">%</span></span>;
}

function CategoryAssetRow({ asset, telemetry }: { asset: Asset; telemetry: TelemetryOverviewAsset | undefined }) {
  const t = useTranslations("devices.hardware.category");
  const [open, setOpen] = useState(false);
  const state: AssetPresentationStatus = resolveAssetPresentationStatus(
    asset.status,
    assetFreshnessLabel(asset.last_seen_at) as AssetPresentationStatus,
  );
  const sections = buildNodeMetadataSections(asset.metadata);
  const fieldCount = sections.reduce((count, section) => count + section.rows.length, 0);
  const meta = asset.metadata ?? {};
  const time = sinceTime(asset.last_seen_at);
  const online = state === "online";
  const problem = state === "offline" ? (time ? t("offlineSince", { time }) : t("offline"))
    : state === "unresponsive" ? (time ? t("notRespondingSince", { time }) : t("notResponding"))
      : state === "unknown" ? t("noCheckIn") : "";
  const tone = state === "offline" || state === "unknown" ? "bad" : "warn";
  const grey = [friendlyTypeLabel(asset.type), meta.os_pretty_name || meta.os_name].filter(Boolean).join(" · ");
  const metrics = online ? telemetry?.metrics : undefined;

  return (
    <>
      <div className={`dv-row dmt-catrow${open ? " sel" : ""}`}>
        {online ? null : <span className={`dv-dot ${tone} dmt-lead`} aria-hidden="true" />}
        <span className="dv-ln">
          <Link href={`/nodes/${encodeURIComponent(asset.id)}`} className="dv-nm">{asset.name}</Link>
          {problem ? <span className={`dv-prob ${tone}`}>{problem}</span> : <span className="dv-grey">{grey}</span>}
        </span>
        <Pct value={metrics?.cpu_used_percent} />
        <Pct value={metrics?.memory_used_percent} />
        <Pct value={metrics?.disk_used_percent} />
        {fieldCount > 0 ? (
          <button
            type="button"
            className={`dmt-icon${open ? " open" : ""}`}
            aria-expanded={open}
            aria-label={t(open ? "hideDetails" : "showDetails", { name: asset.name })}
            onClick={() => setOpen((value) => !value)}
          >
            <ChevronDown aria-hidden="true" />
          </button>
        ) : <span />}
      </div>
      {open ? (
        <div className="dmt-more">
          {sections.map((section) => (
            <Fragment key={section.title}>
              <h3 className="dmt-more-t">{section.title}</h3>
              <dl className="dv-kv dmt-kv2">
                {section.rows.map((row) => (
                  <div key={row.key}>
                    <dt>{row.label}</dt>
                    <dd><span>{row.value}</span></dd>
                  </div>
                ))}
              </dl>
            </Fragment>
          ))}
        </div>
      ) : null}
    </>
  );
}

/** An infrastructure host's category (a Proxmox host's guests, its pools …): one row per resource under it. */
export function CategoryTab({ category, hostAsset }: {
  category: CategoryDef;
  hostAsset: Asset;
}) {
  const t = useTranslations("devices.hardware.category");
  const status = useFastStatus();
  const allAssets = useMemo(() => status?.assets ?? [], [status?.assets]);
  const parentKey = useMemo(() => hostParentKey(hostAsset), [hostAsset]);
  const telemetryByID = useMemo(
    () => new Map((status?.telemetryOverview ?? []).map((entry) => [entry.asset_id, entry])),
    [status?.telemetryOverview],
  );

  const categoryAssets = useMemo(() => {
    const matched = allAssets.filter((a) => {
      if (a.id === hostAsset.id) return false;
      if (isHiddenAsset(a)) return false;
      if (a.source !== hostAsset.source) return false;
      if (childParentKey(a) !== parentKey) return false;
      return assetCategory(a.type) === category.slug;
    });
    return sortCategoryAssets(matched);
  }, [allAssets, category.slug, hostAsset.id, hostAsset.source, parentKey]);

  if (categoryAssets.length === 0) {
    return <div className="dv-sheet dv-empty"><b>{t("empty")}</b></div>;
  }

  return (
    <div className="dmt">
      <div className="dv-list dmt-cat">
        <div className="dv-cols dmt-catrow" aria-hidden="true">
          <span>{t("name")}</span>
          <span className="r dmt-u">{t("cpu")}</span>
          <span className="r dmt-u">{t("memory")}</span>
          <span className="r dmt-u">{t("disk")}</span>
          <span />
        </div>
        {categoryAssets.map((asset) => (
          <CategoryAssetRow key={asset.id} asset={asset} telemetry={telemetryByID.get(asset.id)} />
        ))}
      </div>
    </div>
  );
}
