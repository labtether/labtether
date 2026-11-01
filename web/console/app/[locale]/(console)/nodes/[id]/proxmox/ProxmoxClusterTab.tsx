"use client";

import { useTranslations } from "next-intl";
import type { ClusterStatusEntry } from "../nodeDetailTypes";
import { DiCols, DiDots, DiEmpty, DiKV, DiList, DiMain, DiRow, DiSection, DiSheet, DiStatus } from "./DiList";

type Props = {
  clusterStatus: ClusterStatusEntry[];
};

/** The cluster (name, quorum, size, version) and its nodes, each online or offline. */
export function ProxmoxClusterTab({ clusterStatus }: Props) {
  const t = useTranslations("devices-proxmox.pve.cluster");
  const cluster = clusterStatus.find((entry) => entry.type === "cluster");
  const nodes = clusterStatus
    .filter((entry) => entry.type === "node")
    .sort((left, right) => (left.name ?? "").localeCompare(right.name ?? ""));

  return (
    <>
      {cluster ? (
        <DiSection title={t("cluster")}>
          <DiSheet kind="tight">
            <DiKV
              columns={2}
              items={[
                [t("name"), cluster.name || null],
                [t("quorum"), cluster.quorate == null ? null : cluster.quorate === 1
                  ? <DiStatus key="q" tone="ok" word={t("quorate")} />
                  : <DiStatus key="q" tone="bad" word={t("noQuorum")} />],
                [t("nodes"), cluster.nodes != null ? String(cluster.nodes) : nodes.length ? String(nodes.length) : null],
                [t("version"), cluster.version != null ? String(cluster.version) : null],
              ]}
            />
          </DiSheet>
        </DiSection>
      ) : null}

      <DiSection title={t("nodesTitle")}>
        <DiList cols="minmax(0, 1fr) 96px" label={t("nodesTitle")}>
          <DiCols>
            <span>{t("node")}</span>
            <span className="r">{t("state")}</span>
          </DiCols>
          {nodes.length === 0 ? <DiEmpty title={t("none")} /> : null}
          {nodes.map((node, index) => {
            const online = node.online === 1;
            return (
              <DiRow key={`${node.name ?? "node"}-${index}`} tone={online ? "" : "bad"}>
                <DiMain
                  name={node.name || "—"}
                  grey={
                    <DiDots
                      parts={[
                        node.local === 1 ? t("thisNode") : null,
                        node.ip || null,
                        node.nodeid != null ? t("nodeId", { id: node.nodeid }) : null,
                        node.level || null,
                      ]}
                    />
                  }
                />
                {online ? <span className="di-num">{t("online")}</span> : <DiStatus tone="bad" word={t("offline")} />}
              </DiRow>
            );
          })}
        </DiList>
      </DiSection>
    </>
  );
}
