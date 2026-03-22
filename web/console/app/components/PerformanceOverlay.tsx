"use client";

import { useCallback, useEffect, useState } from "react";
import type { ViewerMetrics } from "../types/viewer";

// ── Helpers ──

function latencyColor(ms: number | null): string {
  if (ms === null) return "rgba(255,255,255,0.85)";
  if (ms < 50) return "var(--ok)";
  if (ms <= 150) return "var(--warn)";
  return "var(--bad)";
}

function fmt(value: number | null, unit: string): string {
  if (value === null) return "\u2014";
  return `${value}${unit}`;
}

function fmtBitrate(kbps: number | null): string {
  if (kbps === null) return "\u2014";
  return `${(kbps / 1000).toFixed(1)} Mbps`;
}

function fmtStr(value: string | null): string {
  return value ?? "\u2014";
}

// ── PerformanceOverlay ──

interface PerformanceOverlayProps {
  metrics: ViewerMetrics;
  visible: boolean;
}

export function PerformanceOverlay({ metrics, visible }: PerformanceOverlayProps) {
  if (!visible) return null;

  const { fps, latencyMs, bitrateKbps, codec, resolution, transport } = metrics;

  const overlayStyle: React.CSSProperties = {
    position: "absolute",
    top: 8,
    right: 8,
    zIndex: 40,
    background: "rgba(0, 0, 0, 0.65)",
    backdropFilter: "blur(8px)",
    WebkitBackdropFilter: "blur(8px)",
    borderRadius: 6,
    padding: "6px 10px",
    fontFamily: "var(--font-mono, monospace)",
    fontSize: 11,
    color: "rgba(255, 255, 255, 0.85)",
    lineHeight: 1.6,
    pointerEvents: "none",
    whiteSpace: "nowrap",
  };

  const rowStyle: React.CSSProperties = {
    display: "flex",
    gap: 12,
  };

  const labelStyle: React.CSSProperties = {
    color: "rgba(255, 255, 255, 0.45)",
  };

  return (
    <div style={overlayStyle} aria-label="Performance metrics" role="status">
      {/* Row 1: FPS + Latency */}
      <div style={rowStyle}>
        <span>
          <span style={labelStyle}>FPS: </span>
          {fmt(fps, "")}
        </span>
        <span>
          <span style={labelStyle}>Latency: </span>
          <span style={{ color: latencyColor(latencyMs) }}>
            {fmt(latencyMs, "ms")}
          </span>
        </span>
      </div>

      {/* Row 2: Bitrate */}
      <div>
        <span style={labelStyle}>Bitrate: </span>
        {fmtBitrate(bitrateKbps)}
      </div>

      {/* Row 3: Codec + Resolution */}
      <div style={rowStyle}>
        <span>
          <span style={labelStyle}>Codec: </span>
          {fmtStr(codec)}
        </span>
        <span>
          <span style={labelStyle}>Res: </span>
          {fmtStr(resolution)}
        </span>
      </div>

      {/* Row 4: Transport */}
      <div>
        <span style={labelStyle}>Transport: </span>
        {transport}
      </div>
    </div>
  );
}

// ── usePerformanceOverlayToggle ──

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
