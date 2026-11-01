"use client";

import { Plus } from "lucide-react";
import { useTranslations } from "next-intl";
import { useMemo, useState } from "react";
import { useRouter } from "../../../../i18n/navigation";
import { AddDeviceModal } from "../../../components/AddDeviceModal";
import { PageHeader } from "../../../components/PageHeader";
import { ContainerList } from "../../../components/containers/ContainerList";
import { ContainersToolbar } from "../../../components/containers/ContainersToolbar";
import {
  buildSections,
  dockerHostAssetId,
  filterRows,
  isRunning,
  sortRows,
  type Grouping,
  type Show,
  type Sort,
  type SortKey,
} from "../../../components/containers/containerModel";
import { useContainerFleet } from "../../../components/containers/useContainerFleet";
import { Button } from "../../../components/ui/Button";
import { useAuth } from "../../../contexts/AuthContext";
import { hasWriteRole } from "../../../lib/roles";

const SHOWS: Show[] = ["all", "running", "stopped"];

/** Every container on every Docker host and Portainer endpoint, grouped by host (or stack), with Run container and Deploy stack. */
export default function ContainersPage() {
  const t = useTranslations("containers");
  const router = useRouter();
  const { user } = useAuth();
  const canWrite = hasWriteRole(user?.role);
  const { hosts, rows, loading, error, failed, reload } = useContainerFleet();
  const [query, setQuery] = useState("");
  const [show, setShow] = useState<Show>("all");
  const [grouping, setGrouping] = useState<Grouping>("host");
  const [sort, setSort] = useState<Sort>({ key: "name", dir: "asc" });
  const [adding, setAdding] = useState(false);

  const list = useMemo(() => sortRows(filterRows(rows, query, show), sort), [rows, query, show, sort]);
  const sections = useMemo(() => buildSections(list, grouping, hosts), [list, grouping, hosts]);
  const shows = SHOWS.filter((s) => s === "all" || s === show || rows.some((r) => (s === "running") === isRunning(r.state)));
  const deployHost = hosts.find((h) => h.canDeploy);
  const filtering = query.trim() !== "" || show !== "all";

  const onSort = (key: SortKey) => setSort((cur) => (cur.key === key
    ? { key, dir: cur.dir === "asc" ? "desc" : "asc" }
    : { key, dir: key === "name" ? "asc" : "desc" }));
  const go = (page: "new-container" | "new-compose") => {
    if (deployHost) router.push(`/nodes/${encodeURIComponent(dockerHostAssetId(deployHost.key))}/${page}`);
  };

  const actions = canWrite && deployHost ? (
    <div className="ct-acts">
      <Button onClick={() => go("new-container")}>{t("runContainer")}</Button>
      <Button variant="primary" onClick={() => go("new-compose")}>
        <Plus size={14} aria-hidden="true" />
        {t("deployStack")}
      </Button>
    </div>
  ) : null;

  if (!loading && !error && rows.length === 0) {
    return (
      <>
        <PageHeader title={t("title")} action={actions} />
        <div className="ct-list ct-none">
          <p>{hosts.length ? t("empty.noContainers") : t("empty.noHosts")}</p>
          {canWrite && !hosts.length ? <Button variant="primary" onClick={() => setAdding(true)}>{t("empty.addHost")}</Button> : null}
        </div>
        <AddDeviceModal open={adding} onClose={() => setAdding(false)} />
      </>
    );
  }

  const empty = query.trim() ? t("empty.match", { query: query.trim() }) : t(`empty.${show === "running" ? "noRunning" : "noStopped"}`);
  return (
    <>
      <PageHeader title={t("title")} action={actions} />
      <ContainersToolbar query={query} onQuery={setQuery} show={show} onShow={setShow} shows={shows} grouping={grouping} onGrouping={setGrouping} />
      {failed > 0 ? <p className="ct-note">{t("hostsFailed", { count: failed })}</p> : null}
      {error && !rows.length ? (
        <div className="ct-list ct-none">
          <p>{t("loadFailed")}</p>
          <Button onClick={() => void reload()}>{t("retry")}</Button>
        </div>
      ) : loading && !rows.length ? (
        <div className="ct-list" aria-busy="true">
          {[0, 1, 2, 3].map((i) => <div key={i} className="ct-row ct-skel"><span className="ct-lead" /><i /><i /></div>)}
        </div>
      ) : (
        <ContainerList
          label={t("title")}
          sections={sections}
          sort={sort}
          onSort={onSort}
          onChanged={() => void reload()}
          empty={empty}
          foot={filtering ? (
            <>
              <span>{t("shown", { shown: list.length, total: rows.length })}</span>
              <button type="button" onClick={() => { setQuery(""); setShow("all"); }}>{t("clearFilter")}</button>
            </>
          ) : null}
        />
      )}
    </>
  );
}
