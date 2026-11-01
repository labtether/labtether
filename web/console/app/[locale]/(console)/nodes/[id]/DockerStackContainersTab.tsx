"use client";

import { useTranslations } from "next-intl";
import { useMemo, useState } from "react";
import { ContainerList } from "../../../../components/containers/ContainerList";
import { sortRows, toRow, type Sort, type SortKey } from "../../../../components/containers/containerModel";
import { ListError, ListSkeleton } from "../../../../components/containers/PanelParts";
import { stackMembers } from "../../../../components/containers/stackModel";
import { agentHost, useHostDocker } from "../../../../components/containers/useHostDocker";

type Props = {
  hostId: string;
  stackName: string;
};

/** The containers of one compose stack, as in the Containers list. */
export function DockerStackContainersTab({ hostId, stackName }: Props) {
  const t = useTranslations("containers");
  const td = useTranslations("devices-docker.stacks");
  const { containers, stacks, loading, error, reload } = useHostDocker(hostId, { withStacks: true });
  const [sort, setSort] = useState<Sort>({ key: "name", dir: "asc" });
  const host = useMemo(() => agentHost(hostId), [hostId]);
  const rows = useMemo(() => {
    const stack = stacks.find((s) => s.name.toLowerCase() === stackName.toLowerCase());
    return stack ? stackMembers(stack, containers).map((c) => toRow(host, c)) : [];
  }, [stacks, containers, stackName, host]);
  const onSort = (key: SortKey) => setSort((cur) => (cur.key === key ? { key, dir: cur.dir === "asc" ? "desc" : "asc" } : { key, dir: key === "name" ? "asc" : "desc" }));

  if (loading && !rows.length) return <ListSkeleton rows={3} />;
  if (error && !rows.length) return <ListError text={t("loadFailed")} onRetry={() => void reload()} />;
  return (
    <div className="ct-tab">
      <ContainerList
        label={td("containers")}
        sections={rows.length ? [{ key: "all", title: "", kind: "all", rows: sortRows(rows, sort).map((row) => ({ row, rail: "", showStack: false, showHost: false })) }] : []}
        sort={sort}
        onSort={onSort}
        onChanged={() => void reload()}
        empty={td("noContainers")}
      />
    </div>
  );
}
