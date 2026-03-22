"use client";

import { useEffect, useRef, useState } from "react";

import type { ViewerMetrics } from "../types/viewer";

export interface UseViewerMetricsOptions {
  protocol: "vnc" | "rdp" | "spice" | "webrtc";
  connected: boolean;
  /** For VNC: the container element that noVNC renders its canvas into. */
  vncContainerRef?: React.RefObject<HTMLElement | null>;
  /** For WebRTC: the active RTCPeerConnection. */
  peerConnection?: RTCPeerConnection | null;
}

const NULL_METRICS: ViewerMetrics = {
  fps: null,
  latencyMs: null,
  bitrateKbps: null,
  codec: null,
  resolution: null,
  transport: "",
};

/**
 * Collects protocol-specific performance metrics and exposes them as a unified
 * ViewerMetrics object suitable for a performance HUD overlay.
 *
 * - VNC: counts rAF callbacks per second; reads canvas dimensions for resolution.
 * - WebRTC: polls RTCPeerConnection.getStats() every 1 s for fps, latency,
 *   bitrate, codec, and resolution.
 * - RDP / SPICE: returns transport label only; protocol-level stats are not yet
 *   available.
 *
 * All metrics reset to null when `connected` is false.
 */
export function useViewerMetrics(options: UseViewerMetricsOptions): ViewerMetrics {
  const { protocol, connected, vncContainerRef, peerConnection } = options;

  const [metrics, setMetrics] = useState<ViewerMetrics>({
    ...NULL_METRICS,
    transport: deriveTransport(protocol),
  });

  // Reset when disconnected.
  useEffect(() => {
    if (!connected) {
      setMetrics({ ...NULL_METRICS, transport: deriveTransport(protocol) });
    }
  }, [connected, protocol]);

  // VNC metrics: rAF-based FPS counter + canvas dimension polling.
  useEffect(() => {
    if (protocol !== "vnc" || !connected) {
      return;
    }

    let frameCount = 0;
    let animFrameId = 0;
    let intervalId = 0;

    const countFrame = () => {
      frameCount++;
      animFrameId = requestAnimationFrame(countFrame);
    };
    animFrameId = requestAnimationFrame(countFrame);

    intervalId = window.setInterval(() => {
      const fps = frameCount;
      frameCount = 0;

      let resolution: string | null = null;
      if (vncContainerRef?.current) {
        const canvas = vncContainerRef.current.querySelector("canvas");
        if (canvas) {
          resolution = `${canvas.width}x${canvas.height}`;
        }
      }

      setMetrics({
        fps,
        latencyMs: null,
        bitrateKbps: null,
        codec: "RFB",
        resolution,
        transport: "VNC",
      });
    }, 1000);

    return () => {
      cancelAnimationFrame(animFrameId);
      window.clearInterval(intervalId);
    };
  }, [protocol, connected, vncContainerRef]);

  // WebRTC metrics: getStats() polling.
  const lastBytesRef = useRef(0);
  const lastAtRef = useRef(0);

  useEffect(() => {
    if (protocol !== "webrtc" || !connected || !peerConnection) {
      return;
    }

    // Reset byte-tracking accumulators when a new effect fires.
    lastBytesRef.current = 0;
    lastAtRef.current = 0;

    let cancelled = false;

    const collect = async () => {
      if (cancelled || !peerConnection) {
        return;
      }

      try {
        const reports = await peerConnection.getStats();

        let fps: number | null = null;
        let latencyMs: number | null = null;
        let bitrateKbps: number | null = null;
        let codec: string | null = null;
        let resolution: string | null = null;

        // Track which codec id the inbound-rtp track refers to so we can
        // look it up in the codec report.
        let videoCodecId: string | undefined;

        reports.forEach((report) => {
          // --- inbound-rtp (video) ---
          if (report.type === "inbound-rtp") {
            const r = report as RTCStats & {
              kind?: string;
              framesPerSecond?: number;
              bytesReceived?: number;
              frameWidth?: number;
              frameHeight?: number;
              codecId?: string;
            };
            if (r.kind !== "video") {
              return;
            }

            if (typeof r.framesPerSecond === "number") {
              fps = Math.round(r.framesPerSecond);
            }

            if (typeof r.bytesReceived === "number") {
              const now = Date.now();
              if (lastAtRef.current > 0 && now > lastAtRef.current) {
                const deltaBytes = r.bytesReceived - lastBytesRef.current;
                const deltaMs = now - lastAtRef.current;
                if (deltaBytes >= 0 && deltaMs > 0) {
                  bitrateKbps = Math.round((deltaBytes * 8) / deltaMs);
                }
              }
              lastBytesRef.current = r.bytesReceived;
              lastAtRef.current = now;
            }

            if (typeof r.frameWidth === "number" && typeof r.frameHeight === "number") {
              resolution = `${r.frameWidth}x${r.frameHeight}`;
            }

            if (typeof r.codecId === "string") {
              videoCodecId = r.codecId;
            }
          }

          // --- remote-inbound-rtp (video) — round-trip time ---
          if (report.type === "remote-inbound-rtp") {
            const r = report as RTCStats & {
              kind?: string;
              roundTripTime?: number;
            };
            if (r.kind === "video" && typeof r.roundTripTime === "number") {
              latencyMs = Math.round(r.roundTripTime * 1000);
            }
          }
        });

        // Resolve codec name from the codec report that inbound-rtp references.
        if (videoCodecId) {
          const codecReport = reports.get(videoCodecId) as
            | (RTCStats & { mimeType?: string })
            | undefined;
          if (codecReport?.mimeType) {
            // mimeType is e.g. "video/VP8" — strip the "video/" prefix.
            codec = codecReport.mimeType.replace(/^video\//i, "");
          }
        }

        setMetrics({ fps, latencyMs, bitrateKbps, codec, resolution, transport: "WebRTC" });
      } catch {
        // Transient stats errors are non-fatal; keep the last known values.
      }
    };

    void collect();
    const timerId = window.setInterval(() => {
      void collect();
    }, 1000);

    return () => {
      cancelled = true;
      window.clearInterval(timerId);
    };
  }, [protocol, connected, peerConnection]);

  return metrics;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function deriveTransport(protocol: UseViewerMetricsOptions["protocol"]): string {
  switch (protocol) {
    case "vnc":
      return "VNC";
    case "rdp":
      return "RDP";
    case "spice":
      return "SPICE";
    case "webrtc":
      return "WebRTC";
  }
}
