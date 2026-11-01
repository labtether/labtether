"use client";

import { useTranslations } from "next-intl";
import { useEffect, useMemo } from "react";
import { useDisks } from "../../../../hooks/useDisks";
import { copyText, ListState, RefreshButton, RowMenu, SizeCell } from "./systemViewParts";

function clampPercent(value: number): number {
  return Number.isFinite(value) ? Math.min(Math.max(value, 0), 100) : 0;
}

/** Mounted file systems, fullest first: a bar and its percent (amber at 85% or more), then used, free and size. */
export function DisksTab({ nodeId }: { nodeId: string }) {
  const t = useTranslations("devices.system");
  const { mounts, loading, error, refresh } = useDisks(nodeId);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const rows = useMemo(() => [...mounts].sort((left, right) => clampPercent(right.use_pct) - clampPercent(left.use_pct)), [mounts]);

  return (
    <div className="dsy">
      <div className="dv-bar">
        <span className="dv-sp" />
        <RefreshButton loading={loading} onClick={() => void refresh()} />
      </div>
      <ListState
        loading={loading}
        hasRows={rows.length > 0}
        error={error}
        loadError={t("disks.loadError")}
        empty={t("disks.empty")}
        onRetry={() => void refresh()}
      >
        <div className="dv-list dsy-disk" role="table" aria-label={t("disks.label")}>
          <div className="dv-cols" role="row">
            <span role="columnheader">{t("disks.cols.mount")}</span>
            <span role="columnheader" className="r">{t("disks.cols.usage")}</span>
            <span role="columnheader" className="r c-used">{t("disks.cols.used")}</span>
            <span role="columnheader" className="r c-free">{t("disks.cols.free")}</span>
            <span role="columnheader" className="r c-size">{t("disks.cols.size")}</span>
            <span role="columnheader" aria-label={t("actions")} />
          </div>
          {rows.map((mount) => {
            const usage = clampPercent(mount.use_pct);
            const hot = usage >= 85;
            const grey = [mount.device, mount.fs_type].filter(Boolean).join(" · ");
            return (
              <div key={`${mount.mount_point}-${mount.device}`} className="dv-row" role="row" tabIndex={0}>
                <span role="cell" className="dv-ln">
                  <span className="dv-nm dv-mono dsy-path">{mount.mount_point || "—"}</span>
                  {grey ? <span className="dv-grey">{grey}</span> : null}
                </span>
                <span role="cell" className={`dsy-meter${hot ? " hot" : ""}`}>
                  <span className="dsy-meter-t" aria-hidden="true"><i style={{ width: `${usage}%` }} /></span>
                  <span className="dsy-meter-n">{Math.round(usage)}<span className="u">%</span></span>
                </span>
                <SizeCell bytes={mount.used} className="c-used" />
                <SizeCell bytes={mount.available} className="c-free" />
                <SizeCell bytes={mount.total} className="c-size" />
                <RowMenu
                  name={mount.mount_point}
                  items={[
                    { label: t("disks.copyPath"), onSelect: () => copyText(mount.mount_point) },
                    ...(mount.device ? [{ label: t("disks.copyDevice"), onSelect: () => copyText(mount.device) }] : []),
                  ]}
                />
              </div>
            );
          })}
        </div>
      </ListState>
    </div>
  );
}
