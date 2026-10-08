"use client";

import { useEffect, type RefObject } from "react";
import type { WebRTCConnectionStats } from "./webrtcViewerTypes";

export function useWebRTCStats(
  connected: boolean,
  pcRef: RefObject<RTCPeerConnection | null>,
  onStatsRef: RefObject<((stats: WebRTCConnectionStats) => void) | undefined>,
) {
  useEffect(() => {
    if (!connected || !pcRef.current) {
      return;
    }

    let cancelled = false;
    let lastBytes = 0;
    let lastAt = 0;

    const collect = async () => {
      const pc = pcRef.current;
      if (!pc || cancelled) {
        return;
      }
      try {
        const reports = await pc.getStats();
        let rttMs: number | null = null;
        let packetsLost: number | null = null;
        let bitrateKbps: number | null = null;
        let fps: number | null = null;
        let selectedCandidatePairID: string | null = null;
        let selectedLocalCandidateID: string | null = null;
        let selectedRemoteCandidateID: string | null = null;
        const localCandidateTypes = new Map<string, string>();
        const remoteCandidateTypes = new Map<string, string>();

        reports.forEach((report) => {
          const typed = report as RTCStats & {
            kind?: string;
            roundTripTime?: number;
            currentRoundTripTime?: number;
            packetsLost?: number;
            framesPerSecond?: number;
            bytesReceived?: number;
            selectedCandidatePairId?: string;
            selected?: boolean;
            localCandidateId?: string;
            remoteCandidateId?: string;
            candidateType?: string;
          };
          if (
            report.type === "transport" &&
            typeof typed.selectedCandidatePairId === "string"
          ) {
            selectedCandidatePairID = typed.selectedCandidatePairId;
          }
          if (
            report.type === "remote-inbound-rtp" &&
            typed.kind === "video"
          ) {
            if (typeof typed.roundTripTime === "number") {
              rttMs = Math.round(typed.roundTripTime * 1000);
            }
          }
          if (report.type === "inbound-rtp" && typed.kind === "video") {
            if (typeof typed.packetsLost === "number") {
              packetsLost = typed.packetsLost;
            }
            if (typeof typed.framesPerSecond === "number") {
              fps = Math.round(typed.framesPerSecond);
            }
            if (typeof typed.bytesReceived === "number") {
              const now = Date.now();
              if (lastAt > 0 && now > lastAt) {
                const deltaBytes = typed.bytesReceived - lastBytes;
                const deltaMs = now - lastAt;
                if (deltaBytes >= 0 && deltaMs > 0) {
                  bitrateKbps = Math.round((deltaBytes * 8) / deltaMs);
                }
              }
              lastBytes = typed.bytesReceived;
              lastAt = now;
            }
          }
          if (
            report.type === "candidate-pair" &&
            (typed.selected === true || report.id === selectedCandidatePairID)
          ) {
            selectedCandidatePairID = report.id;
            selectedLocalCandidateID =
              typeof typed.localCandidateId === "string"
                ? typed.localCandidateId
                : null;
            selectedRemoteCandidateID =
              typeof typed.remoteCandidateId === "string"
                ? typed.remoteCandidateId
                : null;
            if (
              rttMs === null &&
              typeof typed.currentRoundTripTime === "number"
            ) {
              rttMs = Math.round(typed.currentRoundTripTime * 1000);
            }
          }
          if (
            report.type === "local-candidate" &&
            typeof typed.candidateType === "string"
          ) {
            localCandidateTypes.set(report.id, typed.candidateType);
          }
          if (
            report.type === "remote-candidate" &&
            typeof typed.candidateType === "string"
          ) {
            remoteCandidateTypes.set(report.id, typed.candidateType);
          }
        });

        const localCandidateType =
          (selectedLocalCandidateID &&
            localCandidateTypes.get(selectedLocalCandidateID)) ||
          null;
        const remoteCandidateType =
          (selectedRemoteCandidateID &&
            remoteCandidateTypes.get(selectedRemoteCandidateID)) ||
          null;
        const routeClass = deriveCandidateRouteClass(
          localCandidateType,
          remoteCandidateType,
        );

        onStatsRef.current?.({
          rttMs,
          packetsLost,
          bitrateKbps,
          fps,
          routeClass,
        });
      } catch {
        // Ignore transient stats errors.
      }
    };

    void collect();
    const timer = window.setInterval(() => {
      void collect();
    }, 2000);

    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [connected, pcRef, onStatsRef]);

}

function deriveCandidateRouteClass(
  localCandidateType: string | null,
  remoteCandidateType: string | null,
): "direct" | "reflexive" | "relay" | null {
  const candidates = [localCandidateType, remoteCandidateType]
    .map((value) => value?.trim().toLowerCase() ?? "")
    .filter(Boolean);

  if (candidates.length === 0) {
    return null;
  }
  if (candidates.includes("relay")) {
    return "relay";
  }
  if (candidates.includes("srflx") || candidates.includes("prflx")) {
    return "reflexive";
  }
  if (candidates.includes("host")) {
    return "direct";
  }
  return null;
}
