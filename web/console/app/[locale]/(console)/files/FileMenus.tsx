"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { MoreHorizontal } from "lucide-react";
import type { ReactNode } from "react";
import { createPortal } from "react-dom";
import { useTranslations } from "next-intl";
import type { MenuAction, MenuSlot } from "./filesListModel";

// Menus for the file list: one item shape, the row ⋯ button, the right-click menu (a Radix menu anchored where the
// pointer was) and the labels for each action.

export type MenuItemSpec =
  | { key: string; label: string; onSelect: () => void; danger?: boolean; disabled?: boolean; hint?: string }
  | { key: string; sep: true };

export function MenuItems({ items }: { items: MenuItemSpec[] }) {
  return (
    <>
      {items.map((it) =>
        "sep" in it ? (
          <Menu.Separator key={it.key} className="lt-menu-sep" />
        ) : (
          <Menu.Item
            key={it.key}
            className={`lt-menu-item${it.danger ? " fx-danger" : ""}`}
            disabled={it.disabled}
            onSelect={it.onSelect}
          >
            {it.label}
            {it.hint ? <span className="lt-menu-hint">{it.hint}</span> : null}
          </Menu.Item>
        ),
      )}
    </>
  );
}

/** The ⋯ button at the end of a row. */
export function RowMenu({ label, items }: { label: string; items: () => MenuItemSpec[] }) {
  return (
    <Menu.Root modal={false}>
      <Menu.Trigger className="fx-more-b" aria-label={label} title={label} onClick={(e) => e.stopPropagation()} onDoubleClick={(e) => e.stopPropagation()}>
        <MoreHorizontal aria-hidden="true" />
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content className="lt-menu fx-menu" align="end" sideOffset={4} onClick={(e) => e.stopPropagation()}>
          <MenuItems items={items()} />
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}

/** A toolbar menu with its own trigger (the ⋯ More button, New connection, the second pane's source). */
export function ToolMenu({ trigger, label, children, align = "end" }: { trigger: ReactNode; label: string; children: ReactNode; align?: "start" | "end" }) {
  return (
    <Menu.Root modal={false}>
      <Menu.Trigger asChild aria-label={label}>{trigger}</Menu.Trigger>
      <Menu.Portal>
        <Menu.Content className="lt-menu fx-menu" align={align} sideOffset={6}>
          {children}
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}

/** The right-click menu, opened at the pointer (viewport coordinates); closes on Escape, outside clicks and a pick. */
export function PointMenu({ at, label, items, onClose }: {
  at: { x: number; y: number } | null;
  label: string;
  items: MenuItemSpec[];
  onClose: () => void;
}) {
  return (
    <Menu.Root open={at !== null} onOpenChange={(open) => { if (!open) onClose(); }} modal={false}>
      {/* The anchor lives on <body>: a page container (container queries, transforms) would move a fixed child. */}
      {at
        ? createPortal(
          <Menu.Trigger asChild>
            <span aria-hidden="true" className="fx-anchor" style={{ left: at.x, top: at.y }} />
          </Menu.Trigger>,
          document.body,
        )
        : null}
      <Menu.Portal>
        <Menu.Content
          className="lt-menu fx-menu"
          align="start"
          side="bottom"
          sideOffset={2}
          collisionPadding={8}
          aria-label={label}
          aria-labelledby={undefined}
          onCloseAutoFocus={(e) => e.preventDefault()}
        >
          <MenuItems items={items} />
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}

export type MenuLabelInput = { count: number; name: string; dir: boolean };

/** The words for each item action, counted when it acts on several items. */
export function useMenuLabels(): (action: MenuAction, input: MenuLabelInput) => string {
  const t = useTranslations("files.menu");
  return (action, { count, name, dir }) => {
    const many = count > 1;
    switch (action) {
      case "download": return many ? t("downloadMany", { count }) : t("download");
      case "copy": return many ? t("copyMany", { count }) : t("copy");
      case "cut": return many ? t("cutMany", { count }) : t("cut");
      case "delete": return many ? t("deleteMany", { count }) : t("delete");
      case "paste": return dir && name ? t("pasteInto", { name }) : t("paste");
      default: return t(action);
    }
  };
}

/** Turns the model's actions into menu items; actions without a handler are left out. */
export function toMenuItems(
  slots: MenuSlot[],
  handlers: Partial<Record<MenuAction, () => void>>,
  label: (action: MenuAction) => string,
): MenuItemSpec[] {
  const out: MenuItemSpec[] = [];
  slots.forEach((slot, i) => {
    if (slot === "sep") {
      if (out.length && !("sep" in out[out.length - 1])) out.push({ key: `sep-${i}`, sep: true });
      return;
    }
    const run = handlers[slot];
    if (run) out.push({ key: slot, label: label(slot), onSelect: run, danger: slot === "delete" });
  });
  while (out.length && "sep" in out[out.length - 1]) out.pop();
  return out;
}
