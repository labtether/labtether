"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useRouter } from "../../../../i18n/navigation";
import { PageHeader } from "../../../components/PageHeader";
import { Button } from "../../../components/ui/Button";
import { ConfirmDialog } from "../../../components/ui/ConfirmDialog";
import { useAuth } from "../../../contexts/AuthContext";
import { hasWriteRole } from "../../../lib/roles";
import { assetFreshness } from "../nodes/nodesPageUtils";
import { MapCanvas } from "./MapCanvas";
import { MapEditStage } from "./MapEditStage";
import { MapFind } from "./MapFind";
import { MapInspector } from "./MapInspector";
import { layoutMap } from "./mapLayout";
import { MapList } from "./MapList";
import { NodeMenu, PageMenu } from "./MapMenus";
import { hitOf, netRoleOf, toneOf, type MapDevice } from "./mapModel";
import { UnsortedPanel, ZonesPanel } from "./MapPanels";
import { buildDisplayConnections } from "./topologyRelationshipGraph";
import { useMapActions } from "./useMapActions";
import { useMapGraph } from "./useMapGraph";
import { useMapWords } from "./useMapWords";

type Panel = "zones" | "unsorted" | null;
type View = "auto" | "list" | "map";

/**
 * Map (2.0): the lab as a network tree drawn from the hub's devices and their network links, fitted to the width.
 * Selecting a device opens its Inspector. The page ⋯ holds find, Edit layout (the free canvas with zones as boxes),
 * zones, unsorted devices, the list view and the reset. Phones show the tree as a list unless asked for the map.
 */
export default function MapPage() {
  const router = useRouter();
  const { user } = useAuth();
  const canWrite = hasWriteRole(user?.role);
  const map = useMapGraph();
  const { graph, assets, hierarchy, topology, isLoading, error, refresh, statusLoaded, linksFailed, retryLinks } = map;
  const words = useMapWords(graph);
  const { t } = words;
  const assetById = useMemo(() => new Map(assets.map((a) => [a.id, a])), [assets]);
  const nameOf = useCallback((id: string) => assetById.get(id)?.name ?? id, [assetById]);
  const actions = useMapActions(map, words, nameOf);

  const [editing, setEditing] = useState(false);
  const [view, setView] = useState<View>("auto");
  const [sel, setSel] = useState("");
  const [panel, setPanel] = useState<Panel>(null);
  const [focusZone, setFocusZone] = useState("");
  const [connectFor, setConnectFor] = useState("");
  const [find, setFind] = useState<string | null>(null);
  const [open, setOpen] = useState<ReadonlySet<string>>(new Set());
  const [fitKey, setFitKey] = useState(0);
  const [askReset, setAskReset] = useState(false);
  const [wrap, setWrap] = useState(1040);
  const [stage, setStage] = useState<HTMLDivElement | null>(null);

  const q = (find ?? "").trim().toLowerCase();
  const L = useMemo(() => layoutMap(graph, { q, open, wrap, labelText: words.label }), [graph, q, open, wrap, words.label]);

  // Nothing linked: the rows wrap to the stage width, in 80 px steps so a resize does not redraw on every pixel.
  useEffect(() => {
    if (!stage || typeof ResizeObserver !== "function") return;
    const ro = new ResizeObserver(() => { if (stage.clientWidth) setWrap(Math.max(400, Math.floor((stage.clientWidth - 64) / 80) * 80)); });
    ro.observe(stage);
    return () => ro.disconnect();
  }, [stage]);

  const select = useCallback((id: string | null) => { setSel(id ?? ""); setPanel(null); setConnectFor(""); }, []);
  const close = useCallback(() => { setSel(""); setPanel(null); setConnectFor(""); }, []);

  // Esc closes find, then the panel; ⌘F / Ctrl+F opens find on the map.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (askReset || document.querySelector("[role=dialog], [role=menu]")) return;
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "f" && !editing) { e.preventDefault(); setFind((f) => f ?? ""); return; }
      if (e.key !== "Escape") return;
      if (find !== null) setFind(null);
      else if (panel || sel) close();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [askReset, editing, find, panel, sel, close]);

  /** The selected device, or (Edit layout) any asset placed on the canvas. */
  const device = useMemo((): MapDevice | null => {
    if (!sel) return null;
    const d = graph.byId.get(sel); if (d) return d;
    const a = assetById.get(sel); if (!a) return null;
    const freshness = assetFreshness(a);
    const meta = a.metadata ?? {};
    return { id: a.id, name: a.name || a.id, asset: a, freshness, tone: toneOf(freshness), host: "", zone: "", net: netRoleOf(a), address: meta.ip_address || meta.ip || "" };
  }, [sel, graph.byId, assetById]);

  const connections = useMemo(() => {
    if (!device || !topology) return [];
    const stored = new Map(topology.connections.map((c) => [c.id, c]));
    return buildDisplayConnections(assets, topology.connections)
      .filter((c) => c.source_asset_id === device.id || c.target_asset_id === device.id)
      .map((c) => ({ ...c, label: stored.get(c.id)?.label ?? "", origin: stored.get(c.id)?.origin ?? "" }));
  }, [device, topology, assets]);

  const workloads = useMemo(() => hierarchy.entries.find((e) => e.host.id === sel)?.workloads, [hierarchy.entries, sel]);

  const newZone = async () => {
    const z = await actions.newZone();
    if (z) { setFocusZone(z.id); setPanel("zones"); setSel(""); }
  };

  const nodeMenu = (d: MapDevice) => (
    <NodeMenu
      d={d}
      zones={topology?.zones ?? []}
      zone={actions.zoneOf.get(d.id) ?? ""}
      words={words}
      onZone={(z) => { void actions.moveToZone(d.id, z); }}
      onOpen={() => router.push(`/nodes/${encodeURIComponent(d.id)}`)}
      onCopy={() => actions.copy(d.address)}
    />
  );

  const side = panel === "zones" && topology ? (
    <ZonesPanel
      topology={topology}
      words={words}
      focusZone={focusZone}
      onRename={(id, label) => { void actions.renameZone(id, label); }}
      onDelete={(id) => { void actions.deleteZone(id); }}
      onAdd={() => { void newZone(); }}
      onClose={close}
    />
  ) : panel === "unsorted" && topology ? (
    <UnsortedPanel
      ids={topology.unsorted}
      assets={assets}
      words={words}
      onAccept={async (id, zone) => { await actions.acceptSuggestion(id, zone); }}
      onDismiss={async (id) => { await actions.dismiss(id); }}
      onPlaceAll={async () => { await actions.placeAll(); }}
      onClose={close}
    />
  ) : device ? (
    <MapInspector
      key={`${device.id}:${connectFor === device.id ? "c" : ""}`}
      device={device}
      host={device.host ? graph.byId.get(device.host) : undefined}
      assets={assets}
      connections={connections}
      workloads={workloads}
      zones={topology?.zones ?? []}
      zone={actions.zoneOf.get(device.id) ?? ""}
      canWrite={canWrite}
      words={words}
      autoConnect={connectFor === device.id}
      onZone={(z) => { void actions.moveToZone(device.id, z); }}
      onSelect={(id) => select(id)}
      onMap={(id) => graph.byId.has(id) || editing}
      onClose={close}
      onCopy={actions.copy}
      onChangeRel={(id, rel) => { void actions.changeRel(id, rel); }}
      onLabel={(id, label) => { void actions.setLabel(id, label); }}
      onRemove={(c) => { void actions.removeConnection(c); }}
      onConnect={(to, rel) => { void actions.connect(device.id, to, rel); }}
    />
  ) : null;

  const firstHit = () => L.nodes.find((n) => hitOf(n.d, q))?.id ?? L.rows.find((r) => r.d && hitOf(r.d, q))?.id ?? "";
  const listing = view === "list";
  const header = (
    <PageHeader
      title={t("title")}
      action={(
        <div className="mp-head">
          {editing && <Button size="md" onClick={() => { setEditing(false); setFitKey((k) => k + 1); }}>{t("menu.doneEditing")}</Button>}
          <PageMenu
            words={words}
            editing={editing}
            listing={listing}
            unsorted={topology?.unsorted.length ?? 0}
            onFind={() => setFind((f) => f ?? "")}
            onEdit={() => { setEditing((e) => !e); setFind(null); setFitKey((k) => k + 1); }}
            onZones={() => { setFocusZone(""); setPanel("zones"); setSel(""); }}
            onNewZone={() => { void newZone(); }}
            onUnsorted={() => { setPanel("unsorted"); setSel(""); }}
            onToggleList={() => setView(listing ? "auto" : "list")}
            onReset={() => setAskReset(true)}
          />
        </div>
      )}
    />
  );

  const confirm = (
    <ConfirmDialog
      open={askReset}
      onClose={() => setAskReset(false)}
      onConfirm={() => { setAskReset(false); close(); void actions.reset(); }}
      title={t("reset.title")}
      message={t("reset.body")}
      confirmLabel={t("reset.confirm")}
      danger
    />
  );

  if (!statusLoaded && isLoading) return <>{header}<p className="mp-quiet" role="status">{t("loading")}</p></>;

  const empty = !graph.devices.length && !isLoading;
  return (
    <div className={`mp-wrap${view === "map" || editing ? " force" : ""}${listing && !editing ? " list" : ""}`}>
      {header}
      {error && !topology && (
        <p className="mp-err" role="alert">{t("errors.load")} <button type="button" className="mp-lnk" onClick={() => void refresh()}>{t("errors.retry")}</button></p>
      )}
      {linksFailed && !(error && !topology) && (
        <p className="mp-err" role="alert">{t("errors.links")} <button type="button" className="mp-lnk" onClick={retryLinks}>{t("errors.retry")}</button></p>
      )}
      {empty && !editing ? (
        <div className="mp-empty"><p>{t("empty")}</p><Link href="/nodes" className="mp-btn">{t("emptyLink")}</Link></div>
      ) : (
        <>
          {!editing && (
            <div className="mp-ctog" role="group" aria-label={t("view.label")}>
              <button type="button" aria-pressed={view !== "map"} onClick={() => setView("list")}>{t("view.list")}</button>
              <button type="button" aria-pressed={view === "map"} onClick={() => { setView("map"); setFitKey((k) => k + 1); }}>{t("view.map")}</button>
            </div>
          )}
          {find !== null && !editing && (
            <MapFind value={find} words={words} none={!!q && !firstHit()} onChange={setFind} onClose={() => setFind(null)} onPick={() => { const id = firstHit(); if (id) select(id); }} />
          )}
          <div ref={setStage} className={`mp-stage${side ? " insp" : ""}${editing ? " editing" : ""}`}>
            {editing && topology ? (
              <MapEditStage
                topology={topology}
                data={map}
                actions={actions}
                words={words}
                onSelect={select}
                onConnectFrom={(id) => { select(id); setConnectFor(id); }}
                onNewZone={() => { void newZone(); }}
                onReset={() => setAskReset(true)}
              />
            ) : (
              <div className="mp-cv">
                <MapCanvas
                  L={L} graph={graph} words={words} sel={sel} q={q} side={!!side} fitKey={fitKey}
                  onSelect={select} onClose={close}
                  onFold={(h) => setOpen((s) => { const n = new Set(s); if (n.has(h)) n.delete(h); else n.add(h); return n; })}
                  nodeMenu={nodeMenu}
                />
              </div>
            )}
            {side}
          </div>
          {!editing && <MapList graph={graph} words={words} q={q} />}
        </>
      )}
      {confirm}
    </div>
  );
}
