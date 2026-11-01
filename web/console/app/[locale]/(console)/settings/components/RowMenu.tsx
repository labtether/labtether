"use client";

import { Fragment } from "react";
import * as Menu from "@radix-ui/react-dropdown-menu";
import { MoreHorizontal } from "lucide-react";

export type RowMenuItem = {
  label: string;
  onSelect: () => void;
  danger?: boolean;
  disabled?: boolean;
  separatorBefore?: boolean;
};

/** A row's ⋯ menu (Radix, the shell's .lt-menu look). label names the row: "Actions for Gotify". */
export function RowMenu({ label, items }: { label: string; items: RowMenuItem[] }) {
  if (items.length === 0) return <span className="st-more-gap" aria-hidden="true" />;
  return (
    <Menu.Root>
      <Menu.Trigger className="st-more" aria-label={label}>
        <MoreHorizontal size={16} aria-hidden="true" />
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content className="lt-menu" align="end" sideOffset={4}>
          {items.map((item) => (
            <Fragment key={item.label}>
              {item.separatorBefore ? <Menu.Separator className="lt-menu-sep" /> : null}
              <Menu.Item
                className={`lt-menu-item${item.danger ? " st-danger" : ""}`}
                disabled={item.disabled}
                onSelect={item.onSelect}
              >
                {item.label}
              </Menu.Item>
            </Fragment>
          ))}
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}
