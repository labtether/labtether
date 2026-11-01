"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { MoreHorizontal } from "lucide-react";
import type { MapDevice } from "./mapModel";
import type { Zone } from "./topologyCanvasTypes";
import type { MapWords } from "./useMapWords";

export type PageMenuActions = {
  onFind: () => void;
  onEdit: () => void;
  onZones: () => void;
  onNewZone: () => void;
  onUnsorted: () => void;
  onToggleList: () => void;
  onReset: () => void;
};

/** The page ⋯: find, the free layout editor, zones, unsorted devices, the list view and the reset. */
export function PageMenu({ words, editing, listing, unsorted, ...a }: PageMenuActions & {
  words: MapWords; editing: boolean; listing: boolean; unsorted: number;
}) {
  const { t } = words;
  return (
    <Menu.Root>
      <Menu.Trigger className="lt-icon-btn" aria-label={t("more")}>
        <MoreHorizontal aria-hidden="true" strokeWidth={2} />
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content className="lt-menu" align="end" sideOffset={6}>
          {!editing && <Menu.Item className="lt-menu-item" onSelect={a.onFind}>{t("menu.find")}</Menu.Item>}
          <Menu.Item className="lt-menu-item" onSelect={a.onEdit}>{t(editing ? "menu.doneEditing" : "menu.editLayout")}</Menu.Item>
          {!editing && <Menu.Item className="lt-menu-item" onSelect={a.onToggleList}>{t(listing ? "menu.showMap" : "menu.showList")}</Menu.Item>}
          <Menu.Separator className="lt-menu-sep" />
          <Menu.Item className="lt-menu-item" onSelect={a.onZones}>{t("menu.zones")}</Menu.Item>
          <Menu.Item className="lt-menu-item" onSelect={a.onNewZone}>{t("menu.newZone")}</Menu.Item>
          <Menu.Item className="lt-menu-item" onSelect={a.onUnsorted}>
            {t("menu.unsorted")}{unsorted > 0 && <span className="lt-menu-hint">{unsorted}</span>}
          </Menu.Item>
          <Menu.Separator className="lt-menu-sep" />
          <Menu.Item className="lt-menu-item mp-danger" onSelect={a.onReset}>{t("menu.reset")}</Menu.Item>
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}

/** A node's ⋯: its zone, its page and its address. */
export function NodeMenu({ d, zones, zone, words, onZone, onOpen, onCopy }: {
  d: MapDevice; zones: Zone[]; zone: string; words: MapWords;
  onZone: (zone: string) => void; onOpen: () => void; onCopy: () => void;
}) {
  const { t } = words;
  const current = zones.find((z) => z.id === zone);
  return (
    <Menu.Root>
      <Menu.Trigger className="mp-dots" aria-label={t("node.actions", { name: d.name })}>
        <MoreHorizontal aria-hidden="true" width={14} height={14} strokeWidth={2} />
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content className="lt-menu" align="start" sideOffset={4}>
          {zones.filter((z) => z.id !== zone).map((z) => (
            <Menu.Item key={z.id} className="lt-menu-item" onSelect={() => onZone(z.id)}>{t("node.moveTo", { zone: z.label })}</Menu.Item>
          ))}
          {current && <Menu.Item className="lt-menu-item" onSelect={() => onZone("")}>{t("node.removeFrom", { zone: current.label })}</Menu.Item>}
          {zones.length > 0 && <Menu.Separator className="lt-menu-sep" />}
          <Menu.Item className="lt-menu-item" onSelect={onOpen}>{t("node.open")}</Menu.Item>
          {d.address && <Menu.Item className="lt-menu-item" onSelect={onCopy}>{t("node.copy")}</Menu.Item>}
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}
