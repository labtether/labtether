"use client";

import type { ReactNode } from "react";

type SegmentedTabOption<T extends string> = {
  id: T;
  label: ReactNode;
  ariaLabel?: string;
  disabled?: boolean;
};

type SegmentedTabsProps<T extends string> = {
  value: T;
  options: SegmentedTabOption<T>[];
  onChange: (value: T) => void;
  size?: "sm" | "md";
  className?: string;
};

/** 2.0 segmented control (kit.css .kt-seg): a sunk track, the chosen option a raised thumb. 32 px, or 28 px small. */
export function SegmentedTabs<T extends string>({
  value,
  options,
  onChange,
  size = "md",
  className = "",
}: SegmentedTabsProps<T>) {
  return (
    <div role="group" className={`kt-seg${size === "sm" ? " sm" : ""}${className ? ` ${className}` : ""}`}>
      {options.map((option) => (
        <button
          key={option.id}
          type="button"
          aria-label={option.ariaLabel}
          aria-pressed={option.id === value}
          disabled={option.disabled}
          onClick={() => onChange(option.id)}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}
