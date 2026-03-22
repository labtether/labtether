"use client";

import { type ReactNode, useRef, useEffect, useState } from "react";

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

const sizeClasses: Record<NonNullable<SegmentedTabsProps<string>["size"]>, string> = {
  sm: "px-2 py-1 text-[10px]",
  md: "px-3 py-1.5 text-xs",
};

export function SegmentedTabs<T extends string>({
  value,
  options,
  onChange,
  size = "md",
  className = "",
}: SegmentedTabsProps<T>) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [indicator, setIndicator] = useState<{ left: number; width: number } | null>(null);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const buttons = container.querySelectorAll<HTMLButtonElement>("button");
    const idx = options.findIndex((o) => o.id === value);
    const btn = buttons[idx];
    if (btn) {
      setIndicator({ left: btn.offsetLeft, width: btn.offsetWidth });
    }
  }, [value, options]);

  return (
    <div
      ref={containerRef}
      className={`relative flex w-full max-w-full items-center gap-1 overflow-x-auto ${className}`}
    >
      {indicator && (
        <div
          className="absolute top-0 bottom-0 rounded-lg bg-[var(--control-bg-active)] transition-[left,width] duration-[var(--dur-normal)]"
          style={{ left: indicator.left, width: indicator.width, zIndex: 0 }}
        />
      )}
      {options.map((option) => {
        const selected = option.id === value;
        return (
          <button
            key={option.id}
            type="button"
            aria-label={option.ariaLabel}
            aria-pressed={selected}
            disabled={option.disabled}
            className={`relative z-10 rounded-lg font-medium transition-colors duration-[var(--dur-fast)] border border-transparent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--control-focus-ring)] disabled:opacity-50 disabled:pointer-events-none ${sizeClasses[size]} ${
              selected
                ? "text-[var(--control-fg-active)]"
                : "text-[var(--control-fg-muted)] hover:text-[var(--control-fg)] hover:bg-[var(--control-bg-hover)]"
            }`}
            onClick={() => onChange(option.id)}
          >
            {option.label}
          </button>
        );
      })}
    </div>
  );
}
