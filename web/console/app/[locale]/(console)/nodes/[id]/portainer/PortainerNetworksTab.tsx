"use client";

import { useTranslations } from "next-intl";
import { useMemo, type ReactNode } from "react";
import { PortainerResourceTab, type ResourceSpec } from "./PortainerResourceTab";

type DockerNetwork = {
  Id: string;
  Name: string;
  Driver: string;
  Scope: string;
  IPAM?: {
    Config?: Array<{ Subnet?: string; Gateway?: string }>;
  };
};

/** The endpoint's networks: name, driver, subnet, gateway and scope; Create network and Remove…. */
export function PortainerNetworksTab({ assetId, bar }: { assetId: string; bar?: ReactNode }) {
  const t = useTranslations("devices-docker.portainer");
  const spec = useMemo<ResourceSpec<DockerNetwork>>(() => ({
    kind: "networks",
    parse: (data) => (Array.isArray(data) ? (data as DockerNetwork[]) : []),
    id: (n) => n.Id,
    name: (n) => n.Name,
    grid: "minmax(0, 1fr) 80px minmax(0, 1fr) minmax(0, .8fr) 64px",
    columns: [
      { label: t("name"), cls: "ct-cut mono", cell: (n) => n.Name },
      { label: t("driver"), cls: "ct-cut", cell: (n) => n.Driver || "—" },
      { label: t("subnet"), cls: "ct-cut mono ct-wide", cell: (n) => n.IPAM?.Config?.[0]?.Subnet ?? "—" },
      { label: t("gateway"), cls: "ct-cut mono ct-wide", cell: (n) => n.IPAM?.Config?.[0]?.Gateway ?? "—" },
      { label: t("scope"), cls: "ct-cut ct-wide", cell: (n) => n.Scope || "—" },
    ],
    create: {
      button: t("createNetwork"),
      title: t("createNetwork"),
      submit: t("create"),
      path: "",
      fields: [
        { key: "name", label: t("name"), placeholder: "proxy", initial: "", required: true },
        { key: "driver", label: t("driver"), placeholder: "bridge", initial: "bridge" },
      ],
      body: (v) => ({ name: v.name.trim(), driver: v.driver.trim() || "bridge" }),
    },
  }), [t]);
  return <PortainerResourceTab assetId={assetId} bar={bar} spec={spec} />;
}
