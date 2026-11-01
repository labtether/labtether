"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { Check, ChevronDown, Link2, X } from "lucide-react";
import { useState } from "react";
import type { Asset } from "../../../console/models";
import { Button } from "../../../components/ui/Button";
import { Input, Select } from "../../../components/ui/Input";
import { RELATIONSHIP_TYPES, type RelationshipType } from "./topologyCanvasTypes";
import type { DisplayConnection } from "./topologyRelationshipGraph";
import { inferRelationshipType } from "./topologySmartDefaults";
import type { MapWords } from "./useMapWords";

export type ConnectionActions = {
  onChangeRel: (id: string, rel: RelationshipType) => void;
  onLabel: (id: string, label: string) => void;
  onRemove: (c: DisplayConnection & { label: string }) => void;
  onConnect: (target: string, rel: RelationshipType) => void;
};

type Props = ConnectionActions & {
  asset: Asset;
  /** This device's connections, including the ones the hub infers from where a guest runs. */
  connections: Array<DisplayConnection & { label: string; origin: string }>;
  assets: Asset[];
  /** Selects a device that is on the Map; others show as plain names. */
  onSelect: (id: string) => void;
  onMap: (id: string) => boolean;
  words: MapWords;
  /** Opens with the connect form showing ("Connect to…" from a device's menu in Edit layout). */
  autoConnect?: boolean;
};

/** The Inspector's Connections: each one worded from this device's side, its relationship changed from a menu. */
export function MapConnections({ asset, connections, assets, onSelect, onMap, words, autoConnect, onChangeRel, onLabel, onRemove, onConnect }: Props) {
  const { t } = words;
  const [editing, setEditing] = useState("");
  const nameOf = (id: string) => assets.find((a) => a.id === id)?.name ?? id;
  const others = assets.filter((a) => a.id !== asset.id).sort((a, b) => a.name.localeCompare(b.name));
  const guess = (to: string): RelationshipType => inferRelationshipType(asset.type, assets.find((a) => a.id === to)?.type ?? "");
  const [form, setForm] = useState<{ to: string; rel: RelationshipType } | null>(() =>
    (autoConnect && others[0] ? { to: others[0].id, rel: guess(others[0].id) } : null));
  const word = (rel: string, out: boolean) => (t.has(`${out ? "rel" : "inv"}.${rel}`) ? t(`${out ? "rel" : "inv"}.${rel}`) : rel.replace(/_/g, " "));

  const openForm = () => { const to = others[0]?.id ?? ""; setForm({ to, rel: guess(to) }); };

  return (
    <section className="mp-sec" aria-labelledby="mp-i-conn">
      <h3 id="mp-i-conn" className="mp-sh">{t("conn.title")}</h3>
      {connections.length === 0 && <p className="mp-none">{t("conn.none")}</p>}
      {connections.map((c) => {
        const out = c.source_asset_id === asset.id;
        const other = out ? c.target_asset_id : c.source_asset_id;
        const name = nameOf(other);
        const rel = word(c.relationship, out);
        return (
          <div key={c.id} className="mp-lk">
            {c.inferred ? <span className="mp-rel static">{rel}</span> : (
              <Menu.Root>
                <Menu.Trigger className="mp-rel" aria-label={t("conn.change", { rel, name })}>
                  {rel}<ChevronDown aria-hidden="true" width={12} height={12} strokeWidth={2.2} />
                </Menu.Trigger>
                <Menu.Portal>
                  <Menu.Content className="lt-menu" align="start" sideOffset={4}>
                    <Menu.RadioGroup value={c.relationship} onValueChange={(v) => onChangeRel(c.id, v as RelationshipType)}>
                      {RELATIONSHIP_TYPES.map((r) => (
                        <Menu.RadioItem key={r.value} value={r.value} className="lt-menu-item">
                          {word(r.value, out)}
                          <Menu.ItemIndicator className="lt-menu-check"><Check aria-hidden="true" /></Menu.ItemIndicator>
                        </Menu.RadioItem>
                      ))}
                    </Menu.RadioGroup>
                    <Menu.Separator className="lt-menu-sep" />
                    <Menu.Item className="lt-menu-item" onSelect={() => setEditing(c.id)}>{t("conn.editLabel")}</Menu.Item>
                  </Menu.Content>
                </Menu.Portal>
              </Menu.Root>
            )}
            {onMap(other)
              ? <button type="button" className="mp-lnk" onClick={() => onSelect(other)}>{name}</button>
              : <span className="mp-lnk plain">{name}</span>}
            {editing === c.id ? (
              <Input
                className="mp-lbl-in"
                autoFocus
                defaultValue={c.label}
                aria-label={t("conn.label")}
                maxLength={48}
                onBlur={(e) => { if (e.target.value.trim() !== c.label) onLabel(c.id, e.target.value.trim()); setEditing(""); }}
                onKeyDown={(e) => {
                  if (e.key === "Enter") (e.target as HTMLInputElement).blur();
                  if (e.key === "Escape") { e.stopPropagation(); setEditing(""); }
                }}
              />
            ) : (
              <span className="mp-lk-note">{c.label || (c.origin === "discovered" ? t("conn.discovered") : "")}</span>
            )}
            {!c.inferred && (
              <button type="button" className="mp-x" onClick={() => onRemove(c)} aria-label={t("conn.remove", { name })} title={t("conn.removeShort")}>
                <X aria-hidden="true" width={14} height={14} strokeWidth={2} />
              </button>
            )}
          </div>
        );
      })}
      {form ? (
        <div className="mp-conn">
          <label className="mp-field">
            <span>{t("conn.device")}</span>
            <Select value={form.to} onChange={(e) => setForm({ to: e.target.value, rel: guess(e.target.value) })}>
              {others.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}
            </Select>
          </label>
          <label className="mp-field">
            <span>{t("conn.relationship")}</span>
            <Select value={form.rel} onChange={(e) => setForm({ ...form, rel: e.target.value as RelationshipType })}>
              {RELATIONSHIP_TYPES.map((r) => <option key={r.value} value={r.value}>{word(r.value, true)}</option>)}
            </Select>
          </label>
          <div className="mp-conn-acts">
            <Button size="sm" variant="ghost" onClick={() => setForm(null)}>{t("conn.cancel")}</Button>
            <Button size="sm" disabled={!form.to} onClick={() => { onConnect(form.to, form.rel); setForm(null); }}>{t("conn.connect")}</Button>
          </div>
        </div>
      ) : (
        <Button size="sm" variant="ghost" className="mp-connect" onClick={openForm} disabled={!others.length}>
          <Link2 aria-hidden="true" width={14} height={14} />{t("conn.connectTo")}
        </Button>
      )}
    </section>
  );
}
