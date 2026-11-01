"use client";

// A native picker drawn as a 2.0 control: the menu stays the browser's, so it works with keys and on phones.

import type { ReactNode } from "react";
import { ChevronDown } from "lucide-react";

type Props = {
  label: string;
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  children: ReactNode;
};

export function PBSSelect({ label, value, onChange, disabled = false, children }: Props) {
  return (
    <span className="di-pbs-pick">
      <select aria-label={label} value={value} disabled={disabled} onChange={(event) => onChange(event.target.value)}>
        {children}
      </select>
      <ChevronDown aria-hidden="true" />
    </span>
  );
}
