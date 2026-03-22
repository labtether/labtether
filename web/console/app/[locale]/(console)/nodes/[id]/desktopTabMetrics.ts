"use client";

import type { WebRTCConnectionStats } from "../../../../components/WebRTCViewer";
import type { ViewerMetrics } from "../../../../types/viewer";

export function buildWebRTCMetrics(
  stats: WebRTCConnectionStats | null,
  stream: MediaStream | null,
): ViewerMetrics {
  const videoTrack = stream?.getVideoTracks()[0];
  const settings = videoTrack?.getSettings();
  const width = typeof settings?.width === "number" ? settings.width : null;
  const height = typeof settings?.height === "number" ? settings.height : null;

  return {
    fps: stats?.fps ?? null,
    latencyMs: stats?.rttMs ?? null,
    bitrateKbps: stats?.bitrateKbps ?? null,
    codec: "WebRTC",
    resolution: width && height ? `${width}x${height}` : null,
    transport:
      stats?.routeClass === "relay"
        ? "WebRTC relay"
        : stats?.routeClass === "reflexive"
          ? "WebRTC reflexive"
          : stats?.routeClass === "direct"
            ? "WebRTC direct"
            : "WebRTC",
  };
}

export function deriveNetworkQuality(
  metrics: ViewerMetrics | null | undefined,
): "good" | "fair" | "poor" | null {
  if (!metrics) return null;
  const fps = metrics.fps;
  const latencyMs = metrics.latencyMs;
  const bitrateKbps = metrics.bitrateKbps;
  const lowFPS = typeof fps === "number" && fps < 10;
  const reducedFPS = typeof fps === "number" && fps < 20;
  const highLatency = typeof latencyMs === "number" && latencyMs > 200;
  const elevatedLatency = typeof latencyMs === "number" && latencyMs > 100;
  const lowBitrate = typeof bitrateKbps === "number" && bitrateKbps < 800;
  const severeBitrateConstraint =
    typeof bitrateKbps === "number" && bitrateKbps < 250;

  // Bitrate alone is a poor indicator on mostly static screens because a
  // healthy encoder can legitimately idle at a few hundred kbps while still
  // delivering smooth video. Reserve "poor" for low fps/high latency, or for
  // low bitrate paired with another degraded signal.
  if (lowFPS || highLatency || (severeBitrateConstraint && (reducedFPS || elevatedLatency))) {
    return "poor";
  }
  if (reducedFPS || elevatedLatency || lowBitrate) {
    return "fair";
  }
  if (fps !== null || latencyMs !== null || bitrateKbps !== null) {
    return "good";
  }
  return null;
}

export function chooseBrowserRecordingMimeType(): string | undefined {
  if (typeof MediaRecorder === "undefined") return undefined;
  for (const candidate of [
    "video/webm;codecs=vp9,opus",
    "video/webm;codecs=vp8,opus",
    "video/webm",
  ]) {
    if (MediaRecorder.isTypeSupported(candidate)) {
      return candidate;
    }
  }
  return undefined;
}

export function downloadBrowserRecording(nodeName: string, blob: Blob) {
  if (typeof window === "undefined") return;
  const stamp = new Date().toISOString().replace(/[:.]/g, "-");
  const link = document.createElement("a");
  const url = window.URL.createObjectURL(blob);
  link.href = url;
  link.download = `${nodeName || "desktop-session"}-${stamp}.webm`;
  document.body.appendChild(link);
  link.click();
  link.remove();
  window.setTimeout(() => window.URL.revokeObjectURL(url), 1000);
}
