"use client";

import type { Dispatch, SetStateAction } from "react";

import type { ScalingMode } from "../../../../components/RemoteViewToolbar";

export type DesktopProtocol = "vnc" | "rdp" | "spice" | "webrtc";

export type DesktopViewerPrefs = {
  protocol?: DesktopProtocol;
  protocolAutoSelected?: boolean;
  quality?: string;
  scalingMode?: ScalingMode;
  viewOnly?: boolean;
  audioMuted?: boolean;
  volume?: number;
  selectedDisplay?: string;
  wasConnected?: boolean;
};

type DesktopViewerPrefsRestoreOptions = {
  prefs: DesktopViewerPrefs;
  availableProtocols: DesktopProtocol[];
  currentProtocol: DesktopProtocol;
  connectionState: string;
  setScalingMode: Dispatch<SetStateAction<ScalingMode>>;
  setViewOnly: Dispatch<SetStateAction<boolean>>;
  setAudioMuted: Dispatch<SetStateAction<boolean>>;
  setVolume: Dispatch<SetStateAction<number>>;
  setSelectedDisplay: Dispatch<SetStateAction<string>>;
  setQuality: (quality: string) => void;
  setProtocol: (
    protocol: DesktopProtocol,
    options?: { explicit?: boolean },
  ) => void;
  connect: (
    target?: string,
    options?: {
      protocol: DesktopProtocol;
      display: string;
      record: boolean;
    },
  ) => Promise<unknown>;
};

export function readDesktopViewerPrefs(nodeId: string): DesktopViewerPrefs | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = window.sessionStorage.getItem(
      `labtether.desktop.${nodeId}.prefs`,
    );
    if (!raw) return null;
    return JSON.parse(raw) as DesktopViewerPrefs;
  } catch {
    return null;
  }
}

export function writeDesktopViewerPrefs(nodeId: string, prefs: DesktopViewerPrefs) {
  if (typeof window === "undefined") return;
  try {
    window.sessionStorage.setItem(
      `labtether.desktop.${nodeId}.prefs`,
      JSON.stringify(prefs),
    );
  } catch {
    // Ignore storage failures.
  }
}

export function resolveDesktopDisplay(
  protocol: DesktopProtocol,
  selectedDisplay: string,
): string {
  if (protocol !== "vnc") {
    return "";
  }
  return selectedDisplay.trim();
}

export function restoreDesktopViewerPrefs({
  prefs,
  availableProtocols,
  currentProtocol,
  connectionState,
  setScalingMode,
  setViewOnly,
  setAudioMuted,
  setVolume,
  setSelectedDisplay,
  setQuality,
  setProtocol,
  connect,
}: DesktopViewerPrefsRestoreOptions) {
  if (prefs.scalingMode) setScalingMode(prefs.scalingMode);
  if (typeof prefs.viewOnly === "boolean") setViewOnly(prefs.viewOnly);
  if (typeof prefs.audioMuted === "boolean") setAudioMuted(prefs.audioMuted);
  if (typeof prefs.volume === "number") {
    setVolume(Math.max(0, Math.min(1, prefs.volume)));
  }
  if (prefs.selectedDisplay) setSelectedDisplay(prefs.selectedDisplay);
  if (prefs.quality) setQuality(prefs.quality);

  const restoredProtocol =
    prefs.protocolAutoSelected === true
      ? null
      : prefs.protocol && availableProtocols.includes(prefs.protocol)
        ? prefs.protocol
        : null;
  if (restoredProtocol !== null) {
    setProtocol(restoredProtocol);
  }

  if (prefs.wasConnected && connectionState === "idle") {
    const reconnectProtocol =
      restoredProtocol ?? availableProtocols[0] ?? currentProtocol;
    window.setTimeout(() => {
      void connect(undefined, {
        protocol: reconnectProtocol,
        display: resolveDesktopDisplay(
          reconnectProtocol,
          prefs.selectedDisplay ?? "",
        ),
        record: false,
      });
    }, 0);
  }
}

export function buildDesktopViewerPrefs({
  protocol,
  hasExplicitProtocolPreference,
  quality,
  scalingMode,
  viewOnly,
  audioMuted,
  volume,
  selectedDisplay,
  connectionState,
}: {
  protocol: DesktopProtocol;
  hasExplicitProtocolPreference: boolean;
  quality: string;
  scalingMode: ScalingMode;
  viewOnly: boolean;
  audioMuted: boolean;
  volume: number;
  selectedDisplay: string;
  connectionState: string;
}): DesktopViewerPrefs {
  return {
    protocol,
    protocolAutoSelected: !hasExplicitProtocolPreference,
    quality,
    scalingMode,
    viewOnly,
    audioMuted,
    volume,
    selectedDisplay,
    wasConnected:
      connectionState === "connecting" ||
      connectionState === "authenticating" ||
      connectionState === "connected",
  };
}
