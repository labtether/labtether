"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { MoreHorizontal } from "lucide-react";
import { useState, type ReactNode } from "react";
import { Link } from "../../../i18n/navigation";
import { ConfirmDialog } from "../ui/ConfirmDialog";

export type MenuEntry =
  | { label: string; onSelect?: () => void; href?: string; danger?: boolean; hint?: string; icon?: ReactNode }
  | "sep";

/**
 * A ⋯ menu. In a row it shows on hover or focus (the row's CSS); size "ctl" is the header's 32 px button.
 * Items either run onSelect or open href; "sep" draws a line.
 */
export function RowMenu({ label, items, size = "row" }: { label: string; items: MenuEntry[]; size?: "row" | "ctl" }) {
  // A line only between two items: none first, last or twice.
  const list: MenuEntry[] = [];
  for (const item of items) {
    if (item === "sep" && (!list.length || list[list.length - 1] === "sep")) continue;
    list.push(item);
  }
  if (list[list.length - 1] === "sep") list.pop();
  if (!list.length) return null;
  return (
    <Menu.Root modal={false}>
      <Menu.Trigger className={`ct-more${size === "ctl" ? " ctl" : ""}`} aria-label={label} onClick={(e) => e.stopPropagation()}>
        <MoreHorizontal aria-hidden="true" strokeWidth={2} />
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content className="lt-menu" align="end" sideOffset={6} onClick={(e) => e.stopPropagation()}>
          {list.map((item, i) => {
            if (item === "sep") return <Menu.Separator key={`sep-${i}`} className="lt-menu-sep" />;
            const cls = `lt-menu-item${item.danger ? " ct-danger" : ""}`;
            const body = (
              <>
                {item.icon}
                {item.label}
                {item.hint ? <span className="lt-menu-hint">{item.hint}</span> : null}
              </>
            );
            return item.href ? (
              <Menu.Item key={item.label} className={cls} asChild>
                <Link href={item.href}>{body}</Link>
              </Menu.Item>
            ) : (
              <Menu.Item key={item.label} className={cls} onSelect={() => item.onSelect?.()}>
                {body}
              </Menu.Item>
            );
          })}
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}

export type Ask = { title: string; body: string; confirm: string; danger?: boolean; run: () => Promise<void> | void };

/** A confirm dialog for risky actions: ask({ title, body, confirm, run }) opens it; it closes once run settles. */
export function useConfirm() {
  const [ask, setAsk] = useState<Ask | null>(null);
  const [busy, setBusy] = useState(false);
  const dialog = (
    <ConfirmDialog
      open={ask !== null}
      onClose={() => { if (!busy) setAsk(null); }}
      onConfirm={() => {
        if (!ask) return;
        setBusy(true);
        void Promise.resolve(ask.run()).finally(() => { setBusy(false); setAsk(null); });
      }}
      title={ask?.title ?? ""}
      message={ask?.body ?? ""}
      confirmLabel={ask?.confirm}
      danger={ask?.danger}
      loading={busy}
    />
  );
  return [setAsk, dialog] as const;
}
