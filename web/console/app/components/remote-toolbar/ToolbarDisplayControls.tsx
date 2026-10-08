"use client";
import { Monitor } from "lucide-react";
import type { RemoteViewToolbarProps, ScalingMode } from "./types";
import { SegmentGroup } from "./ToolbarButtons";

function latencyColor(ms: number | null): string {
  if (ms === null) return "var(--muted)";
  if (ms < 50) return "var(--ok)";
  if (ms <= 150) return "var(--warn)";
  return "var(--bad)";
}

function networkQualityColor(
  quality: "good" | "fair" | "poor" | null | undefined,
): string {
  if (quality === "good") return "var(--ok)";
  if (quality === "fair") return "var(--warn)";
  if (quality === "poor") return "var(--bad)";
  return "rgba(255,255,255,0.25)";
}

export function ToolbarStatus({
  latencyMs,
  transportLabel,
  protocol,
  networkQuality,
  isOverlayLayout,
}: Pick<
  RemoteViewToolbarProps,
  "latencyMs" | "transportLabel" | "protocol" | "networkQuality"
> & { isOverlayLayout: boolean }) {
  const dotColor = latencyColor(latencyMs);
  return (
    <div
      className="flex items-center gap-1.5"
      style={isOverlayLayout ? { pointerEvents: "none" } : undefined}
    >
      <span
        className="rounded-full"
        style={{
          width: 6,
          height: 6,
          backgroundColor: dotColor,
          boxShadow: `0 0 6px ${dotColor}`,
          flexShrink: 0,
        }}
      />
      {latencyMs !== null && (
        <span
          style={{
            fontSize: 11,
            color: "rgba(255,255,255,0.8)",
            whiteSpace: "nowrap",
          }}
        >
          {latencyMs}ms
        </span>
      )}
      {!isOverlayLayout && (
        <span
          style={{
            fontSize: 10,
            color: "rgba(255,255,255,0.4)",
            whiteSpace: "nowrap",
          }}
        >
          {transportLabel}
        </span>
      )}
      {!isOverlayLayout && (
        <>
          <span
            className="rounded-sm border border-white/20 px-1 py-[1px] font-semibold uppercase tracking-wide"
            style={{ fontSize: 9, color: "rgba(255,255,255,0.75)" }}
          >
            {protocol}
          </span>
          {networkQuality && (
            <span
              className="rounded-sm border px-1 py-[1px] font-semibold uppercase tracking-wide"
              style={{
                fontSize: 9,
                color: networkQualityColor(networkQuality),
                borderColor: networkQualityColor(networkQuality),
              }}
            >
              {networkQuality}
            </span>
          )}
        </>
      )}
    </div>
  );
}

export function ToolbarStatusBadges({
  protocol,
  quality,
  networkQuality,
}: Pick<RemoteViewToolbarProps, "protocol" | "quality" | "networkQuality">) {
  return (
    <div className="flex items-center gap-1">
      <span
        className="rounded-sm border border-white/15 px-1.5 py-[2px] font-semibold uppercase tracking-wide"
        style={{ fontSize: 9, color: "rgba(255,255,255,0.78)" }}
      >
        {protocol}
      </span>
      <span
        className="rounded-sm border border-white/15 px-1.5 py-[2px] font-semibold uppercase tracking-wide"
        style={{ fontSize: 9, color: "rgba(255,255,255,0.65)" }}
      >
        {quality}
      </span>
      {networkQuality && (
        <span
          className="rounded-sm border px-1.5 py-[2px] font-semibold uppercase tracking-wide"
          style={{
            fontSize: 9,
            color: networkQualityColor(networkQuality),
            borderColor: networkQualityColor(networkQuality),
          }}
        >
          {networkQuality}
        </span>
      )}
    </div>
  );
}

export function ToolbarQuality({
  quality,
  onQualityChange,
}: Pick<RemoteViewToolbarProps, "quality" | "onQualityChange">) {
  return (
    <SegmentGroup
      label="Stream quality"
      options={[
        { value: "low", label: "Low" },
        { value: "medium", label: "Med" },
        { value: "high", label: "High" },
      ]}
      value={quality}
      onChange={onQualityChange}
    />
  );
}

export function ToolbarDisplayPicker({
  displays,
  onDisplayChange,
  selectedDisplay,
}: Pick<
  RemoteViewToolbarProps,
  "displays" | "onDisplayChange" | "selectedDisplay"
>) {
  return displays && displays.length > 1 && onDisplayChange ? (
    <div className="flex items-center gap-1">
      <Monitor className="w-3.5 h-3.5 text-white/50 flex-shrink-0" />
      <select
        aria-label="Select display"
        value={selectedDisplay || ""}
        onChange={(e) => onDisplayChange(e.target.value)}
        className="rounded-md border border-white/10 bg-transparent text-white/80 px-1.5 py-0.5 outline-none focus:border-[var(--accent)] focus:ring-1 focus:ring-[var(--accent)] cursor-pointer appearance-none"
        style={{
          fontSize: 11,
          backgroundImage: `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='10' viewBox='0 0 24 24' fill='none' stroke='rgba(255,255,255,0.5)' stroke-width='2'%3E%3Cpolyline points='6 9 12 15 18 9'%3E%3C/polyline%3E%3C/svg%3E")`,
          backgroundRepeat: "no-repeat",
          backgroundPosition: "right 4px center",
          paddingRight: 18,
        }}
      >
        {displays.map((d) => (
          <option
            key={d.name}
            value={d.name}
            style={{ background: "#1a1a1a", color: "#fff" }}
          >
            {d.name} ({d.width}x{d.height}){d.primary ? " \u2605" : ""}
          </option>
        ))}
      </select>
    </div>
  ) : null;
}

export function ToolbarScaling({
  scalingMode,
  onScalingModeChange,
}: Pick<RemoteViewToolbarProps, "scalingMode" | "onScalingModeChange">) {
  return (
    <SegmentGroup
      label="Scaling mode"
      options={[
        { value: "fit", label: "Fit" },
        { value: "native", label: "1:1" },
        { value: "fill", label: "Fill" },
      ]}
      value={scalingMode}
      onChange={(v) => onScalingModeChange(v as ScalingMode)}
    />
  );
}
