"use client";

import dynamic from "next/dynamic";
import { useTranslations } from "next-intl";
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Button } from "../../../../../components/ui/Button";
import { ContainerList } from "../../../../../components/containers/ContainerList";
import { nodeHref, railRows, sortRows, toRow, type CtHost, type CtRow, type Sort, type SortKey } from "../../../../../components/containers/containerModel";
import { Bar, ListError, ListSkeleton } from "../../../../../components/containers/PanelParts";
import type { DockerContainer } from "../../../../../../lib/docker";
import { portainerFetch } from "./usePortainerData";
import { buildBrowserWsUrl } from "../../../../../lib/ws";

const XTerminal = dynamic(() => import("../../../../../components/XTerminal"), {
  ssr: false,
  loading: () => <div className="ct-exec-wait" />,
});

type RawContainer = {
  Id: string;
  Names: string[];
  Image: string;
  State: string;
  Status: string;
  Created?: number;
  Labels?: Record<string, string>;
  Ports?: Array<{ IP?: string; PrivatePort: number; PublicPort?: number; Type: string }>;
};

/** Portainer's container list in the hub's Docker shape. */
function toDocker(c: RawContainer): DockerContainer {
  const name = c.Names?.[0] ?? c.Id.slice(0, 12);
  return {
    id: c.Id,
    name: name.startsWith("/") ? name.slice(1) : name,
    image: c.Image,
    state: (c.State || "").toLowerCase(),
    status: c.Status || "",
    created: c.Created ? new Date(c.Created * 1000).toISOString() : "",
    ports: (c.Ports ?? []).filter((p) => p.PublicPort).map((p) => `${p.PublicPort}:${p.PrivatePort}/${p.Type}`).join(", "),
    stack_name: c.Labels?.["com.docker.compose.project"] ?? "",
    labels: c.Labels ?? {},
  };
}

type Props = {
  assetId: string;
  hostName?: string;
  canExec: boolean;
  bar?: ReactNode;
};

/** The endpoint's containers, stacks kept together; Start, Stop…, Restart and Shell in each row's ⋯. */
export function PortainerContainersTab({ assetId, hostName, canExec, bar }: Props) {
  const t = useTranslations("devices-docker.portainer");
  const tc = useTranslations("containers");
  const [containers, setContainers] = useState<RawContainer[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [shell, setShell] = useState<{ name: string; wsUrl: string } | null>(null);
  const [sort, setSort] = useState<Sort>({ key: "name", dir: "asc" });
  const seqRef = useRef(0);

  const load = useCallback(async () => {
    const id = ++seqRef.current;
    setLoading(true);
    try {
      const data = await portainerFetch<RawContainer[]>(`/api/portainer/assets/${encodeURIComponent(assetId)}/containers`);
      if (seqRef.current !== id) return;
      setContainers(data ?? []);
      setError(null);
    } catch (err) {
      if (seqRef.current !== id) return;
      setError(err instanceof Error ? err.message : "error");
    } finally {
      if (seqRef.current === id) setLoading(false);
    }
  }, [assetId]);

  useEffect(() => { void load(); }, [load]);

  const host = useMemo<CtHost>(() => ({ key: assetId, label: hostName || assetId, source: "portainer", href: nodeHref(assetId), canDeploy: false }), [assetId, hostName]);
  const rows = useMemo(() => sortRows(containers.map((c) => toRow(host, toDocker(c))), sort), [containers, host, sort]);
  const onSort = (key: SortKey) => setSort((cur) => (cur.key === key ? { key, dir: cur.dir === "asc" ? "desc" : "asc" } : { key, dir: key === "name" ? "asc" : "desc" }));
  const shellItem = (row: CtRow) => (canExec && row.state.kind === "running" ? [{
    label: tc("menu.shell"),
    onSelect: () => setShell({ name: row.name, wsUrl: buildBrowserWsUrl(`/portainer/assets/${encodeURIComponent(assetId)}/containers/${encodeURIComponent(row.id)}/exec`) }),
  }] : []);

  let body: ReactNode;
  if (loading && !containers.length) body = <ListSkeleton />;
  else if (error && !containers.length) body = <ListError text={tc("loadFailed")} onRetry={() => void load()} />;
  else body = (
    <ContainerList
      label={tc("title")}
      sections={rows.length ? [{ key: "all", title: "", kind: "all", rows: railRows(rows) }] : []}
      sort={sort}
      onSort={onSort}
      onChanged={() => void load()}
      storeKey={`labtether.containers.closed.${assetId}`}
      empty={tc("empty.noContainers")}
      extra={shellItem}
    />
  );

  return (
    <>
      <Bar left={bar} right={<Button variant="ghost" onClick={() => void load()} disabled={loading}>{t("refresh")}</Button>} />
      {body}
      {shell ? (
        <div className="ct-exec" role="dialog" aria-modal="true" aria-label={t("shellIn", { name: shell.name })} onClick={(e) => { if (e.target === e.currentTarget) setShell(null); }}>
          <div className="ct-exec-box">
            <div className="ct-exec-bar">
              <span>{t("shellIn", { name: shell.name })}</span>
              <Button size="sm" variant="ghost" onClick={() => setShell(null)}>{t("disconnect")}</Button>
            </div>
            <div className="ct-exec-term">
              <XTerminal wsUrl={shell.wsUrl} onDisconnected={() => setShell(null)} />
            </div>
          </div>
        </div>
      ) : null}
    </>
  );
}
