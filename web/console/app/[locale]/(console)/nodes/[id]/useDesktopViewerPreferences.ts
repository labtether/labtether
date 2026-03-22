"use client";

import { useEffect, useRef, type Dispatch, type SetStateAction } from "react";
import type { ScalingMode } from "../../../../components/RemoteViewToolbar";
import {
  buildDesktopViewerPrefs,
  readDesktopViewerPrefs,
  restoreDesktopViewerPrefs,
  type DesktopProtocol,
  writeDesktopViewerPrefs,
} from "./desktopTabPreferences";

type UseDesktopViewerPreferencesArgs = {
  nodeId: string;
  availableProtocols: DesktopProtocol[];
  protocol: DesktopProtocol;
  hasExplicitProtocolPreference: boolean;
  connectionState: string;
  quality: string;
  scalingMode: ScalingMode;
  viewOnly: boolean;
  audioMuted: boolean;
  volume: number;
  selectedDisplay: string;
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

export function useDesktopViewerPreferences({
  nodeId,
  availableProtocols,
  protocol,
  hasExplicitProtocolPreference,
  connectionState,
  quality,
  scalingMode,
  viewOnly,
  audioMuted,
  volume,
  selectedDisplay,
  setScalingMode,
  setViewOnly,
  setAudioMuted,
  setVolume,
  setSelectedDisplay,
  setQuality,
  setProtocol,
  connect,
}: UseDesktopViewerPreferencesArgs) {
  const restoredPrefsRef = useRef(false);

  useEffect(() => {
    if (restoredPrefsRef.current || availableProtocols.length === 0) {
      return;
    }
    restoredPrefsRef.current = true;

    const prefs = readDesktopViewerPrefs(nodeId);
    if (!prefs) {
      return;
    }
    restoreDesktopViewerPrefs({
      prefs,
      availableProtocols,
      currentProtocol: protocol,
      connectionState,
      setScalingMode,
      setViewOnly,
      setAudioMuted,
      setVolume,
      setSelectedDisplay,
      setQuality,
      setProtocol,
      connect,
    });
  }, [
    availableProtocols,
    connect,
    connectionState,
    nodeId,
    protocol,
    setAudioMuted,
    setProtocol,
    setQuality,
    setScalingMode,
    setSelectedDisplay,
    setViewOnly,
    setVolume,
  ]);

  useEffect(() => {
    writeDesktopViewerPrefs(
      nodeId,
      buildDesktopViewerPrefs({
        protocol,
        hasExplicitProtocolPreference,
        quality,
        scalingMode,
        viewOnly,
        audioMuted,
        volume,
        selectedDisplay,
        connectionState,
      }),
    );
  }, [
    audioMuted,
    connectionState,
    hasExplicitProtocolPreference,
    nodeId,
    protocol,
    quality,
    scalingMode,
    selectedDisplay,
    viewOnly,
    volume,
  ]);
}
