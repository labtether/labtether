"use client";

// The pools of a TrueNAS device, one line each: name, layout and scrub as grey text (the health word replaces them
// when the pool is not ONLINE), a one-hue meter, the percent and "used of total", and Scrub now. A click on the line
// opens its layout (vdevs and disks), free space and fragmentation.

import { useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Button } from "../../../../../components/ui/Button";
import { DiDot, DiDots, DiKV, DiList, DiMain, DiRow, DiUsage } from "../proxmox/DiList";
import { percentOf, sizeText, whenText } from "../proxmox/diFormat";
import { failureText, healthTone, poolLayout, sortPools } from "./truenasModel";
import { TnFailure, TnListState, useHealthWord } from "./TrueNASParts";
import type { TrueNASPool, TrueNASVdev } from "./truenasTypes";
import { useTrueNASAction } from "./useTrueNASAction";
import { truenasAction, useTrueNASList } from "./useTrueNASData";

export type { TrueNASPool, TrueNASVdev } from "./truenasTypes";

function VdevTree({ vdevs, depth = 0 }: { vdevs: TrueNASVdev[]; depth?: number }) {
  const word = useHealthWord();
  return (
    <ul className="di-tn-vdevs">
      {vdevs.map((vdev, index) => {
        const tone = healthTone(vdev.status);
        return (
          <li key={`${vdev.name}-${index}`}>
            <span className="di-tn-vdev" style={{ paddingLeft: depth * 16 }}>
              {tone ? <DiDot tone={tone} /> : null}
              <span className="di-mono">{vdev.name}</span>
              <span className="di-grey">{vdev.type}{tone ? ` · ${word(vdev.status)}` : ""}</span>
            </span>
            {vdev.children && vdev.children.length > 0 ? <VdevTree vdevs={vdev.children} depth={depth + 1} /> : null}
          </li>
        );
      })}
    </ul>
  );
}

function PoolDetails({ pool }: { pool: TrueNASPool }) {
  const t = useTranslations("devices-truenas.pools");
  return (
    <div className="di-tn-pool-x">
      <DiKV
        columns={2}
        items={[
          [t("free"), sizeText(pool.free)],
          [t("fragmentation"), pool.fragmentation != null ? `${pool.fragmentation}%` : ""],
        ]}
      />
      {pool.topology && pool.topology.length > 0 ? <VdevTree vdevs={pool.topology} /> : null}
    </div>
  );
}

/** The pool lines, from a pools list the caller fetched. */
export function TrueNASPoolLines({ assetId, pools, loading, error, onRefresh }: {
  assetId: string;
  pools: TrueNASPool[];
  loading: boolean;
  error: string | null;
  onRefresh: () => void;
}) {
  const t = useTranslations("devices-truenas.pools");
  const locale = useLocale();
  const word = useHealthWord();
  const [open, setOpen] = useState<string | null>(null);
  const action = useTrueNASAction();

  const layoutText = (pool: TrueNASPool) => {
    const layout = poolLayout(pool.topology);
    if (!layout) return "";
    const kind = layout.kind === "STRIPE" ? t("stripe") : layout.kind === "MIRROR" ? t("mirror") : layout.kind;
    const shape = layout.groups > 1 ? t("groups", { groups: layout.groups, kind }) : kind;
    return `${shape} · ${t("disks", { count: layout.disks })}`;
  };

  const scrub = (pool: TrueNASPool) => {
    void action.run(pool.name, () => truenasAction(
      `/api/truenas/assets/${encodeURIComponent(assetId)}/pools/${encodeURIComponent(pool.name)}/scrub`,
      "POST",
    ), onRefresh);
  };

  return (
    <>
      <TnFailure message={action.error ? t("scrubFailed", { pool: action.error.key, reason: failureText(action.error.message, t("noReason")) }) : null} />
      <DiList
        className="di-tn-pools"
        label={t("label")}
        cols="minmax(0, 1fr) minmax(48px, 160px) 44px 112px auto"
        colsM="minmax(0, 1fr) 44px 112px auto"
        colsS="minmax(0, 1fr) 44px auto"
      >
        <TnListState loading={loading} error={error} count={pools.length} empty={t("empty")} onRetry={onRefresh} failed={(reason) => t("loadFailed", { reason })} />
        {sortPools(pools).map((pool) => {
          const tone = healthTone(pool.status);
          const scrubbed = pool.last_scrub ? t("lastScrub", { when: whenText(pool.last_scrub, locale) }) : t("neverScrubbed");
          const expanded = open === pool.name;
          return (
            <DiRow
              key={pool.name}
              selected={expanded}
              onOpen={() => setOpen(expanded ? null : pool.name)}
              after={expanded ? <PoolDetails pool={pool} /> : null}
            >
              <DiMain
                name={pool.name}
                grey={<DiDots parts={[layoutText(pool), scrubbed]} />}
                problem={tone ? { tone: tone === "warn" ? "warn" : "bad", text: word(pool.status) } : null}
              />
              <DiUsage pct={percentOf(pool.allocated, pool.size)} used={pool.allocated} total={pool.size} />
              <span className="di-acts">
                <Button size="sm" variant="ghost" disabled={action.busy !== null} loading={action.busy === pool.name} onClick={() => scrub(pool)}>
                  {t("scrub")}
                </Button>
              </span>
            </DiRow>
          );
        })}
      </DiList>
    </>
  );
}

/** The pools on their own, fetching them (the tab passes its own list to TrueNASPoolLines instead). */
export function TrueNASPoolsTab({ assetId }: { assetId: string }) {
  const { data, loading, error, refresh } = useTrueNASList<TrueNASPool>(assetId, "pools");
  return <TrueNASPoolLines assetId={assetId} pools={data} loading={loading} error={error} onRefresh={refresh} />;
}
