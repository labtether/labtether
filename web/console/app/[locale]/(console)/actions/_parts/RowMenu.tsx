"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { MoreHorizontal } from "lucide-react";
import { Switch as KitSwitch } from "../../../../components/ui/Switch";

export type RowMenuItem =
  | { label: string; onSelect: () => void; danger?: boolean; disabled?: boolean; hint?: string }
  | { separator: true };

/** A row's ⋯ menu (the shell's menu look). label names the row for screen readers: "Actions for nas01 backup". */
export function RowMenu({ label, items }: { label: string; items: RowMenuItem[] }) {
  return (
    <Menu.Root modal={false}>
      <Menu.Trigger className="au-more" aria-label={label} onClick={(event) => event.stopPropagation()}>
        <MoreHorizontal aria-hidden="true" strokeWidth={2} />
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content className="lt-menu" align="end" sideOffset={4} onClick={(event) => event.stopPropagation()}>
          {items.map((item, index) => "separator" in item
            ? <Menu.Separator key={`sep-${index}`} className="lt-menu-sep" />
            : (
              <Menu.Item
                key={item.label}
                className={`lt-menu-item${item.danger ? " au-danger" : ""}`}
                disabled={item.disabled}
                onSelect={item.onSelect}
              >
                {item.label}
                {item.hint ? <span className="lt-menu-hint">{item.hint}</span> : null}
              </Menu.Item>
            ))}
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}

/** The kit's small on/off switch, for a row or a form; a click on it never reaches the row under it. */
export function Switch({
  on,
  label,
  onChange,
  disabled = false,
  labelledBy,
  id,
}: {
  on: boolean;
  label?: string;
  onChange: (next: boolean) => void;
  disabled?: boolean;
  labelledBy?: string;
  id?: string;
}) {
  return (
    <KitSwitch
      id={id}
      size="sm"
      checked={on}
      aria-label={labelledBy ? undefined : label}
      aria-labelledby={labelledBy}
      disabled={disabled}
      onClick={(event) => event.stopPropagation()}
      onChange={onChange}
    />
  );
}
