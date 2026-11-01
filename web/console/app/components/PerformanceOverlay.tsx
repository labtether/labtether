"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import type { ViewerMetrics } from "../types/viewer";

const DASH = "—";
const num = (value: number | null, unit: string) => (value === null ? DASH : `${value} ${unit}`);
const mbit = (kbps: number | null) => (kbps === null ? DASH : `${(kbps / 1000).toFixed(1)} Mbit/s`);

interface PerformanceOverlayProps {
  metrics: ViewerMetrics;
  visible: boolean;
}

/** The stream's numbers in the screen's corner: frames, latency and bitrate, then codec, size and transport. */
export function PerformanceOverlay({ metrics, visible }: PerformanceOverlayProps) {
  const t = useTranslations("desktop.perf");
  if (!visible) return null;
  const { fps, latencyMs, bitrateKbps, codec, resolution, transport } = metrics;
  return (
    <div className="rv-perf" aria-label={t("label")} role="status">
      <span>{[num(fps, "fps"), num(latencyMs, "ms"), mbit(bitrateKbps)].join(" · ")}</span>
      <span>{[codec ?? DASH, resolution?.replace("x", "×") ?? DASH, transport].join(" · ")}</span>
    </div>
  );
}

/** The performance numbers' on/off state; Ctrl+F1 flips it. */
export function usePerformanceOverlayToggle(): {
  visible: boolean;
  toggle: () => void;
} {
  const [visible, setVisible] = useState(false);

  const toggle = useCallback(() => {
    setVisible((v) => !v);
  }, []);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.ctrlKey && e.key === "F1") {
        e.preventDefault();
        toggle();
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [toggle]);

  return { visible, toggle };
}
