"use client";

import type { ReactNode } from "react";
import { Card } from "./ui/Card";
import { Button } from "./ui/Button";
import { Select } from "./ui/Input";
import type { SessionConnectionState, SessionType } from "../hooks/useSession";

const qualityOptions = [
  { value: "low", label: "Low" },
  { value: "medium", label: "Medium" },
  { value: "high", label: "High" },
];

export interface SessionToolbarProps {
  type: SessionType;
  target: string;
  setTarget: (v: string) => void;
  isFixedTarget: boolean;
  assets: { id: string; name: string; platform?: string; source: string }[];
  connectedAgentIds: Set<string>;
  connectionState: SessionConnectionState;
  activeSessionId: string;
  isReconnecting: boolean;
  quality?: string;
  setQuality?: (v: string) => void;
  onConnect: () => void;
  onDisconnect: () => void;
  extraActions?: ReactNode;
  compact?: boolean;
}

export function SessionToolbar({
  type,
  target,
  setTarget,
  isFixedTarget,
  assets,
  connectedAgentIds,
  connectionState,
  activeSessionId,
  isReconnecting,
  quality,
  setQuality,
  onConnect,
  onDisconnect,
  extraActions,
  compact,
}: SessionToolbarProps) {
  const isConnected = connectionState === "connected";
  const isConnecting = connectionState === "connecting";
  const isAuthenticating = connectionState === "authenticating";
  const isError = connectionState === "error";
  const isActive = isConnecting || isAuthenticating || isConnected;
  const targetHasAgent = connectedAgentIds.has(target);
  const targetAsset = assets.find((asset) => asset.id === target);
  const targetLabel = targetAsset?.name ?? target;
  const targetIsProxmox = targetAsset?.source === "proxmox";
  const transportLabel = targetHasAgent
    ? "agent tunnel"
    : targetIsProxmox
      ? "direct API"
      : type === "terminal"
        ? "SSH"
        : "VNC";

  const statusText = isConnected
    ? `${targetLabel} connected via ${transportLabel}`
    : isAuthenticating
      ? `Authenticating ${targetLabel}...`
      : isConnecting
        ? isReconnecting ? `Reconnecting to ${targetLabel}...` : `Connecting to ${targetLabel}...`
        : isError
          ? "Connection failed"
          : "Ready to connect";

  const statusDotClass = isConnected
    ? "h-1.5 w-1.5 rounded-full bg-[var(--ok)]"
    : isConnecting || isAuthenticating
      ? "h-1.5 w-1.5 rounded-full bg-[var(--warn)]"
      : isError
        ? "h-1.5 w-1.5 rounded-full bg-[var(--bad)]"
        : "h-1.5 w-1.5 rounded-full bg-[var(--muted)]";

  return (
    <Card className={`flex items-center justify-between mb-4${compact ? " py-2" : ""}`}>
      <div className="flex items-center gap-2">
        {!isFixedTarget && (
          <Select
            className="min-w-[220px]"
            value={target}
            onChange={(e) => setTarget(e.target.value)}
            disabled={isActive}
          >
            <option value="">Choose a device...</option>
            {assets.map((asset) => (
              <option key={asset.id} value={asset.id}>
                {connectedAgentIds.has(asset.id) ? "\u2713 " : ""}{asset.name} ({asset.platform || asset.source})
              </option>
            ))}
          </Select>
        )}
        {type === "desktop" && setQuality && (
          <Select
            className="min-w-[110px]"
            value={quality}
            onChange={(e) => setQuality(e.target.value)}
            disabled={isActive}
          >
            {qualityOptions.map((opt) => (
              <option key={opt.value} value={opt.value}>{opt.label}</option>
            ))}
          </Select>
        )}
        {isConnected || isAuthenticating ? (
          <Button variant="danger" onClick={onDisconnect}>
            Disconnect
          </Button>
        ) : (
          <Button
            variant="primary"
            onClick={onConnect}
            disabled={!target || isConnecting}
          >
            {isConnecting ? (isReconnecting ? "Reconnecting..." : "Connecting...") : "Connect"}
          </Button>
        )}
      </div>
      <div className="flex items-center gap-2 text-xs text-[var(--muted)]">
        <span className={statusDotClass} />
        <span>{statusText}</span>
        {(isConnected || activeSessionId) && (
          <>
            <span className="w-px h-3 bg-[var(--line)]" />
            {extraActions}
            {activeSessionId && (
              <span className="text-[10px] px-1.5 py-0.5 rounded-lg border border-[var(--line)] font-mono">
                {activeSessionId.slice(0, 12)}
              </span>
            )}
          </>
        )}
      </div>
    </Card>
  );
}
