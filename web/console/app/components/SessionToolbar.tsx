"use client";

import { useTranslations } from "next-intl";
import type { ReactNode } from "react";
import type { SessionConnectionState, SessionType } from "../hooks/useSession";
import { Select } from "./ui/Input";

const qualityOptions = ["low", "medium", "high"] as const;

export interface SessionToolbarProps {
  type: SessionType;
  target: string;
  setTarget: (v: string) => void;
  isFixedTarget: boolean;
  assets: { id: string; name: string; platform?: string; source: string }[];
  connectedAgentIds: Set<string>;
  connectionState: SessionConnectionState;
  /** Kept for callers; the session's ID is not shown. */
  activeSessionId: string;
  isReconnecting: boolean;
  quality?: string;
  setQuality?: (v: string) => void;
  onConnect: () => void;
  onDisconnect: () => void;
  extraActions?: ReactNode;
  compact?: boolean;
}

/**
 * A session's bar: a status dot and word on the left ("Connected via SSH", "Reconnecting…"), then the device choice
 * when it is not fixed, Connect or Disconnect, and the caller's own actions.
 */
export function SessionToolbar({
  type,
  target,
  setTarget,
  isFixedTarget,
  assets,
  connectedAgentIds,
  connectionState,
  isReconnecting,
  quality,
  setQuality,
  onConnect,
  onDisconnect,
  extraActions,
  compact,
}: SessionToolbarProps) {
  const t = useTranslations("terminal.toolbar");
  const isConnected = connectionState === "connected";
  const isConnecting = connectionState === "connecting";
  const isAuthenticating = connectionState === "authenticating";
  const isError = connectionState === "error";
  const isActive = isConnecting || isAuthenticating || isConnected;
  const targetAsset = assets.find((asset) => asset.id === target);
  const via = connectedAgentIds.has(target)
    ? ""
    : targetAsset?.source === "proxmox"
      ? t("viaProxmox")
      : type === "terminal" ? t("viaSsh") : t("viaVnc");

  const statusText = isConnected
    ? via ? t("connectedVia", { via }) : t("connected")
    : isAuthenticating
      ? t("signingIn")
      : isConnecting
        ? isReconnecting ? t("reconnecting") : t("connecting")
        : isError
          ? t("failed")
          : t("notConnected");
  const tone = isConnected ? "ok" : isError ? "bad" : isActive ? "warn" : "off";

  return (
    <div className={`tm-tabbar${compact ? " compact" : ""}`}>
      <span className={`kt-st${isError ? " bad" : ""}`} role="status">
        <i className={`kt-dot sm ${tone}${isConnecting || isAuthenticating ? " busy" : ""}`} aria-hidden="true" />
        {statusText}
      </span>
      <span className="tm-sp" />
      {!isFixedTarget ? (
        <Select aria-label={t("device")} value={target} onChange={(e) => setTarget(e.target.value)} disabled={isActive}>
          <option value="">{t("chooseDevice")}</option>
          {assets.map((asset) => (
            <option key={asset.id} value={asset.id}>{asset.name}</option>
          ))}
        </Select>
      ) : null}
      {type === "desktop" && setQuality ? (
        <Select aria-label={t("quality")} value={quality} onChange={(e) => setQuality(e.target.value)} disabled={isActive}>
          {qualityOptions.map((value) => (
            <option key={value} value={value}>{t(`quality_${value}`)}</option>
          ))}
        </Select>
      ) : null}
      {isConnected || isAuthenticating ? (
        <button type="button" className="tm-qbtn" title={t("disconnect")} onClick={onDisconnect}>{t("disconnect")}</button>
      ) : (
        <button type="button" className="tm-qbtn" onClick={onConnect} disabled={!target || isConnecting}>
          {t("connect")}
        </button>
      )}
      {extraActions}
    </div>
  );
}
