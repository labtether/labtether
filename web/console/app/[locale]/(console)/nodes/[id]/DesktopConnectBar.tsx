"use client";

import { useTranslations } from "next-intl";
import DisplayPicker from "../../../../components/DisplayPicker";
import WakeButton from "../../../../components/WakeButton";
import type { DesktopProtocol } from "../../../../components/SessionPanel";
import type { SessionConnectionState } from "../../../../hooks/useSession";

export interface DesktopConnectBarProps {
  nodeId: string;
  connectionState: SessionConnectionState;
  quality: string;
  onQualityChange: (quality: string) => void;
  protocol: DesktopProtocol;
  onProtocolChange: (protocol: DesktopProtocol) => void;
  availableProtocols: DesktopProtocol[];
  protocolLabel: (protocol: DesktopProtocol) => string;
  selectedDisplay: string;
  onSelectedDisplayChange: (display: string) => void;
  targetStatus?: string | null;
  onWake: () => void | Promise<void>;
}

/**
 * A device's screen before it connects: how to connect (quality, protocol, display) in the row above the screen, and
 * Wake when the device is not online. Connect itself sits on the screen. Locked while a connection is starting.
 */
export function DesktopConnectBar({
  nodeId,
  connectionState,
  quality,
  onQualityChange,
  protocol,
  onProtocolChange,
  availableProtocols,
  protocolLabel,
  selectedDisplay,
  onSelectedDisplayChange,
  targetStatus,
  onWake,
}: DesktopConnectBarProps) {
  const t = useTranslations("desktop.device");
  if (connectionState === "connected") return null;
  const locked = connectionState === "connecting" || connectionState === "authenticating";

  return (
    <div className="rv-connect">
      <label className="rv-sel">
        <span aria-hidden="true">{t("quality")}</span>
        <select aria-label={t("quality")} value={quality} onChange={(event) => onQualityChange(event.target.value)} disabled={locked}>
          <option value="low">{t("q.low")}</option>
          <option value="medium">{t("q.medium")}</option>
          <option value="high">{t("q.high")}</option>
        </select>
      </label>
      <label className="rv-sel">
        <span aria-hidden="true">{t("protocol")}</span>
        <select
          aria-label={t("protocol")}
          value={protocol}
          onChange={(event) => onProtocolChange(event.target.value as DesktopProtocol)}
          disabled={locked}
        >
          {availableProtocols.map((item) => (
            <option key={item} value={item}>
              {protocolLabel(item)}
            </option>
          ))}
        </select>
      </label>
      {protocol === "vnc" ? <DisplayPicker assetId={nodeId} value={selectedDisplay} onSelect={onSelectedDisplayChange} /> : null}
      {targetStatus !== "online" ? (
        <span className="rv-connect-end">
          <WakeButton assetId={nodeId} onWoken={() => void onWake()} />
        </span>
      ) : null}
    </div>
  );
}
