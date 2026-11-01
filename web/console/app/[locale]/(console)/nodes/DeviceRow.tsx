"use client";

import { ChevronDown } from "lucide-react";
import { useTranslations } from "next-intl";
import type { MouseEvent } from "react";
import { Link, useRouter } from "../../../../i18n/navigation";
import { formatAge } from "../../../console/formatters";
import { friendlySourceLabel } from "../../../console/taxonomy";
import { deviceIcon, needsAttention } from "./deviceListModel";
import type { DeviceCardData } from "./nodesPageUtils";

/** Usage as a thin bar and its percent, amber at 85% or more; an empty cell without data. */
function UsageBar({ value }: { value: number | null }) {
  if (value == null) return <span className="ld-ub" />;
  const v = Math.max(0, Math.min(100, Math.round(value)));
  return (
    <span className={`ld-ub${v >= 85 ? " hot" : ""}`}>
      <span className="ld-ub-t" aria-hidden="true"><i style={{ width: `${v}%` }} /></span>
      <span className="ld-ub-n">{v}%</span>
    </span>
  );
}

/** The device's kind in a quiet tile, with a status dot on its corner only when something is wrong. */
export function DeviceTile({ card, small = false }: { card: DeviceCardData; small?: boolean }) {
  const Icon = deviceIcon(card.asset);
  const tone = card.freshness === "offline" ? "bad" : card.freshness === "online" ? "" : "warn";
  return (
    <span className={`ld-tile${small ? " sm" : ""}`} aria-hidden="true">
      <Icon strokeWidth={1.75} />
      {tone && <i className={`ld-dot ${tone}`} />}
    </span>
  );
}

/**
 * One device: its tile, the name, then a second line with what it runs and where (or the problem in its tone colour),
 * its address and usage. The whole row opens the device. guests: the Type view's VMs folded under this host, opened by
 * the count in the second line. tree: a folded guest's place under its host ("mid" or "last").
 */
export function DeviceRow({
  card,
  guests = 0,
  open = false,
  onToggleGuests,
  nested = false,
  tree,
}: {
  card: DeviceCardData;
  guests?: number;
  open?: boolean;
  onToggleGuests?: () => void;
  nested?: boolean;
  tree?: "mid" | "last";
}) {
  const t = useTranslations("devices.list");
  const router = useRouter();
  const { asset } = card;
  const href = `/nodes/${asset.id}`;
  const meta = asset.metadata ?? {};
  const address = meta.ip_address || meta.ip || "";

  let line;
  if (needsAttention(card)) {
    const age = formatAge(asset.last_seen_at);
    const word = card.freshness === "offline" ? t("offline", { age }) : card.freshness === "unknown" ? t("noCheckIn") : t("notResponding", { age });
    line = <span className={`ld-prob ${card.freshness === "offline" ? "bad" : "warn"}`}>{word}</span>;
  } else {
    const what = meta.os_pretty_name || meta.os_name || friendlySourceLabel(asset.source);
    const host = card.hostedOn?.name;
    const where = !host ? "" : nested ? (meta.vmid ? t("vm", { vmid: meta.vmid }) : "") : meta.vmid ? t("vmOn", { vmid: meta.vmid, host }) : t("on", { host });
    const containers = card.workloads.containers;
    line = (
      <>
        {[what, where].filter(Boolean).join(" · ")}
        {guests > 0 ? (
          <>
            {" · "}
            <button type="button" className={`ld-gt${open ? " open" : ""}`} aria-expanded={open} onClick={(e) => { e.stopPropagation(); onToggleGuests?.(); }}>
              {t("guests", { count: guests })}
              <ChevronDown aria-hidden="true" strokeWidth={2.4} />
            </button>
          </>
        ) : containers > 0 ? ` · ${t("containers", { count: containers })}` : null}
      </>
    );
  }

  const openDevice = (event: MouseEvent<HTMLDivElement>) => {
    if ((event.target as HTMLElement).closest("a, button")) return;
    if (event.metaKey || event.ctrlKey) window.open(href, "_blank", "noopener,noreferrer");
    else router.push(href);
  };

  return (
    <div className={`ld-row${nested ? " guest" : ""}`} onClick={openDevice}>
      {tree ? <span className={`ld-tree ${tree}`} aria-hidden="true" /> : <DeviceTile card={card} />}
      <div className="ld-who">
        {tree && <DeviceTile card={card} small />}
        <div className="ld-ln">
          <Link href={href} className="ld-nm">{asset.name}</Link>
          <span className="ld-sub">{line}</span>
        </div>
      </div>
      <span className="ld-ip">{address}</span>
      <UsageBar value={needsAttention(card) ? null : card.cpu} />
      <UsageBar value={needsAttention(card) ? null : card.mem} />
      <UsageBar value={needsAttention(card) ? null : card.disk} />
    </div>
  );
}
