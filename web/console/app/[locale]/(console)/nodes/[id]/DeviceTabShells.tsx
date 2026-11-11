"use client";

import type { ReactNode } from "react";
import { useTranslations } from "next-intl";
import { formatBytes } from "../../../../console/formatters";
import type { Asset } from "../../../../console/models";
import { useAuth } from "../../../../contexts/AuthContext";
import { hasWriteRole } from "../../../../lib/roles";
import { Button } from "../../../../components/ui/Button";
import { DeviceViewSwitch } from "./DeviceTabBar";
import { NodeDeleteConfirmModal } from "./NodeDeleteConfirmModal";
import { useNodeDeleteFlow } from "./useNodeDeleteFlow";

/** Overview: usage, services and activity (and what the integration adds) with Details beside them. */
export function DeviceOverviewLayout({ main, details }: { main: ReactNode; details: ReactNode }) {
  return (
    <div className="dv-ov">
      <div className="dv-main">{main}</div>
      {details}
    </div>
  );
}

/** The facts the System tab names above its switch: kernel, CPU model and memory. */
function systemFacts(asset: Asset): string[] {
  const meta = asset.metadata ?? {};
  const mem = Number(meta.memory_total_bytes ?? "");
  return [meta.kernel_release?.trim() ?? "", meta.cpu_model?.trim() ?? "", Number.isFinite(mem) && mem > 0 ? formatBytes(mem) : ""];
}

/** System: kernel and CPU in one line, the view switch, then the open view. */
export function DeviceSystemTab({ asset, views, current, children }: { asset: Asset; views: string[]; current: string; children: ReactNode }) {
  const t = useTranslations("devices.page");
  const [kernel, cpu, mem] = systemFacts(asset);
  const facts = [kernel ? <span key="k" className="dv-mono">{kernel}</span> : null, cpu || null, mem || null].filter(Boolean);
  return (
    <div className="dv-tab">
      {facts.length ? (
        <p className="dv-facts">
          {facts.map((fact, i) => (
            <span key={i}>
              {i > 0 ? <span aria-hidden="true" className="dv-facts-sep">·</span> : null}
              {fact}
            </span>
          ))}
        </p>
      ) : null}
      {views.length > 1 ? (
        <div className="dv-bar">
          <DeviceViewSwitch nodeId={asset.id} views={views} current={current} label={t("views.systemLabel")} />
        </div>
      ) : null}
      {children}
    </div>
  );
}

/** Logs: stored and journal lines, or the action runs aimed at this device. */
export function DeviceLogsTab({ nodeId, views, current, children }: { nodeId: string; views: string[]; current: string; children: ReactNode }) {
  const t = useTranslations("devices.page");
  return (
    <div className="dv-tab">
      {views.length > 1 ? (
        <div className="dv-bar">
          <DeviceViewSwitch nodeId={nodeId} views={views} current={current} label={t("views.logsLabel")} />
        </div>
      ) : null}
      {children}
    </div>
  );
}

/** Settings: how the hub reaches the device (agent, collector, connections), then Remove. */
export function DeviceSettingsTab({ asset, children }: { asset: Asset; children: ReactNode }) {
  const t = useTranslations("devices.page.removeSection");
  const { user } = useAuth();
  const del = useNodeDeleteFlow({ nodeId: asset.id });
  return (
    <div className="dv-tab dv-set">
      {children}
      {hasWriteRole(user?.role) ? (
        <section className="dv-sec" aria-label={t("title", { name: asset.name })}>
          <div className="dv-sheet dv-rm">
            <div>
              <p className="dv-nm">{t("title", { name: asset.name })}</p>
              <p className="dv-hint">{t("hint")}</p>
            </div>
            <Button variant="danger" size="sm" onClick={del.openDeleteConfirm}>{t("button")}</Button>
          </div>
          <NodeDeleteConfirmModal
            open={del.showDeleteConfirm}
            assetId={asset.id}
            assetName={asset.name}
            agent={asset.source === "agent"}
            confirmInput={del.deleteConfirmInput}
            deleting={del.deleting}
            error={del.deleteError}
            onConfirmInputChange={del.setDeleteConfirmInput}
            onCancel={del.cancelDeleteConfirm}
            onConfirm={() => void del.confirmDelete()}
          />
        </section>
      ) : null}
    </div>
  );
}
