"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { Check, ChevronDown, TerminalSquare, X } from "lucide-react";
import { Link } from "../../../../i18n/navigation";
import type { Asset } from "../../../console/models";
import { friendlySourceLabel, friendlyTypeLabel } from "../../../console/taxonomy";
import { MapConnections, type ConnectionActions } from "./MapConnections";
import { MapCpu } from "./MapCpu";
import type { MapDevice } from "./mapModel";
import type { Zone } from "./topologyCanvasTypes";
import type { WorkloadSummary } from "./topologyHierarchy";
import type { DisplayConnection } from "./topologyRelationshipGraph";
import type { MapWords } from "./useMapWords";

type Props = ConnectionActions & {
  device: MapDevice;
  host?: MapDevice;
  assets: Asset[];
  connections: Array<DisplayConnection & { label: string; origin: string }>;
  workloads?: WorkloadSummary;
  zones: Zone[];
  /** The zone this device is a member of in the stored topology ("" when unsorted). */
  zone: string;
  canWrite: boolean;
  words: MapWords;
  onZone: (zone: string) => void;
  onSelect: (id: string) => void;
  onMap: (id: string) => boolean;
  onClose: () => void;
  onCopy: (text: string) => void;
  autoConnect?: boolean;
};

const NO_ZONE = "__none";
const COUNTS: Array<keyof WorkloadSummary> = ["vms", "containers", "services", "stacks", "pools", "datasets", "other"];

/** Right-hand panel for the selected device or guest: its state, where to go next, its facts and its connections. */
export function MapInspector(p: Props) {
  const { device: d, words } = p;
  const { t } = words;
  const meta = d.asset.metadata ?? {};
  const os = meta.os_pretty_name || meta.os_name || "";
  const live = d.asset.source === "agent" && d.tone === "ok" && p.canWrite;
  const contents = p.workloads ? COUNTS.filter((k) => p.workloads![k] > 0).map((k) => t(`contents.${k}`, { count: p.workloads![k] })).join(" · ") : "";
  const zoneLabel = p.zones.find((z) => z.id === p.zone)?.label ?? t("inspector.noZone");

  return (
    <aside className="mp-insp" aria-labelledby="mp-i-t">
      <div className="mp-ih">
        <h2 id="mp-i-t">{d.name}</h2>
        <button type="button" className="mp-x lg" onClick={p.onClose} aria-label={t("inspector.close")} title={t("inspector.closeTip")}>
          <X aria-hidden="true" width={16} height={16} strokeWidth={2} />
        </button>
      </div>
      {d.tone !== "ok" && (
        <p className={`mp-ist ${d.tone}`}><i className={`mp-gd ${d.tone}`} aria-hidden="true" />{words.long(d)}</p>
      )}
      <div className="mp-iacts">
        <Link href={`/nodes/${encodeURIComponent(d.id)}`} className="mp-btn">{t("inspector.open")}</Link>
        {live && (
          <Link href={`/terminal?target=${encodeURIComponent(d.id)}`} className="mp-btn">
            <TerminalSquare aria-hidden="true" width={14} height={14} />{t("inspector.terminal")}
          </Link>
        )}
      </div>
      <dl className="mp-facts">
        {d.address && (
          <div className="mp-kv">
            <dt>{t("inspector.address")}</dt>
            <dd><button type="button" className="mp-lnk mp-ip" onClick={() => p.onCopy(d.address)} title={t("inspector.copy")}>{d.address}</button></dd>
          </div>
        )}
        <div className="mp-kv"><dt>{os ? t("inspector.os") : t("inspector.type")}</dt><dd>{os || friendlyTypeLabel(d.asset.type)}</dd></div>
        <div className="mp-kv"><dt>{t("inspector.source")}</dt><dd>{friendlySourceLabel(d.asset.source)}</dd></div>
        {p.host ? (
          <div className="mp-kv">
            <dt>{t("inspector.runsOn")}</dt>
            <dd>
              {meta.vmid ? <span>{t("inspector.vmOn", { vmid: meta.vmid })} </span> : null}
              <button type="button" className="mp-lnk" onClick={() => p.onSelect(p.host!.id)}>{p.host.name}</button>
            </dd>
          </div>
        ) : (
          <div className="mp-kv">
            <dt>{t("inspector.zone")}</dt>
            <dd>
              <Menu.Root>
                <Menu.Trigger className="mp-rel" aria-label={t("inspector.changeZone", { zone: zoneLabel })}>
                  {zoneLabel}<ChevronDown aria-hidden="true" width={12} height={12} strokeWidth={2.2} />
                </Menu.Trigger>
                <Menu.Portal>
                  <Menu.Content className="lt-menu" align="start" sideOffset={4}>
                    <Menu.RadioGroup value={p.zone || NO_ZONE} onValueChange={(v) => p.onZone(v === NO_ZONE ? "" : v)}>
                      {p.zones.map((z) => (
                        <Menu.RadioItem key={z.id} value={z.id} className="lt-menu-item">
                          {z.label}
                          <Menu.ItemIndicator className="lt-menu-check"><Check aria-hidden="true" /></Menu.ItemIndicator>
                        </Menu.RadioItem>
                      ))}
                      {p.zones.length > 0 && <Menu.Separator className="lt-menu-sep" />}
                      <Menu.RadioItem value={NO_ZONE} className="lt-menu-item">
                        {t("inspector.noZone")}
                        <Menu.ItemIndicator className="lt-menu-check"><Check aria-hidden="true" /></Menu.ItemIndicator>
                      </Menu.RadioItem>
                    </Menu.RadioGroup>
                  </Menu.Content>
                </Menu.Portal>
              </Menu.Root>
            </dd>
          </div>
        )}
        {meta.uptime && <div className="mp-kv"><dt>{t("inspector.uptime")}</dt><dd>{meta.uptime}</dd></div>}
        {contents && <div className="mp-kv"><dt>{t("inspector.contents")}</dt><dd>{contents}</dd></div>}
      </dl>
      {d.tone === "ok" && <MapCpu id={d.id} words={words} />}
      <MapConnections
        asset={d.asset}
        connections={p.connections}
        assets={p.assets}
        onSelect={p.onSelect}
        onMap={p.onMap}
        words={words}
        autoConnect={p.autoConnect}
        onChangeRel={p.onChangeRel}
        onLabel={p.onLabel}
        onRemove={p.onRemove}
        onConnect={p.onConnect}
      />
    </aside>
  );
}
