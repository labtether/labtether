"use client";

import { Fragment, useState, type ReactNode } from "react";
import { useTranslations } from "next-intl";
import { Link } from "../../../../../i18n/navigation";
import { Select } from "../../../../components/ui/Input";
import type { Asset } from "../../../../console/models";
import { friendlyTypeLabel } from "../../../../console/taxonomy";
import { guestStatusTone, haStateTone, sourceBadgeLabel } from "./clusterTopologyUtils";
import type { AssetDependency, ClusterStatusEntry, HAResource } from "./clusterTopologyTypes";
import { DiCols, DiDots, DiList, DiMain, DiNote, DiRow, DiStatus } from "./proxmox/DiList";

type LinkEditorProps = {
  apiHosts: Asset[];
  draftTargetID: string;
  saving: boolean;
  canSave: boolean;
  linked: boolean;
  nameMatch: boolean;
  error?: string;
  onDraftChange: (targetID: string) => void;
  onSave: () => void;
  onClear: () => void;
  extra?: ReactNode;
};

/** Pick the host a guest runs as (from another integration), then Link, Update or Remove the link. */
export function ClusterGuestLinkEditor({
  apiHosts, draftTargetID, saving, canSave, linked, nameMatch, error, onDraftChange, onSave, onClear, extra,
}: LinkEditorProps) {
  const t = useTranslations("devices-proxmox.topology");
  return (
    <div className="di-ct-link">
      <Select
        value={draftTargetID}
        disabled={apiHosts.length === 0 || saving}
        onChange={(event) => { onDraftChange(event.target.value); }}
        aria-label={t("pickHost")}
      >
        <option value="">{apiHosts.length > 0 ? t("pickHost") : t("noHosts")}</option>
        {apiHosts.map((host) => (
          <option key={host.id} value={host.id}>
            {host.name} ({sourceBadgeLabel(host.source)})
          </option>
        ))}
      </Select>
      <div className="di-acts">
        {nameMatch ? <span className="di-grey">{t("nameMatch")}</span> : null}
        <button type="button" className="di-btn sm" disabled={saving || !canSave} onClick={onSave}>
          {saving ? t("saving") : linked ? t("update") : t("link")}
        </button>
        {linked ? (
          <button type="button" className="di-btn sm" disabled={saving} onClick={onClear}>{t("clear")}</button>
        ) : null}
        {extra}
      </div>
      {error ? <DiNote tone="bad">{error}</DiNote> : null}
    </div>
  );
}

type ClusterTopologyListViewProps = {
  nodeEntries: ClusterStatusEntry[];
  haByNode: Map<string, HAResource[]>;
  guestsByNode: Map<string, Asset[]>;
  guestRunsOn: Record<string, AssetDependency | null>;
  linkDrafts: Record<string, string>;
  linkErrors: Record<string, string>;
  savingGuestID: string | null;
  autoLinkingGuestID: string | null;
  loadingGuestLinks: boolean;
  apiHosts: Asset[];
  apiHostsByID: Map<string, Asset>;
  suggestedHostForGuest: (guest: Asset) => Asset | undefined;
  onDraftChange: (guestID: string, targetID: string) => void;
  onSaveGuestLink: (guest: Asset) => void;
  onClearGuestLink: (guest: Asset) => void;
};

/** Each node, its guests under it (a guest opens its link editor) and its HA resources. */
export function ClusterTopologyListView({
  nodeEntries,
  haByNode,
  guestsByNode,
  guestRunsOn,
  linkDrafts,
  linkErrors,
  savingGuestID,
  autoLinkingGuestID,
  loadingGuestLinks,
  apiHosts,
  apiHostsByID,
  suggestedHostForGuest,
  onDraftChange,
  onSaveGuestLink,
  onClearGuestLink,
}: ClusterTopologyListViewProps) {
  const t = useTranslations("devices-proxmox.topology");
  const [open, setOpen] = useState<string | null>(null);
  const linking = loadingGuestLinks || autoLinkingGuestID != null;

  return (
    <>
      {linking ? <DiNote>{t("linking")}</DiNote> : null}
      <DiList cols="minmax(0, 1fr) minmax(0, 220px) 96px" colsS="minmax(0, 1fr) auto" label={t("title")}>
        <DiCols>
          <span>{t("nodeOrGuest")}</span>
          <span className="di-s">{t("runsAs")}</span>
          <span className="r">{t("state")}</span>
        </DiCols>
        {nodeEntries.map((node) => {
          const online = node.online === 1;
          const guests = guestsByNode.get(node.name ?? "") ?? [];
          const ha = haByNode.get(node.name ?? "") ?? [];
          return (
            <Fragment key={node.nodeid ?? node.name}>
              <DiRow tone={online ? "" : "bad"}>
                <DiMain name={node.name ?? "—"} grey={<DiDots parts={[node.local === 1 ? t("local") : null, node.ip || null]} />} />
                <span className="di-s" />
                {online ? <span className="di-num">{t("online")}</span> : <DiStatus tone="bad" word={t("offline")} />}
              </DiRow>
              {guests.map((guest) => {
                const status = (guest.metadata?.status ?? guest.status ?? "").toLowerCase();
                const tone = guestStatusTone(status);
                const mapping = guestRunsOn[guest.id] ?? null;
                const linkedHost = mapping ? apiHostsByID.get(mapping.target_asset_id) : undefined;
                const draft = linkDrafts[guest.id] ?? "";
                const saving = savingGuestID === guest.id || autoLinkingGuestID === guest.id;
                const suggestion = suggestedHostForGuest(guest);
                const isOpen = open === guest.id;
                return (
                  <DiRow
                    key={guest.id}
                    className="di-ct-guest"
                    tone={tone === "bad" ? "bad" : ""}
                    onOpen={() => setOpen(isOpen ? null : guest.id)}
                    selected={isOpen}
                    after={isOpen ? (
                      <div className="di-panel">
                        <ClusterGuestLinkEditor
                          apiHosts={apiHosts}
                          draftTargetID={draft}
                          saving={saving}
                          canSave={draft !== "" && (!mapping || mapping.target_asset_id !== draft)}
                          linked={!!mapping}
                          nameMatch={!mapping && !!suggestion && draft === suggestion.id}
                          error={linkErrors[guest.id]}
                          onDraftChange={(targetID) => onDraftChange(guest.id, targetID)}
                          onSave={() => onSaveGuestLink(guest)}
                          onClear={() => onClearGuestLink(guest)}
                        />
                      </div>
                    ) : null}
                  >
                    <DiMain
                      name={guest.name}
                      grey={<DiDots parts={[friendlyTypeLabel(guest.type), mapping?.metadata?.binding === "auto" ? t("auto") : null]} />}
                    />
                    <span className="di-s di-ct-host">
                      {linkedHost ? (
                        <Link href={`/nodes/${encodeURIComponent(linkedHost.id)}`} className="di-ct-hostlink">
                          {linkedHost.name} · {sourceBadgeLabel(linkedHost.source)}
                        </Link>
                      ) : mapping ? <span className="di-bad">{t("targetMissing")}</span> : <span className="di-grey">—</span>}
                    </span>
                    <span className={`di-num${tone === "off" ? " none" : ""}`}>{status || "—"}</span>
                  </DiRow>
                );
              })}
              {ha.map((resource, index) => {
                const tone = haStateTone(resource.state);
                return (
                  <DiRow key={resource.sid ?? index} className="di-ct-guest" tone={tone === "bad" || tone === "warn" ? tone : ""}>
                    <DiMain name={resource.sid ?? "—"} mono grey={<DiDots parts={[t("ha"), resource.group || null]} />} />
                    <span className="di-s" />
                    <span className={`di-num${tone === "off" ? " none" : ""}`}>{resource.state ?? "—"}</span>
                  </DiRow>
                );
              })}
            </Fragment>
          );
        })}
      </DiList>
    </>
  );
}
