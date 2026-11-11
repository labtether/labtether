"use client";

import { Play, RotateCw } from "lucide-react";
import { useTranslations } from "next-intl";
import { useState } from "react";
import { ContainerMenu, useContainerAction } from "../../../../components/containers/ContainerMenu";
import type { MenuEntry } from "../../../../components/containers/RowMenu";
import { nodeHref, parseDockerStatus } from "../../../../components/containers/containerModel";
import type { Asset } from "../../../../console/models";
import { useAuth } from "../../../../contexts/AuthContext";
import { useSlowStatus, useStatusControls } from "../../../../contexts/StatusContext";
import { hasWriteRole } from "../../../../lib/roles";
import { NodeDeleteConfirmModal } from "./NodeDeleteConfirmModal";
import { NodeEditModal } from "./NodeEditModal";
import { useNodeDeleteFlow } from "./useNodeDeleteFlow";

/**
 * A Docker container's header actions: Restart (Start while it is stopped) and the containers' own ⋯, which adds
 * Edit… and Remove… for the device record.
 */
export function DeviceContainerActions({ asset }: { asset: Asset }) {
  const t = useTranslations("containers.menu");
  const tPage = useTranslations("devices.page.menu");
  const { user } = useAuth();
  const slowStatus = useSlowStatus();
  const { fetchStatus } = useStatusControls();
  const [editing, setEditing] = useState(false);
  const del = useNodeDeleteFlow({ nodeId: asset.id });
  const meta = asset.metadata ?? {};
  const state = parseDockerStatus(meta.state || asset.status, meta.status ?? "");
  const running = state.kind === "running";
  const busy = state.kind === "restarting" || state.kind === "removing";
  const target = { source: "docker" as const, assetId: asset.id };
  const refresh = () => void fetchStatus();
  const { run, pending } = useContainerAction(asset.name, target, refresh);
  const canWrite = hasWriteRole(user?.role);
  const extra: MenuEntry[] = canWrite
    ? [{ label: tPage("edit"), onSelect: () => setEditing(true) }, { label: tPage("remove"), danger: true, onSelect: del.openDeleteConfirm }]
    : [];

  return (
    <>
      {canWrite && !busy ? (
        <button type="button" className="dv-btn" disabled={pending !== null} onClick={() => void run(running ? "restart" : "start")}>
          {running ? <RotateCw aria-hidden="true" strokeWidth={1.75} /> : <Play aria-hidden="true" strokeWidth={1.75} />}
          {t(running ? "restart" : "start")}
        </button>
      ) : null}
      <ContainerMenu
        target={target}
        name={asset.name}
        image={meta.image ?? ""}
        href={nodeHref(asset.id)}
        running={running}
        busy={busy}
        onPage
        extra={extra}
        onChanged={refresh}
      />
      {editing ? <NodeEditModal asset={asset} groups={slowStatus?.groups ?? []} onClose={() => setEditing(false)} onSave={refresh} /> : null}
      <NodeDeleteConfirmModal
        open={del.showDeleteConfirm}
        assetId={asset.id}
        assetName={asset.name}
        confirmInput={del.deleteConfirmInput}
        deleting={del.deleting}
        error={del.deleteError}
        onConfirmInputChange={del.setDeleteConfirmInput}
        onCancel={del.cancelDeleteConfirm}
        onConfirm={() => void del.confirmDelete()}
      />
    </>
  );
}
