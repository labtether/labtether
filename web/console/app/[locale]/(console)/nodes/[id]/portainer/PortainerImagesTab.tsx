"use client";

import { useTranslations } from "next-intl";
import { useMemo, type ReactNode } from "react";
import { formatMem } from "../../../../../components/containers/containerModel";
import { PortainerResourceTab, type ResourceSpec } from "./PortainerResourceTab";

type DockerImage = {
  Id: string;
  RepoTags?: string[];
  Size: number;
};

function shortId(id: string): string {
  return (id.startsWith("sha256:") ? id.slice(7) : id).slice(0, 12);
}

/** The endpoint's images: tags, id and size; Pull image… and Remove…. */
export function PortainerImagesTab({ assetId, bar }: { assetId: string; bar?: ReactNode }) {
  const t = useTranslations("devices-docker.portainer");
  const spec = useMemo<ResourceSpec<DockerImage>>(() => ({
    kind: "images",
    parse: (data) => (Array.isArray(data) ? (data as DockerImage[]) : []),
    id: (img) => img.Id,
    name: (img) => img.RepoTags?.[0] ?? shortId(img.Id),
    grid: "minmax(0, 1fr) 112px 72px",
    columns: [
      { label: t("image"), cls: "ct-cut mono", cell: (img) => (img.RepoTags?.length ? img.RepoTags.join(", ") : <span className="ct-quiet">{t("untagged")}</span>) },
      { label: t("id"), cls: "ct-cut mono ct-quiet", cell: (img) => shortId(img.Id) },
      { label: t("size"), cls: "ct-n", cell: (img) => formatMem(img.Size) },
    ],
    create: {
      button: t("pullImage"),
      title: t("pullImage"),
      submit: t("pull"),
      path: "pull",
      fields: [{ key: "image", label: t("image"), placeholder: "nginx:1.29", initial: "", required: true }],
      body: (v) => ({ image: v.image.trim() }),
    },
  }), [t]);
  return <PortainerResourceTab assetId={assetId} bar={bar} spec={spec} />;
}
