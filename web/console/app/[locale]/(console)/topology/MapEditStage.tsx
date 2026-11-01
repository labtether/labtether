"use client";

import dynamic from "next/dynamic";
import { ErrorBoundary } from "../../../components/ErrorBoundary";
import type { TopologyState } from "./topologyCanvasTypes";
import type { useTopologyData } from "./useTopologyData";
import type { MapActions } from "./useMapActions";
import type { MapWords } from "./useMapWords";

const TopologyCanvas = dynamic(() => import("./TopologyCanvas"), { ssr: false });

type Data = ReturnType<typeof useTopologyData>;

/**
 * Edit layout: the free canvas with zones as boxes. Drag zones, resize them, drag between two devices to connect
 * them, right-click for zone, device and connection actions. Positions and the view are saved on the hub.
 */
export function MapEditStage({ topology, data, actions, words, onSelect, onConnectFrom, onNewZone, onReset }: {
  topology: TopologyState;
  data: Data;
  actions: MapActions;
  words: MapWords;
  onSelect: (id: string | null) => void;
  /** "Connect to…" from a device's menu: its Inspector opens with the connect form. */
  onConnectFrom: (id: string) => void;
  onNewZone: () => void;
  /** Asks before the reset (the page's confirm dialog). */
  onReset: () => void;
}) {
  const { t } = words;
  const { run } = actions;
  return (
    <div className="mp-edit">
      <ErrorBoundary fallback={<p className="mp-none mp-pad">{t("errors.edit")}</p>}>
        <TopologyCanvas
          topology={topology}
          onViewportChange={data.saveViewport}
          onZoneLabelChange={(id, label) => { void actions.renameZone(id, label); }}
          onZoneToggleCollapse={(id) => {
            const zone = topology.zones.find((z) => z.id === id);
            if (zone) void run(() => data.updateZone(id, { collapsed: !zone.collapsed }));
          }}
          onZoneDelete={(id) => { void actions.deleteZone(id); }}
          onZoneMove={(id, x, y) => { void run(() => data.updateZone(id, { position: { x, y } })); }}
          onAssetSelect={onSelect}
          onConnectionSelect={(connID) => {
            const c = topology.connections.find((x) => x.id === connID);
            onSelect(c ? c.source_asset_id : null);
          }}
          onCreateConnection={(c) => { void actions.connect(c.source_asset_id, c.target_asset_id, c.relationship); }}
          onConnectTo={onConnectFrom}
          onCreateZone={onNewZone}
          onMoveToZone={(assetID, zoneID) => { void actions.moveToZone(assetID, zoneID); }}
          onRemoveFromZone={(assetID) => { void actions.moveToZone(assetID, ""); }}
          onChangeConnectionType={(id, rel) => { void actions.changeRel(id, rel); }}
          onDeleteConnection={(id) => {
            const c = topology.connections.find((x) => x.id === id);
            if (c) void actions.removeConnection({ ...c, inferred: false, label: c.label ?? "" });
          }}
          onResetLayout={onReset}
        />
      </ErrorBoundary>
    </div>
  );
}
