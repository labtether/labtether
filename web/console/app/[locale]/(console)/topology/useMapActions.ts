"use client";

import { useCallback, useMemo } from "react";
import { useToast } from "../../../contexts/ToastContext";
import type { RelationshipType, TopologyConnection, TopologyState, Zone, ZoneMember } from "./topologyCanvasTypes";
import type { DisplayConnection } from "./topologyRelationshipGraph";
import type { useTopologyData } from "./useTopologyData";
import type { MapWords } from "./useMapWords";
import { copyText } from "../../../lib/clipboard";

type Data = ReturnType<typeof useTopologyData>;

/** The Map's changes to the stored topology. A failed save says so in a toast; nothing fails silently. */
export function useMapActions(data: Data, words: MapWords, nameOf: (id: string) => string) {
  const { t } = words;
  const { addToast } = useToast();
  const topology: TopologyState | null = data.topology;

  const run = useCallback(async <T,>(work: () => Promise<T>): Promise<T | undefined> => {
    try { return await work(); } catch { addToast("error", t("errors.save")); return undefined; }
  }, [addToast, t]);

  /** The zone a device is a member of in the stored topology ("" when it has none). */
  const zoneOf = useMemo(() => new Map((topology?.members ?? []).map((m) => [m.asset_id, m.zone_id])), [topology?.members]);

  /** Moves a device to a zone, or out of its zone with "". */
  const moveToZone = useCallback((assetID: string, zoneID: string) => run(async () => {
    if (!topology) return;
    const from = zoneOf.get(assetID) ?? "";
    if (from === zoneID) return;
    if (from) await data.setMembers(from, topology.members.filter((m) => m.zone_id === from && m.asset_id !== assetID));
    if (zoneID) {
      const there = topology.members.filter((m) => m.zone_id === zoneID);
      const added: ZoneMember = { zone_id: zoneID, asset_id: assetID, position: { x: 20, y: 20 + there.length * 50 }, sort_order: there.length };
      await data.setMembers(zoneID, [...there, added]);
    }
  }), [run, topology, zoneOf, data]);

  /** A new zone with a free name ("New zone", "New zone 2" …). */
  const newZone = useCallback(() => run(async (): Promise<Zone> => {
    const taken = new Set((topology?.zones ?? []).map((z) => z.label));
    let label = t("zones.newName");
    for (let n = 2; taken.has(label); n++) label = t("zones.newNameN", { n });
    return data.createZone({ label, color: "blue" });
  }), [run, topology?.zones, t, data]);

  const connect = useCallback((source: string, target: string, relationship: RelationshipType) =>
    run(() => data.createConnection({ source_asset_id: source, target_asset_id: target, relationship })), [run, data]);

  /** Removes a connection; the toast's Undo puts the same connection back. */
  const removeConnection = useCallback((c: DisplayConnection & { label: string }) => run(async () => {
    await data.deleteConnection(c.id);
    const restore: Partial<TopologyConnection> = { source_asset_id: c.source_asset_id, target_asset_id: c.target_asset_id, relationship: c.relationship, label: c.label };
    addToast("info", t("conn.removed", { a: nameOf(c.source_asset_id), b: nameOf(c.target_asset_id) }), 6000, {
      label: t("conn.undo"),
      onClick: () => { void run(() => data.createConnection(restore)); },
    });
  }), [run, data, addToast, t, nameOf]);

  const changeRel = useCallback((id: string, relationship: RelationshipType) => run(() => data.updateConnection(id, { relationship })), [run, data]);
  const setLabel = useCallback((id: string, label: string) => run(() => data.updateConnection(id, { label })), [run, data]);

  const copy = useCallback((text: string) => {
    void copyText(text).then(() => addToast("success", t("copied", { text })), () => addToast("error", t("errors.copy")));
  }, [addToast, t]);

  return {
    run, zoneOf, moveToZone, newZone, connect, removeConnection, changeRel, setLabel, copy,
    renameZone: (id: string, label: string) => run(() => data.updateZone(id, { label })),
    deleteZone: (id: string) => run(() => data.deleteZone(id)),
    acceptSuggestion: (id: string, zone: string) => moveToZone(id, zone),
    dismiss: (id: string) => run(() => data.dismissAsset(id)),
    placeAll: () => run(() => data.autoPlace()),
    reset: () => run(() => data.resetTopology()),
  };
}

export type MapActions = ReturnType<typeof useMapActions>;
