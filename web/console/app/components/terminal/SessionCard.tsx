"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { MoreHorizontal } from "lucide-react";
import type { MouseEvent, ReactNode } from "react";

export type SessionRowAction = { label: string; onSelect: () => void; danger?: boolean } | "sep";

export type SessionCardProps = {
  name: string;
  /** Grey text after the name: the device, or user@host. */
  grey?: string;
  /** Right-hand status and time ("Detached 12 min ago"). */
  meta?: string;
  /** A status dot in the row's left margin. */
  tone?: "ok" | "warn" | "bad";
  /** The row's own action; newTab when Shift or the middle button was used. */
  onOpen: (newTab: boolean) => void;
  openLabel: string;
  moreLabel: string;
  actions: SessionRowAction[];
  /** Replaces the name while renaming. */
  editor?: ReactNode;
};

/** One running session, saved connection or ended session: a one-line row that opens it, and a ⋯ menu. */
export default function SessionCard({ name, grey, meta, tone, onOpen, openLabel, moreLabel, actions, editor }: SessionCardProps) {
  const open = (event: MouseEvent) => onOpen(event.shiftKey || event.button === 1);
  return (
    <div className="tm-row tm-srow">
      <span className="tm-lead" aria-hidden="true">{tone ? <i className={`kt-dot sm ${tone}`} /> : null}</span>
      {editor ?? (
        <button
          type="button"
          className="tm-open"
          title={openLabel}
          onClick={open}
          onAuxClick={(event) => {
            if (event.button === 1) {
              event.preventDefault();
              open(event);
            }
          }}
        >
          <span className="tm-nm">{name}</span>
          {grey ? <span className="tm-grey">{grey}</span> : null}
        </button>
      )}
      <span className="tm-meta">{meta}</span>
      <Menu.Root>
        <Menu.Trigger className="tm-more" aria-label={moreLabel}>
          <MoreHorizontal aria-hidden="true" />
        </Menu.Trigger>
        <Menu.Portal>
          <Menu.Content className="lt-menu" align="end" sideOffset={4}>
            {actions.map((action, index) =>
              action === "sep" ? (
                <Menu.Separator key={`sep-${index}`} className="lt-menu-sep" />
              ) : (
                <Menu.Item
                  key={action.label}
                  className={`lt-menu-item${action.danger ? " tm-danger" : ""}`}
                  onSelect={action.onSelect}
                >
                  {action.label}
                </Menu.Item>
              ),
            )}
          </Menu.Content>
        </Menu.Portal>
      </Menu.Root>
    </div>
  );
}
