"use client";

import { useLocale, useTranslations } from "next-intl";
import { Button } from "../../../../../components/ui/Button";
import { withCollector } from "../proxmoxFormatters";
import { DiCols, DiDots, DiEmpty, DiList, DiMain, DiNote, DiRow, DiSection, DiStatus, DiTime } from "./DiList";
import { spanText, whenText } from "./diFormat";
import { useProxmoxList } from "./useProxmoxData";

type ReplicationJob = {
  id?: string;
  type?: string;
  target?: string;
  schedule?: string;
  enabled?: boolean | number;
  state?: string;
  last_sync?: number;
  next_sync?: number;
  duration?: number;
  fail_count?: number;
  comment?: string;
  vmid?: number;
};

type Props = {
  proxmoxNode: string;
  proxmoxCollectorID: string;
};

/** Storage replication jobs on the node: guest and target, schedule, last sync and failures. */
export function ProxmoxReplicationTab({ proxmoxNode, proxmoxCollectorID }: Props) {
  const t = useTranslations("devices-proxmox.pve.replication");
  const kit = useTranslations("devices-proxmox.kit");
  const locale = useLocale();
  const path = proxmoxNode
    ? withCollector(`/api/proxmox/nodes/${encodeURIComponent(proxmoxNode)}/replication`, proxmoxCollectorID)
    : null;
  const { data: jobs, loading, error, refresh } = useProxmoxList<ReplicationJob>(path);

  return (
    <DiSection
      title={t("title")}
      right={<Button size="sm" onClick={refresh} disabled={loading}>{loading ? kit("loading") : kit("refresh")}</Button>}
    >
      {error ? <DiNote tone="bad">{t("loadFailed", { error })}</DiNote> : null}
      <DiList cols="minmax(0, 1fr) 120px 96px" colsS="minmax(0, 1fr) auto" label={t("title")}>
        <DiCols>
          <span>{t("job")}</span>
          <span className="di-s">{t("state")}</span>
          <span className="r">{t("lastSync")}</span>
        </DiCols>
        {jobs.length === 0 ? <DiEmpty title={loading ? kit("loading") : t("none")} /> : null}
        {jobs.map((job, index) => {
          const enabled = job.enabled === true || job.enabled === 1;
          const failed = (job.state ?? "").toLowerCase() === "error" || (job.fail_count ?? 0) > 0;
          const guest = job.vmid != null ? t("guest", { vmid: job.vmid }) : job.id || "—";
          return (
            <DiRow key={job.id ?? index} tone={failed ? "bad" : enabled ? "" : "off"}>
              <DiMain
                name={job.target ? t("toTarget", { guest, target: job.target }) : guest}
                grey={
                  <DiDots
                    parts={[
                      job.schedule ? t("schedule", { schedule: job.schedule }) : null,
                      job.type || null,
                      job.duration ? spanText(job.duration) : null,
                      job.next_sync ? t("next", { when: whenText(job.next_sync, locale) }) : null,
                      enabled ? null : kit("off"),
                      job.comment || null,
                    ]}
                  />
                }
              />
              <span className="di-s">
                {failed
                  ? <DiStatus tone="bad" word={job.fail_count ? t("failedTimes", { count: job.fail_count }) : t("failed")} />
                  : <span className="di-word">{job.state || "—"}</span>}
              </span>
              <DiTime>{whenText(job.last_sync, locale) || kit("never")}</DiTime>
            </DiRow>
          );
        })}
      </DiList>
    </DiSection>
  );
}
