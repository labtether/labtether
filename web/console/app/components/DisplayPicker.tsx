"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslations } from "next-intl";

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

/** The remote machine's displays as a compact select; hidden when it has one. Picks the primary display by default. */
export default function DisplayPicker({ assetId, value, onSelect }: DisplayPickerProps) {
  const t = useTranslations("desktop.device");
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
    return <span className="rv-sel-note">{t("detecting")}</span>;
  }

  if (displays.length <= 1) {
    return null;
  }

  return (
    <label className="rv-sel">
      <span aria-hidden="true">{t("display")}</span>
      <select aria-label={t("display")} value={selectedValue} onChange={(event) => handleChange(event.target.value)}>
        <option value="">{t("allDisplays")}</option>
        {displays.map((display) => (
          <option key={display.name} value={display.name}>
            {t(display.primary ? "displayPrimary" : "displayItem", { name: display.name, width: display.width, height: display.height })}
          </option>
        ))}
      </select>
    </label>
  );
}
