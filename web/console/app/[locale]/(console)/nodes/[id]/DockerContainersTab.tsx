"use client";

import { Plus } from "lucide-react";
import { useTranslations } from "next-intl";
import { useMemo, useState, type ReactNode } from "react";
import { ContainerList } from "../../../../components/containers/ContainerList";
import { FilterField, Seg } from "../../../../components/containers/ContainersToolbar";
import { dockerHostAssetId, filterRows, isRunning, nodeHref, railRows, sortRows, toRow, type Show, type Sort, type SortKey } from "../../../../components/containers/containerModel";
import { Bar, LinkButton, ListError, ListSkeleton } from "../../../../components/containers/PanelParts";
import { agentHost, useHostDocker } from "../../../../components/containers/useHostDocker";
import { useAuth } from "../../../../contexts/AuthContext";
import { hasWriteRole } from "../../../../lib/roles";

/** A Docker host's containers: filter, All / Running / Stopped, the list (stacks kept together) and Run container. */
export function DockerContainersTab({ hostId, bar }: { hostId: string; bar?: ReactNode }) {
  const t = useTranslations("containers");
  const { user } = useAuth();
  const { containers, loading, error, reload } = useHostDocker(hostId);
  const [query, setQuery] = useState("");
  const [show, setShow] = useState<Show>("all");
  const [sort, setSort] = useState<Sort>({ key: "name", dir: "asc" });

  const host = useMemo(() => agentHost(hostId), [hostId]);
  const rows = useMemo(() => containers.map((c) => toRow(host, c)), [containers, host]);
  const list = useMemo(() => sortRows(filterRows(rows, query, show), sort), [rows, query, show, sort]);
  const onSort = (key: SortKey) => setSort((cur) => (cur.key === key ? { key, dir: cur.dir === "asc" ? "desc" : "asc" } : { key, dir: key === "name" ? "asc" : "desc" }));
  const running = rows.filter((r) => isRunning(r.state)).length;

  const run = hasWriteRole(user?.role) ? (
    <LinkButton href={`${nodeHref(dockerHostAssetId(hostId))}/new-container`}>
      <Plus size={14} aria-hidden="true" />
      {t("runContainer")}
    </LinkButton>
  ) : null;

  let body: ReactNode;
  if (loading && !rows.length) body = <ListSkeleton />;
  else if (error && !rows.length) body = <ListError text={t("loadFailed")} onRetry={() => void reload()} />;
  else {
    const empty = !rows.length ? t("empty.noContainers") : query.trim() ? t("empty.match", { query: query.trim() }) : t(`empty.${show === "running" ? "noRunning" : "noStopped"}`);
    body = (
      <ContainerList
        label={t("title")}
        sections={list.length ? [{ key: "all", title: "", kind: "all", rows: railRows(list) }] : []}
        sort={sort}
        onSort={onSort}
        onChanged={() => void reload()}
        storeKey={`labtether.containers.closed.${host.key}`}
        empty={empty}
      />
    );
  }

  return (
    <>
      <Bar left={bar} right={run} />
      {rows.length > 0 ? (
        <div className="ct-tools">
          <FilterField value={query} onChange={setQuery} placeholder={t("filterHost")} />
          <span className="lt-sp" />
          <Seg<Show>
            value={show}
            items={[["all", t("show.all")], ["running", t("show.runningN", { count: running })], ["stopped", t("show.stoppedN", { count: rows.length - running })]]}
            onChange={setShow}
            label={t("show.label")}
          />
        </div>
      ) : null}
      {body}
    </>
  );
}
