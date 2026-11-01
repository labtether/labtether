"use client";

import { Plus, Trash2, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import type { Asset } from "../../../console/models";
import { friendlyTypeLabel } from "../../../console/taxonomy";
import { Button } from "../../../components/ui/Button";
import { ConfirmDialog } from "../../../components/ui/ConfirmDialog";
import { Input } from "../../../components/ui/Input";
import type { PlacementSuggestion, TopologyState } from "./topologyCanvasTypes";
import type { MapWords } from "./useMapWords";

function PanelHead({ id, title, close, onClose }: { id: string; title: string; close: string; onClose: () => void }) {
  return (
    <div className="mp-ih">
      <h2 id={id}>{title}</h2>
      <button type="button" className="mp-x lg" onClick={onClose} aria-label={close}>
        <X aria-hidden="true" width={16} height={16} strokeWidth={2} />
      </button>
    </div>
  );
}

/** Zones (from the page ⋯): rename in place, delete, add. Devices pick their zone from the Inspector or their ⋯. */
export function ZonesPanel({ topology, words, focusZone, onRename, onDelete, onAdd, onClose }: {
  topology: TopologyState;
  words: MapWords;
  /** A zone just made: its name field takes focus with the text selected. */
  focusZone: string;
  onRename: (id: string, label: string) => void;
  onDelete: (id: string) => void;
  onAdd: () => void;
  onClose: () => void;
}) {
  const { t } = words;
  const [ask, setAsk] = useState<{ id: string; label: string; count: number } | null>(null);
  const zones = [...topology.zones].sort((a, b) => a.sort_order - b.sort_order);
  const count = (id: string) => topology.members.filter((m) => m.zone_id === id).length;
  const save = (id: string, before: string, value: string) => { const v = value.trim().slice(0, 48); if (v && v !== before) onRename(id, v); };
  return (
    <aside className="mp-insp" aria-labelledby="mp-zs-t">
      <PanelHead id="mp-zs-t" title={t("zones.title")} close={t("zones.close")} onClose={onClose} />
      <div className="mp-zrs">
        {zones.length === 0 && <p className="mp-none">{t("zones.none")}</p>}
        {zones.map((z) => (
          <div key={`${z.id}:${z.label}`} className="mp-zr">
            <Input
              defaultValue={z.label}
              maxLength={48}
              aria-label={t("zones.name", { zone: z.label })}
              autoComplete="off"
              autoFocus={z.id === focusZone}
              onFocus={(e) => { if (z.id === focusZone) e.target.select(); }}
              onBlur={(e) => save(z.id, z.label, e.target.value)}
              onKeyDown={(e) => { if (e.key === "Enter") (e.target as HTMLInputElement).blur(); }}
            />
            <button type="button" className="mp-x" onClick={() => setAsk({ id: z.id, label: z.label, count: count(z.id) })} aria-label={t("zones.delete", { zone: z.label })}>
              <Trash2 aria-hidden="true" width={15} height={15} strokeWidth={1.75} />
            </button>
          </div>
        ))}
      </div>
      <div>
        <Button size="sm" variant="ghost" className="mp-connect" onClick={onAdd}><Plus aria-hidden="true" width={14} height={14} />{t("zones.new")}</Button>
      </div>
      <ConfirmDialog
        open={!!ask}
        onClose={() => setAsk(null)}
        onConfirm={() => { if (ask) onDelete(ask.id); setAsk(null); }}
        title={t("zones.deleteTitle", { zone: ask?.label ?? "" })}
        message={t("zones.deleteBody", { count: ask?.count ?? 0 })}
        confirmLabel={t("zones.deleteConfirm")}
        danger
      />
    </aside>
  );
}

/** Devices the hub found that have no zone yet, with the hub's suggested zone, and the way to leave one off the map. */
export function UnsortedPanel({ ids, assets, words, onAccept, onDismiss, onPlaceAll, onClose }: {
  ids: string[];
  assets: Asset[];
  words: MapWords;
  onAccept: (id: string, zone: string) => Promise<void> | void;
  onDismiss: (id: string) => Promise<void> | void;
  onPlaceAll: () => Promise<void> | void;
  onClose: () => void;
}) {
  const { t } = words;
  const [suggestions, setSuggestions] = useState<PlacementSuggestion[]>([]);
  const [busy, setBusy] = useState<Set<string>>(new Set());
  const byId = useMemo(() => new Map(assets.map((a) => [a.id, a])), [assets]);
  const key = ids.join(",");

  useEffect(() => {
    let cancelled = false;
    fetch("/api/topology/unsorted", { cache: "no-store" })
      .then((r) => (r.ok ? r.json() : null))
      .then((json) => {
        const data = json?.data ?? json;
        if (!cancelled && Array.isArray(data?.suggestions)) setSuggestions(data.suggestions);
      })
      .catch(() => { /* suggestions are optional */ });
    return () => { cancelled = true; };
  }, [key]);

  const run = async (id: string, work: () => Promise<void> | void) => {
    setBusy((s) => new Set(s).add(id));
    try { await work(); } finally { setBusy((s) => { const n = new Set(s); n.delete(id); return n; }); }
  };

  return (
    <aside className="mp-insp" aria-labelledby="mp-us-t">
      <PanelHead id="mp-us-t" title={t("unsorted.title")} close={t("unsorted.close")} onClose={onClose} />
      {ids.length === 0 ? <p className="mp-none">{t("unsorted.none")}</p> : (
        <ul className="mp-us">
          {ids.map((id) => {
            const a = byId.get(id);
            const name = a?.name ?? id;
            const s = suggestions.find((x) => x.asset_id === id);
            return (
              <li key={id} className="mp-us-row">
                <span className="mp-us-who">
                  <span className="mp-us-nm">{name}</span>
                  <span className="mp-us-sub">{[a ? friendlyTypeLabel(a.type) : "", s?.zone_label ? t("unsorted.suggest", { zone: s.zone_label }) : ""].filter(Boolean).join(" · ")}</span>
                </span>
                {s?.zone_id && s.zone_label && (
                  <Button size="sm" disabled={busy.has(id)} onClick={() => run(id, () => onAccept(id, s.zone_id!))}>{t("unsorted.accept")}</Button>
                )}
                <button type="button" className="mp-x" disabled={busy.has(id)} onClick={() => run(id, () => onDismiss(id))} aria-label={t("unsorted.dismiss", { name })} title={t("unsorted.dismissTip")}>
                  <X aria-hidden="true" width={14} height={14} strokeWidth={2} />
                </button>
              </li>
            );
          })}
        </ul>
      )}
      {ids.length > 0 && (
        <div><Button size="sm" disabled={busy.has("*")} onClick={() => run("*", onPlaceAll)}>{t("unsorted.placeAll")}</Button></div>
      )}
    </aside>
  );
}
