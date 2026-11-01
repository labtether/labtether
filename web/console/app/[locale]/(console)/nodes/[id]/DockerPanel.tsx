"use client";

import { useTranslations } from "next-intl";
import { Seg } from "../../../../components/containers/ContainersToolbar";
import { DockerContainersTab } from "./DockerContainersTab";
import { DockerImagesTab } from "./DockerImagesTab";
import { DockerStacksTab } from "./DockerStacksTab";

type DockerView = "stacks" | "containers" | "images";

/** A Docker host's tab: Stacks · Containers · Images in one switch, each view with its own buttons on the right. */
export function DockerPanel({ hostId, sub, onSub }: { hostId: string; sub: string | null; onSub: (sub: string) => void }) {
  const t = useTranslations("devices-docker.views");
  const view: DockerView = sub === "stacks" || sub === "images" ? sub : "containers";
  const bar = (
    <Seg<DockerView>
      value={view}
      items={[["stacks", t("stacks")], ["containers", t("containers")], ["images", t("images")]]}
      onChange={onSub}
      label={t("label")}
    />
  );
  return (
    <div className="ct-tab">
      {view === "stacks" ? <DockerStacksTab hostId={hostId} bar={bar} /> : null}
      {view === "containers" ? <DockerContainersTab hostId={hostId} bar={bar} /> : null}
      {view === "images" ? <DockerImagesTab hostId={hostId} bar={bar} /> : null}
    </div>
  );
}
