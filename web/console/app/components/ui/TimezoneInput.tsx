"use client";

import { useEffect, useId, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { Clock3 } from "lucide-react";
import { listEligibleTimezones } from "../../console/timezones";
import { Input } from "./Input";

type TimezoneInputProps = {
  value: string;
  onChange: (nextValue: string) => void;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  ariaLabel?: string;
};

const POPULAR_TIMEZONES = [
  "UTC",
  "America/New_York",
  "America/Chicago",
  "America/Denver",
  "America/Los_Angeles",
  "Europe/London",
  "Europe/Berlin",
  "Asia/Tokyo",
  "Australia/Sydney",
] as const;

const MAX_SUGGESTIONS = 10;

export function TimezoneInput({
  value,
  onChange,
  placeholder = "America/New_York",
  disabled = false,
  className = "",
  ariaLabel = "Timezone",
}: TimezoneInputProps) {
  const rootRef = useRef<HTMLDivElement>(null);
  const listboxID = useId();
  const allTimezones = useMemo(() => listEligibleTimezones(), []);
  const localTimezone = useMemo(() => {
    try {
      return Intl.DateTimeFormat().resolvedOptions().timeZone ?? "";
    } catch {
      return "";
    }
  }, []);

  const [open, setOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(-1);

  useEffect(() => {
    const onPointerDown = (event: PointerEvent) => {
      if (!rootRef.current) return;
      if (rootRef.current.contains(event.target as Node)) return;
      setOpen(false);
      setActiveIndex(-1);
    };

    document.addEventListener("pointerdown", onPointerDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
    };
  }, []);

  const suggestions = useMemo(() => {
    const query = value.trim().toLowerCase();

    if (!query) {
      const available = new Set(allTimezones);
      const initial: string[] = [];
      for (const candidate of [localTimezone, ...POPULAR_TIMEZONES]) {
        if (!candidate || !available.has(candidate) || initial.includes(candidate)) continue;
        initial.push(candidate);
      }
      return initial.slice(0, MAX_SUGGESTIONS);
    }

    const ranked = allTimezones
      .map((timezone) => ({ timezone, rank: timezoneMatchRank(timezone, query) }))
      .filter((entry) => entry.rank < 99)
      .sort((left, right) => {
        const rankDiff = left.rank - right.rank;
        if (rankDiff !== 0) return rankDiff;
        const lengthDiff = left.timezone.length - right.timezone.length;
        if (lengthDiff !== 0) return lengthDiff;
        return left.timezone.localeCompare(right.timezone);
      })
      .slice(0, MAX_SUGGESTIONS)
      .map((entry) => entry.timezone);

    return ranked;
  }, [allTimezones, localTimezone, value]);

  useEffect(() => {
    if (suggestions.length === 0) {
      setActiveIndex(-1);
      return;
    }
    setActiveIndex((current) => {
      if (current < 0) return 0;
      if (current >= suggestions.length) return suggestions.length - 1;
      return current;
    });
  }, [suggestions]);

  const commitSelection = (timezone: string) => {
    onChange(timezone);
    setOpen(false);
    setActiveIndex(-1);
  };

  const onInputKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (!open || suggestions.length === 0) {
      if (event.key === "ArrowDown" && suggestions.length > 0) {
        event.preventDefault();
        setOpen(true);
        setActiveIndex(0);
      }
      return;
    }

    if (event.key === "ArrowDown") {
      event.preventDefault();
      setActiveIndex((current) => Math.min(current + 1, suggestions.length - 1));
      return;
    }
    if (event.key === "ArrowUp") {
      event.preventDefault();
      setActiveIndex((current) => Math.max(current - 1, 0));
      return;
    }
    if (event.key === "Enter") {
      const picked = suggestions[activeIndex];
      if (picked) {
        event.preventDefault();
        commitSelection(picked);
      }
      return;
    }
    if (event.key === "Escape") {
      event.preventDefault();
      setOpen(false);
      setActiveIndex(-1);
    }
  };

  const hasSuggestions = open && suggestions.length > 0 && !disabled;

  return (
    <div ref={rootRef} className={`relative ${className}`}>
      <Input
        value={value}
        onChange={(event) => {
          onChange(event.target.value);
          setOpen(true);
        }}
        onFocus={() => {
          if (!disabled) setOpen(true);
        }}
        onKeyDown={onInputKeyDown}
        placeholder={placeholder}
        disabled={disabled}
        aria-label={ariaLabel}
        autoComplete="off"
        aria-haspopup="listbox"
        aria-expanded={hasSuggestions}
        aria-controls={hasSuggestions ? listboxID : undefined}
        className="pr-8"
      />
      <Clock3
        size={14}
        strokeWidth={1.75}
        className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 text-[var(--text-3)]"
      />

      {hasSuggestions ? (
        <ul id={listboxID} role="listbox" className="kt-tz-list">
          {suggestions.map((timezone, index) => {
            const isActive = index === activeIndex;
            return (
              <li key={timezone} role="option" aria-selected={isActive}>
                <button
                  type="button"
                  onMouseDown={(event) => {
                    event.preventDefault();
                  }}
                  onClick={() => {
                    commitSelection(timezone);
                  }}
                  className={`kt-tz-item${isActive ? " on" : ""}`}
                >
                  <span className="kt-tz-name">{timezone}</span>
                  {timezone === localTimezone ? <span className="kt-tag">local</span> : null}
                  <span className="kt-tz-hint">{timezoneHint(timezone)}</span>
                </button>
              </li>
            );
          })}
        </ul>
      ) : null}
    </div>
  );
}

function timezoneHint(timezone: string): string {
  const parts = timezone.split("/");
  if (parts.length <= 1) return "Standard timezone";
  const region = parts[0] ?? "";
  const city = parts.slice(1).join(" / ").replace(/_/g, " ");
  return `${region} / ${city}`;
}

function timezoneMatchRank(timezone: string, query: string): number {
  const lower = timezone.toLowerCase();
  const pretty = lower.replace(/_/g, " ");

  if (lower === query) return 0;
  if (lower.startsWith(query)) return 1;
  if (lower.includes(`/${query}`)) return 2;
  if (lower.includes(query)) return 3;
  if (pretty.includes(query)) return 4;
  return 99;
}
