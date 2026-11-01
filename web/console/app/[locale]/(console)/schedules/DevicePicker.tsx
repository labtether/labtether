"use client";

import { X } from "lucide-react";
import type { Asset } from "../../../console/models";
import { Select } from "../../../components/ui/Input";

/**
 * Devices a schedule runs on: the picked ones as small tags with a remove button, then a menu to add another. A picked
 * id the hub no longer reports stays as its id so saving does not drop it silently.
 */
export function DevicePicker({
  id,
  value,
  devices,
  addLabel,
  removeLabel,
  onChange,
  describedBy,
}: {
  id: string;
  value: string[];
  devices: Asset[];
  addLabel: string;
  removeLabel: (name: string) => string;
  onChange: (next: string[]) => void;
  describedBy?: string;
}) {
  const name = (target: string) => devices.find((d) => d.id === target)?.name || target;
  const left = devices.filter((d) => !value.includes(d.id));

  return (
    <div className="au-pick">
      {value.length ? (
        <ul className="au-tags">
          {value.map((target) => (
            <li key={target} className="au-tag">
              {name(target)}
              <button type="button" aria-label={removeLabel(name(target))} onClick={() => onChange(value.filter((v) => v !== target))}>
                <X aria-hidden="true" />
              </button>
            </li>
          ))}
        </ul>
      ) : null}
      <Select
        className="w-full"
        id={id}
        value=""
        aria-describedby={describedBy}
        disabled={left.length === 0}
        onChange={(event) => { if (event.target.value) onChange([...value, event.target.value]); }}
      >
        <option value="">{addLabel}</option>
        {left.map((d) => <option key={d.id} value={d.id}>{d.name || d.id}</option>)}
      </Select>
    </div>
  );
}
