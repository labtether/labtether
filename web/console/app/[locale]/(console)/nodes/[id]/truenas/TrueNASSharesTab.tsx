"use client";

// SMB and NFS shares in one list: the path, then the kind and its name, comment, networks and hosts in grey; a switch
// turns a share on or off. Delete sits behind ⋯ and asks first.

import { useTranslations } from "next-intl";
import { DiCols, DiDots, DiList, DiMain, DiNote, DiRow } from "../proxmox/DiList";
import { DiRowMenu, useDiConfirm } from "../proxmox/DiControls";
import { failureText } from "./truenasModel";
import { TnFailure, TnListState, TnSwitch } from "./TrueNASParts";
import type { TrueNASNFSShare, TrueNASSMBShare } from "./truenasTypes";
import { useTrueNASAction } from "./useTrueNASAction";
import { truenasAction, useTrueNASList } from "./useTrueNASData";

export type { TrueNASNFSShare, TrueNASSMBShare } from "./truenasTypes";

type Kind = "smb" | "nfs";
type ShareRow = { kind: Kind; id: string | number; path: string; enabled: boolean; grey: Array<string | undefined> };

export function TrueNASSharesTab({ assetId }: { assetId: string }) {
  const t = useTranslations("devices-truenas.shares");
  const k = useTranslations("devices-proxmox.kit");
  const smb = useTrueNASList<TrueNASSMBShare>(assetId, "shares/smb");
  const nfs = useTrueNASList<TrueNASNFSShare>(assetId, "shares/nfs");
  const action = useTrueNASAction();
  const { ask, dialog } = useDiConfirm();

  const kindWord = (kind: Kind) => (kind === "smb" ? t("smb") : t("nfs"));
  const path = (row: ShareRow) => `/api/truenas/assets/${encodeURIComponent(assetId)}/shares/${row.kind}/${encodeURIComponent(String(row.id))}`;
  const refreshKind = (kind: Kind) => (kind === "smb" ? smb.refresh() : nfs.refresh());

  const rows: ShareRow[] = [
    ...smb.data.map((share): ShareRow => ({
      kind: "smb", id: share.id, path: share.path, enabled: share.enabled, grey: [t("smb"), share.name, share.comment],
    })),
    ...nfs.data.map((share): ShareRow => ({
      kind: "nfs", id: share.id, path: share.path, enabled: share.enabled,
      grey: [
        t("nfs"),
        share.networks?.length ? t("networks", { list: share.networks.join(", ") }) : undefined,
        share.hosts?.length ? t("hosts", { list: share.hosts.join(", ") }) : undefined,
      ],
    })),
  ];

  const toggle = (row: ShareRow) => {
    void action.run(`toggle:${row.path}`, () => truenasAction(path(row), "PUT", { enabled: !row.enabled }), () => refreshKind(row.kind));
  };

  const remove = (row: ShareRow) => ask({
    title: t("deleteTitle", { kind: kindWord(row.kind), path: row.path }),
    message: t("deleteBody"),
    confirmLabel: t("deleteConfirm"),
    onConfirm: () => action.run(`delete:${row.path}`, () => truenasAction(path(row), "DELETE"), () => refreshKind(row.kind)),
  });

  const failure = action.error
    ? t(action.error.key.startsWith("toggle:") ? "toggleFailed" : "deleteFailed", {
        path: action.error.key.split(":").slice(1).join(":"),
        reason: failureText(action.error.message, t("noReason")),
      })
    : null;
  const listError = rows.length === 0 ? (smb.error ?? nfs.error) : null;

  return (
    <>
      {rows.length > 0 && smb.error ? <DiNote tone="bad">{t("smbFailed", { reason: failureText(smb.error, t("noReason")) })}</DiNote> : null}
      {rows.length > 0 && nfs.error ? <DiNote tone="bad">{t("nfsFailed", { reason: failureText(nfs.error, t("noReason")) })}</DiNote> : null}
      <TnFailure message={failure} />
      <DiList label={t("label")} cols="minmax(0, 1fr) 40px 28px" colsS="minmax(0, 1fr) 40px 28px">
        <DiCols>
          <span>{t("colPath")}</span>
          <span>{t("colOn")}</span>
          <span />
        </DiCols>
        <TnListState
          loading={smb.loading || nfs.loading}
          error={listError}
          count={rows.length}
          empty={t("empty")}
          onRetry={() => { smb.refresh(); nfs.refresh(); }}
          failed={(reason) => t("loadFailed", { reason })}
        />
        {rows.map((row) => (
          <DiRow key={`${row.kind}-${row.id}`}>
            <DiMain name={row.path} mono grey={<DiDots parts={row.grey} />} />
            <TnSwitch
              on={row.enabled}
              label={t("switch", { kind: kindWord(row.kind), path: row.path })}
              disabled={action.busy !== null}
              onToggle={() => toggle(row)}
            />
            <DiRowMenu
              label={k("actionsFor", { name: row.path })}
              items={[{ label: t("delete"), danger: true, disabled: action.busy !== null, onSelect: () => remove(row) }]}
            />
          </DiRow>
        ))}
      </DiList>
      {dialog}
    </>
  );
}
