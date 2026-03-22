"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type { DesktopProtocol } from "../../../../components/SessionPanel";
import type { SessionConnectionState } from "../../../../hooks/useSession";

export interface DesktopProtocolAsset {
  source?: string | null;
  platform?: string | null;
  metadata?: Record<string, unknown> | null;
}

function normalizeLower(value: unknown): string {
  if (typeof value === "string") return value.toLowerCase();
  return "";
}

function metadataFlag(
  targetAsset: DesktopProtocolAsset | null | undefined,
  key: string,
): boolean {
  return normalizeLower(targetAsset?.metadata?.[key]) === "true";
}

export function computeAvailableDesktopProtocols(
  targetAsset: DesktopProtocolAsset | null | undefined,
  targetHasAgent: boolean,
): DesktopProtocol[] {
  const options: DesktopProtocol[] = ["vnc"];
  const source = normalizeLower(targetAsset?.source);
  const proxmoxType = normalizeLower(targetAsset?.metadata?.proxmox_type);
  const webrtcAvailable =
    metadataFlag(targetAsset, "webrtc_available");
  const platform = normalizeLower(targetAsset?.platform);
  const sessionType = normalizeLower(targetAsset?.metadata?.desktop_session_type);
  const vncRealDesktopSupported = metadataFlag(
    targetAsset,
    "desktop_vnc_real_desktop_supported",
  );
  const isWayland = platform === "linux" && sessionType === "wayland";
  const preferVNCFirst =
    platform === "linux" && (!isWayland || vncRealDesktopSupported);

  if (platform === "windows") {
    options.unshift("rdp");
  }
  if (source === "proxmox" && proxmoxType === "qemu") {
    options.push("spice");
  }
  if (targetHasAgent && webrtcAvailable) {
    if (preferVNCFirst) {
      options.push("webrtc");
    } else {
      options.unshift("webrtc");
    }
  }
  if (isWayland && !vncRealDesktopSupported) {
    const reordered: DesktopProtocol[] = options.filter(
      (protocol) => protocol !== "vnc",
    );
    reordered.push("vnc");
    return Array.from(new Set(reordered));
  }

  return Array.from(new Set(options));
}

export function getDesktopTransportLabel(
  protocol: DesktopProtocol,
  targetHasAgent: boolean,
  source?: string | null,
): string {
  if (protocol === "webrtc") return "webrtc p2p";
  if (targetHasAgent) return "agent tunnel";
  if (source === "proxmox") return "direct API";
  return protocol.toUpperCase();
}

export function getDesktopProtocolLabel(
  protocol: DesktopProtocol,
  availableProtocols: DesktopProtocol[],
  targetAsset?: DesktopProtocolAsset | null,
): string {
  if (protocol === "webrtc") {
    return availableProtocols[0] === "webrtc"
      ? "WebRTC (Recommended)"
      : "WebRTC";
  }
  if (protocol === "rdp") return "RDP (Recommended)";
  if (protocol === "spice") return "SPICE (Optional)";
  const isWayland =
    normalizeLower(targetAsset?.platform) === "linux" &&
    normalizeLower(targetAsset?.metadata?.desktop_session_type) === "wayland";
  const vncRealDesktopSupported = metadataFlag(
    targetAsset,
    "desktop_vnc_real_desktop_supported",
  );
  if (protocol === "vnc" && isWayland && !vncRealDesktopSupported) {
    return availableProtocols[0] === "webrtc"
      ? "VNC (Fallback Desktop)"
      : "VNC (Fallback Only)";
  }
  if (protocol === "vnc" && availableProtocols[0] === "vnc") {
    return "VNC (Recommended)";
  }
  if (availableProtocols[0] === "vnc" && availableProtocols.includes("spice")) {
    return "VNC (Recommended)";
  }
  if (availableProtocols[0] !== "vnc") return "VNC (Fallback)";
  return "VNC";
}

interface UseDesktopProtocolStateOptions {
  nodeId: string;
  targetAsset: DesktopProtocolAsset | null | undefined;
  connectedAgentIds: Set<string>;
  connectionState: SessionConnectionState;
}

interface SetDesktopProtocolOptions {
  explicit?: boolean;
}

export function useDesktopProtocolState({
  nodeId,
  targetAsset,
  connectedAgentIds,
  connectionState,
}: UseDesktopProtocolStateOptions) {
  const [protocol, setProtocolState] = useState<DesktopProtocol>("vnc");
  const requestedProtocolRef = useRef<DesktopProtocol | null>(null);
  const targetHasAgent = connectedAgentIds.has(nodeId);

  const availableProtocols = useMemo(
    () => computeAvailableDesktopProtocols(targetAsset, targetHasAgent),
    [targetAsset, targetHasAgent],
  );

  useEffect(() => {
    if (availableProtocols.length === 0) return;
    const requestedProtocol = requestedProtocolRef.current;
    const desiredProtocol =
      requestedProtocol && availableProtocols.includes(requestedProtocol)
        ? requestedProtocol
        : availableProtocols[0];

    if (!availableProtocols.includes(protocol)) {
      setProtocolState(desiredProtocol);
      return;
    }
    if (connectionState === "idle" && desiredProtocol !== protocol) {
      setProtocolState(desiredProtocol);
    }
  }, [availableProtocols, connectionState, protocol]);

  useEffect(() => {
    requestedProtocolRef.current = null;
  }, [nodeId]);

  const setProtocol = useCallback((
    nextProtocol: DesktopProtocol,
    options?: SetDesktopProtocolOptions,
  ) => {
    requestedProtocolRef.current =
      options?.explicit === false ? null : nextProtocol;
    setProtocolState(nextProtocol);
  }, []);

  const transportLabel = useMemo(
    () =>
      getDesktopTransportLabel(protocol, targetHasAgent, targetAsset?.source),
    [protocol, targetAsset?.source, targetHasAgent],
  );

  const protocolLabel = useCallback(
    (item: DesktopProtocol) =>
      getDesktopProtocolLabel(item, availableProtocols, targetAsset),
    [availableProtocols, targetAsset],
  );

  const allowAutomaticFallbackToVNC = !(
    normalizeLower(targetAsset?.platform) === "linux" &&
    normalizeLower(targetAsset?.metadata?.desktop_session_type) === "wayland" &&
    !metadataFlag(targetAsset, "desktop_vnc_real_desktop_supported")
  );

  return {
    protocol,
    setProtocol,
    hasExplicitProtocolPreference: requestedProtocolRef.current !== null,
    availableProtocols,
    targetHasAgent,
    transportLabel,
    protocolLabel,
    allowAutomaticFallbackToVNC,
  };
}
