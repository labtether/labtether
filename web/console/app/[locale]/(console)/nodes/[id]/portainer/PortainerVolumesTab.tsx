"use client";

import { useTranslations } from "next-intl";
import { useMemo, type ReactNode } from "react";
import { PortainerResourceTab, type ResourceSpec } from "./PortainerResourceTab";

type DockerVolume = {
  Name: string;
  Driver: string;
  Mountpoint: string;
};

/** The endpoint's volumes: name, driver and mountpoint; Create volume and Remove…. */
export function PortainerVolumesTab({ assetId, bar }: { assetId: string; bar?: ReactNode }) {
  const t = useTranslations("devices-docker.portainer");
  const spec = useMemo<ResourceSpec<DockerVolume>>(() => ({
    kind: "volumes",
    parse: (data) => {
      const list = (data as { Volumes?: DockerVolume[] } | null)?.Volumes;
      return Array.isArray(list) ? list : [];
    },
    id: (v) => v.Name,
    name: (v) => v.Name,
    grid: "minmax(0, 1fr) 88px minmax(0, 1.2fr)",
    columns: [
      { label: t("name"), cls: "ct-cut mono", cell: (v) => v.Name },
      { label: t("driver"), cls: "ct-cut", cell: (v) => v.Driver || "—" },
      { label: t("mountpoint"), cls: "ct-cut mono ct-quiet", cell: (v) => v.Mountpoint || "—" },
    ],
    create: {
      button: t("createVolume"),
      title: t("createVolume"),
      submit: t("create"),
      path: "",
      fields: [
        { key: "name", label: t("name"), placeholder: "media-cache", initial: "", required: true },
        { key: "driver", label: t("driver"), placeholder: "local", initial: "local" },
      ],
      body: (v) => ({ name: v.name.trim(), driver: v.driver.trim() || "local" }),
    },
  }), [t]);
  return <PortainerResourceTab assetId={assetId} bar={bar} spec={spec} />;
}
