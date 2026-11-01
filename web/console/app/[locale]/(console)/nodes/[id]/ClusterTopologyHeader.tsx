"use client";

import { useTranslations } from "next-intl";
import type { ClusterStatusEntry, TopologyView } from "./clusterTopologyTypes";
import { DiSeg } from "./proxmox/DiControls";
import { DiStatus } from "./proxmox/DiList";

type ClusterTopologyHeaderProps = {
  topologyView: TopologyView;
  onTopologyViewChange: (view: TopologyView) => void;
  clusterEntry?: ClusterStatusEntry;
};

/** The right side of the Cluster title: size, quorum (red only when lost), version and the Graph / List switch. */
export function ClusterTopologyHeader({
  topologyView,
  onTopologyViewChange,
  clusterEntry,
}: ClusterTopologyHeaderProps) {
  const t = useTranslations("devices-proxmox.topology");
  const facts = clusterEntry
    ? [
        clusterEntry.nodes != null ? t("nodes", { count: clusterEntry.nodes }) : "",
        clusterEntry.version != null ? t("version", { version: clusterEntry.version }) : "",
      ].filter(Boolean).join(" · ")
    : "";

  return (
    <>
      {facts ? <span>{facts}</span> : null}
      {clusterEntry?.quorate != null ? (
        clusterEntry.quorate === 1
          ? <span className="di-word">{t("quorate")}</span>
          : <DiStatus tone="bad" word={t("noQuorum")} />
      ) : null}
      <DiSeg
        label={t("viewLabel")}
        options={[{ id: "graph", label: t("graph") }, { id: "list", label: t("list") }]}
        value={topologyView}
        onChange={onTopologyViewChange}
      />
    </>
  );
}
