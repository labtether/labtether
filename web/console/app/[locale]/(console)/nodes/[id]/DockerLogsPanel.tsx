"use client";

import { useTranslations } from "next-intl";
import { DockerLogsTab } from "./DockerLogsTab";

/** The container's Logs tab in the page's language. */
export function DockerLogsPanel({ containerId }: { containerId: string }) {
  const t = useTranslations("devices-docker.logs");
  return (
    <DockerLogsTab
      containerId={containerId}
      words={{ lines: t("lines"), follow: t("follow"), refresh: t("refresh"), loading: t("loading"), failed: t("failed"), none: t("none"), label: t("label") }}
    />
  );
}
