"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "../../../../../components/ui/Button";
import { withCollector } from "../proxmoxFormatters";
import { DiPick } from "./DiControls";
import { DiEmpty, DiLog, DiNote, DiSection, DiSheet } from "./DiList";
import { useProxmoxFetch } from "./useProxmoxData";

type SyslogResponse = {
  lines?: string[];
  total?: number;
};

const LIMITS = ["100", "250", "500", "1000"] as const;
type Limit = (typeof LIMITS)[number];

type Props = {
  proxmoxNode: string;
  proxmoxCollectorID: string;
};

/** The node's syslog as Proxmox returns it, newest lines last. */
export function ProxmoxLogsTab({ proxmoxNode, proxmoxCollectorID }: Props) {
  const t = useTranslations("devices-proxmox.pve.syslog");
  const kit = useTranslations("devices-proxmox.kit");
  const [limit, setLimit] = useState<Limit>("250");
  const path = proxmoxNode
    ? withCollector(`/api/proxmox/nodes/${encodeURIComponent(proxmoxNode)}/syslog?limit=${limit}`, proxmoxCollectorID)
    : null;
  const { data, loading, error, refresh } = useProxmoxFetch<SyslogResponse>(path);
  const lines: string[] = data?.lines ?? (Array.isArray(data) ? (data as string[]) : []);

  return (
    <DiSection
      title={t("title")}
      right={
        <>
          <DiPick label={t("limitLabel")} options={LIMITS.map((id) => ({ id, label: t("last", { count: Number(id) }) }))} value={limit} onChange={setLimit} />
          <Button size="sm" onClick={refresh} disabled={loading}>{loading ? kit("loading") : kit("refresh")}</Button>
        </>
      }
    >
      {error ? <DiNote tone="bad">{t("loadFailed", { error })}</DiNote> : null}
      {lines.length > 0 ? (
        <DiSheet kind="flush">
          <DiLog label={t("title")}>{lines.join("\n")}</DiLog>
        </DiSheet>
      ) : (
        <DiSheet>
          <DiEmpty title={loading ? kit("loading") : t("none")} />
        </DiSheet>
      )}
    </DiSection>
  );
}
