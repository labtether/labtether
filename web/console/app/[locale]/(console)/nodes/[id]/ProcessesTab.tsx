"use client";

import { useTranslations } from "next-intl";
import { useEffect, useMemo, useState } from "react";
import { useProcesses, type ProcessInfo } from "../../../../hooks/useProcesses";
import { copyText, ListState, PctCell, RefreshButton, RowMenu, SizeCell, SortHead, type SortState } from "./systemViewParts";

type ProcSort = "name" | "pid" | "user" | "cpu" | "memory";

const COMPARE: Record<ProcSort, (a: ProcessInfo, b: ProcessInfo) => number> = {
  name: (a, b) => a.name.localeCompare(b.name),
  pid: (a, b) => a.pid - b.pid,
  user: (a, b) => a.user.localeCompare(b.user),
  cpu: (a, b) => a.cpu_pct - b.cpu_pct,
  memory: (a, b) => a.mem_rss - b.mem_rss || a.mem_pct - b.mem_pct,
};

/** The busiest processes on the device (the hub picks them by CPU or memory), with their command line in grey. */
export function ProcessesTab({ nodeId }: { nodeId: string }) {
  const t = useTranslations("devices.system");
  const { processes, loading, error, refresh } = useProcesses(nodeId);
  const [sort, setSort] = useState<SortState<ProcSort>>({ key: "cpu", dir: "desc" });
  // The hub returns the top processes by CPU or by memory; the other columns sort what it sent.
  const fetchBy = sort.key === "memory" ? "memory" : "cpu";

  useEffect(() => {
    void refresh(fetchBy);
  }, [refresh, fetchBy]);

  const rows = useMemo(() => {
    const list = [...processes].sort(COMPARE[sort.key]);
    return sort.dir === "desc" ? list.reverse() : list;
  }, [processes, sort]);

  const head = (k: ProcSort, label: string, right: boolean, className = "") => (
    <SortHead k={k} label={label} sort={sort} onSort={setSort} right={right} className={className} firstDir={right ? "desc" : "asc"} />
  );

  return (
    <div className="dsy">
      <div className="dv-bar">
        {processes.length > 0 ? (
          <span className="dv-hint">{t(fetchBy === "memory" ? "processes.topMemory" : "processes.topCpu", { count: processes.length })}</span>
        ) : null}
        <span className="dv-sp" />
        <RefreshButton loading={loading} onClick={() => void refresh(fetchBy)} />
      </div>
      <ListState
        loading={loading}
        hasRows={rows.length > 0}
        error={error}
        loadError={t("processes.loadError")}
        empty={t("processes.empty")}
        onRetry={() => void refresh(fetchBy)}
      >
        <div className="dv-list dsy-proc" role="table" aria-label={t("processes.label")}>
          <div className="dv-cols" role="row">
            {head("name", t("processes.cols.process"), false)}
            {head("pid", t("processes.cols.pid"), true, "c-pid")}
            {head("user", t("processes.cols.user"), false, "c-user")}
            {head("cpu", t("processes.cols.cpu"), true)}
            <span role="columnheader" className="r c-mempct">{t("processes.cols.memPct")}</span>
            {head("memory", t("processes.cols.memory"), true, "c-rss")}
            <span role="columnheader" aria-label={t("actions")} />
          </div>
          {rows.map((proc) => (
            <div key={proc.pid} className="dv-row" role="row" tabIndex={0}>
              <span role="cell" className="dv-ln">
                <span className="dv-nm">{proc.name || "—"}</span>
                {proc.command && proc.command !== proc.name ? <span className="dv-grey mono dsy-cmd">{proc.command}</span> : null}
              </span>
              <span role="cell" className="dv-n none c-pid">{proc.pid}</span>
              <span role="cell" className="dv-txt c-user">{proc.user || "—"}</span>
              <PctCell value={proc.cpu_pct} />
              <PctCell value={proc.mem_pct} className="c-mempct" />
              <SizeCell bytes={proc.mem_rss} className="c-rss" />
              <RowMenu
                name={proc.name}
                items={[
                  { label: t("processes.copyPid"), onSelect: () => copyText(String(proc.pid)) },
                  { label: t("processes.copyCommand"), onSelect: () => copyText(proc.command || proc.name) },
                ]}
              />
            </div>
          ))}
        </div>
      </ListState>
    </div>
  );
}
