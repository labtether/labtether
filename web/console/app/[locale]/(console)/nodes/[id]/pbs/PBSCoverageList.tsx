"use client";

// Coverage: every backup group across the datastores with its last backup (amber when over a day old, red when over
// three days or never) and snapshot count. A row opens the group's snapshots; its ⋯ forgets the group after a confirm.

import { useMemo, useState } from "react";

import { DiRowMenu, DiSeg, DiSortHead, useDiConfirm, type DiSortState } from "../proxmox/DiControls";
import { DiCols, DiDots, DiEmpty, DiFoot, DiList, DiMain, DiNote, DiNum, DiRow, DiSection } from "../proxmox/DiList";
import { normalizePBSGroupsResponse, type PBSGroupsResponse } from "../pbsTabModel";
import { PBSGroupSnapshots } from "./PBSGroupSnapshots";
import { coverageName, coverageWhere, coverageRows, sortCoverage, type CoverageRow, type CoverageSortKey } from "./pbsViewModel";
import { pbsAction, pbsErrorDetail, usePBSActions, usePBSResource } from "./usePBSData";
import { usePBSWords } from "./usePBSWords";

const EMPTY: PBSGroupsResponse = { datastores: [] };
const COLS = "minmax(0, 1fr) 120px 72px 28px";
const COLS_S = "minmax(0, 1fr) 96px 28px";

type Show = "all" | "old";

export function PBSCoverageList({ assetId }: { assetId: string }) {
  const { t, tk, when } = usePBSWords();
  const [reloadKey, setReloadKey] = useState(0);
  const groups = usePBSResource(`/api/pbs/assets/${encodeURIComponent(assetId)}/groups`, normalizePBSGroupsResponse, EMPTY, reloadKey);
  const [sort, setSort] = useState<DiSortState<CoverageSortKey>>({ key: "last", dir: "asc" });
  const [show, setShow] = useState<Show>("all");
  const [open, setOpen] = useState<string | null>(null);
  const { busy, note, run } = usePBSActions();
  const { ask, dialog } = useDiConfirm();

  const all = useMemo(() => coverageRows(groups.loaded ? groups.data : null), [groups.data, groups.loaded]);
  const needs = all.filter((row) => row.tone !== "ok");
  const effectiveShow: Show = show === "old" && needs.length > 0 ? "old" : "all";
  const list = sortCoverage(effectiveShow === "old" ? needs : all, sort.key, sort.dir);

  const forget = (row: CoverageRow) => run(
    `forget-${row.key}`,
    () => {
      const params = new URLSearchParams({ store: row.store, ns: row.ns, "backup-type": row.type, "backup-id": row.id });
      return pbsAction(`/api/pbs/assets/${encodeURIComponent(assetId)}/groups/forget?${params.toString()}`, "DELETE");
    },
    t("coverage.forgotten", { name: coverageName(row), store: coverageWhere(row) }),
    t("coverage.forgetFailed", { name: coverageName(row) }),
    () => {
      setOpen(null);
      setReloadKey((key) => key + 1);
    },
  );

  const lastCell = (row: CoverageRow) => {
    if (!row.last) return <span className="di-num di-pbs-bad">{tk("never")}</span>;
    return <span className={`di-num${row.tone === "warn" ? " hot" : row.tone === "bad" ? " di-pbs-bad" : ""}`}>{when(row.last)}</span>;
  };

  return (
    <DiSection
      id="pbs-coverage"
      title={t("coverage.title")}
      right={needs.length > 0 ? (
        <DiSeg<Show>
          label={t("coverage.show")}
          value={effectiveShow}
          onChange={setShow}
          options={[{ id: "all", label: t("coverage.all") }, { id: "old", label: t("coverage.needs") }]}
        />
      ) : null}
    >
      {groups.error ? (
        <DiNote tone="bad">{[t("coverage.loadFailed"), pbsErrorDetail(groups.error)].filter(Boolean).join(" ")}</DiNote>
      ) : null}
      {note ? <DiNote tone={note.tone}>{note.text}</DiNote> : null}
      <DiList cols={COLS} colsS={COLS_S} hang label={t("coverage.title")}>
        <DiCols>
          <DiSortHead id="name" label={t("cols.group")} sort={sort} onSort={setSort} />
          <DiSortHead id="last" label={t("cols.lastBackup")} sort={sort} onSort={setSort} right />
          <DiSortHead id="snaps" label={t("cols.snapshots")} sort={sort} onSort={setSort} right className="di-s" />
          <span />
        </DiCols>
        {list.length === 0 ? (
          <DiEmpty title={groups.loading && !groups.loaded ? tk("loading") : t("coverage.empty")} />
        ) : null}
        {list.map((row) => {
          const expanded = open === row.key;
          const name = coverageName(row);
          const toggle = () => setOpen(expanded ? null : row.key);
          return (
            <DiRow
              key={row.key}
              tone={row.tone === "ok" ? "" : row.tone}
              onOpen={toggle}
              selected={expanded}
              after={expanded ? <PBSGroupSnapshots assetId={assetId} store={row.store} ns={row.ns} type={row.type} id={row.id} /> : null}
            >
              <DiMain
                name={name}
                grey={<DiDots parts={[row.comment ? <span className="di-mono">{`${row.type}/${row.id}`}</span> : null, coverageWhere(row), row.owner]} />}
              />
              {lastCell(row)}
              <DiNum value={row.count || undefined} className="di-s" />
              <DiRowMenu
                label={tk("actionsFor", { name: row.ns ? `${name} ${row.ns}` : name })}
                items={[
                  { label: expanded ? t("coverage.hideSnapshots") : t("coverage.showSnapshots"), onSelect: toggle },
                  { separator: true },
                  {
                    label: t("coverage.forget"),
                    danger: true,
                    disabled: busy !== null,
                    onSelect: () => ask({
                      title: t("coverage.ask.title", { name, store: coverageWhere(row) }),
                      message: t("coverage.ask.body", { count: row.count }),
                      confirmLabel: t("coverage.ask.confirm"),
                      onConfirm: () => forget(row),
                    }),
                  },
                ]}
              />
            </DiRow>
          );
        })}
        {effectiveShow === "old" ? (
          <DiFoot
            left={tk("shown", { shown: list.length, total: all.length })}
            right={<button type="button" onClick={() => setShow("all")}>{tk("clearFilter")}</button>}
          />
        ) : null}
      </DiList>
      {dialog}
    </DiSection>
  );
}
