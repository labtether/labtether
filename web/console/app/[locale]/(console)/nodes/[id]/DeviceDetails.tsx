"use client";

import { Copy } from "lucide-react";
import type { ReactNode } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link } from "../../../../../i18n/navigation";
import type { Asset, Group } from "../../../../console/models";
import { ContainsSummary } from "./ContainsSummary";
import { GroupPathBreadcrumb } from "./GroupPathBreadcrumb";
import { agentVersion, deviceAddress, deviceOS, deviceState, parseBackupAgeDays, shortWhen, uptimeParts } from "./deviceStatusModel";

type Props = {
  asset: Asset;
  nodeHasAgent: boolean;
  /** The host this device runs on (a Proxmox guest's node), when the hub knows it. */
  host: Asset | null;
  groups: Group[];
  /** A hypervisor's guests, for "5 of 6 running" with the ones in trouble named under it. */
  guests?: Asset[] | null;
};

function Row({ k, children }: { k: string; children: ReactNode }) {
  return (
    <div>
      <dt>{k}</dt>
      <dd>{children}</dd>
    </div>
  );
}

/** The device's facts beside Overview: address, OS, where it runs, group, what it contains, agent, uptime, tags. */
export function DeviceDetails({ asset, nodeHasAgent, host, groups, guests }: Props) {
  const t = useTranslations("devices.page.details");
  const tState = useTranslations("devices.page.state");
  const locale = useLocale();
  const meta = asset.metadata ?? {};
  const address = deviceAddress(asset);
  const os = deviceOS(asset);
  const state = deviceState(asset);
  const up = state.tone === "ok" ? uptimeParts(asset) : null;
  const backupDays = parseBackupAgeDays(meta.days_since_backup ?? "");
  const version = agentVersion(asset);
  const agentText = version || (nodeHasAgent ? t("connected") : asset.source === "agent" ? t("agentOffline") : t("none"));
  const guestStates = (guests ?? []).map((g) => ({ g, s: deviceState(g) }));
  const troubled = guestStates.filter(({ s }) => s.tone !== "ok");
  const vmid = meta.vmid?.trim();

  return (
    <aside className="dv-aside" aria-labelledby="dv-props">
      <div className="dv-sh"><h2 id="dv-props">{t("title")}</h2></div>
      <dl className="dv-sheet tight dv-kv dv-props">
        {address ? (
          <Row k={t("address")}>
            <span>{address}</span>
            <button type="button" className="dv-copy" aria-label={t("copyAddress")} onClick={() => void navigator.clipboard?.writeText(address)}>
              <Copy aria-hidden="true" />
            </button>
          </Row>
        ) : null}
        {os ? <Row k={t("os")}><span>{os}</span></Row> : null}
        {host ? (
          <Row k={t("runsOn")}>
            <Link className="dv-link" href={`/nodes/${encodeURIComponent(host.id)}`}>{host.name}</Link>
            {vmid ? <span className="dv-grey">{t("vmid", { vmid })}</span> : null}
          </Row>
        ) : null}
        {guestStates.length ? (
          <Row k={t("guests")}>
            <span className="dv-hold">
              <Link className="dv-link" href={`/nodes/${encodeURIComponent(asset.id)}?panel=compute`}>
                {t("running", { running: guestStates.length - troubled.length, total: guestStates.length })}
              </Link>
              {troubled.map(({ g, s }) => (
                <Link key={g.id} className={`dv-prob ${s.tone}`} href={`/nodes/${encodeURIComponent(g.id)}`}>
                  {g.name} · {tState(s.word)}
                </Link>
              ))}
            </span>
          </Row>
        ) : (
          <ContainsSummary assetID={asset.id} />
        )}
        <GroupPathBreadcrumb groupId={asset.group_id} groups={groups} />
        <Row k={t("agent")}><span className={version ? "num" : undefined}>{agentText}</span></Row>
        {up ? (
          <Row k={t("uptime")}>
            <span>{up.days > 0 ? t("days", { count: up.days }) : t("hoursMinutes", { hours: up.hours, minutes: up.minutes })}</span>
          </Row>
        ) : null}
        {backupDays != null ? <Row k={t("lastBackup")}><span>{t("backupAge", { count: Math.floor(backupDays) })}</span></Row> : null}
        {asset.tags?.length ? (
          <Row k={t("tags")}>{asset.tags.map((tag) => <span key={tag} className="dv-tag">{tag}</span>)}</Row>
        ) : null}
        {state.tone !== "ok" && state.since ? <Row k={t("lastSeen")}><span>{shortWhen(state.since, locale)}</span></Row> : null}
      </dl>
    </aside>
  );
}
