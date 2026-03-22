"use client";

import { useCallback, useEffect, useMemo, useState } from "react";

import { Select } from "./ui/Input";

export interface DisplayInfo {
  name: string;
  width: number;
  height: number;
  primary: boolean;
  offset_x: number;
  offset_y: number;
}

interface DisplayPickerProps {
  assetId: string;
  value: string;
  onSelect: (displayName: string) => void;
}

export default function DisplayPicker({ assetId, value, onSelect }: DisplayPickerProps) {
  const [loading, setLoading] = useState(true);
  const [displays, setDisplays] = useState<DisplayInfo[]>([]);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    fetch(`/api/displays/${encodeURIComponent(assetId)}`, { cache: "no-store" })
      .then((response) => response.json())
      .then((payload: { displays?: DisplayInfo[] }) => {
        if (cancelled) return;
        const next = Array.isArray(payload?.displays) ? payload.displays : [];
        setDisplays(next);
      })
      .catch(() => {
        if (!cancelled) setDisplays([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [assetId]);

  const selectedValue = useMemo(() => {
    if (value) return value;
    const primary = displays.find((display) => display.primary);
    return primary?.name ?? "";
  }, [displays, value]);

  useEffect(() => {
    if (value) return;
    const primary = displays.find((display) => display.primary);
    if (primary?.name) {
      onSelect(primary.name);
    }
  }, [displays, onSelect, value]);

  const handleChange = useCallback(
    (next: string) => {
      onSelect(next);
    },
    [onSelect],
  );

  if (loading) {
    return <span className="text-xs text-[var(--muted)]">Detecting displays...</span>;
  }

  if (displays.length <= 1) {
    return null;
  }

  return (
    <div className="flex items-center gap-2">
      <span className="text-xs text-[var(--muted)]">Display</span>
      <Select
        className="min-w-[180px]"
        value={selectedValue}
        onChange={(event) => handleChange(event.target.value)}
      >
        <option value="">All Displays</option>
        {displays.map((display) => (
          <option key={display.name} value={display.name}>
            {display.name} ({display.width}x{display.height}){display.primary ? " *" : ""}
          </option>
        ))}
      </Select>
    </div>
  );
}
