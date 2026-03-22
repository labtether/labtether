"use client";

import DisplayPicker from "../../../../components/DisplayPicker";
import WakeButton from "../../../../components/WakeButton";
import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import { Select } from "../../../../components/ui/Input";
import type { DesktopProtocol } from "../../../../components/SessionPanel";
import type { SessionConnectionState } from "../../../../hooks/useSession";

interface DesktopConnectionControlsCardProps {
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
  isReconnecting: boolean;
  onConnect: () => void;
  onDisconnect: () => void;
}

export function DesktopConnectionControlsCard({
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
  isReconnecting,
  onConnect,
  onDisconnect,
}: DesktopConnectionControlsCardProps) {
  if (connectionState === "connected") return null;

  const controlsDisabled = connectionState === "connecting" || connectionState === "authenticating";
  const showDisplayPicker = protocol === "vnc";

  return (
    <Card className="flex flex-col md:flex-row md:items-center md:justify-between mb-4 gap-3 py-2">
      <div className="flex flex-wrap items-center gap-2">
        <Select
          className="min-w-[110px]"
          value={quality}
          onChange={(event) => onQualityChange(event.target.value)}
          disabled={controlsDisabled}
        >
          <option value="low">Low</option>
          <option value="medium">Medium</option>
          <option value="high">High</option>
        </Select>
        <Select
          className="min-w-[120px]"
          value={protocol}
          onChange={(event) => onProtocolChange(event.target.value as DesktopProtocol)}
          disabled={controlsDisabled}
        >
          {availableProtocols.map((item) => (
            <option key={item} value={item}>{protocolLabel(item)}</option>
          ))}
        </Select>
        {showDisplayPicker && (
          <DisplayPicker
            assetId={nodeId}
            value={selectedDisplay}
            onSelect={onSelectedDisplayChange}
          />
        )}
        {targetStatus !== "online" && (
          <WakeButton assetId={nodeId} onWoken={() => { void onWake(); }} />
        )}
        {connectionState === "authenticating" ? (
          <Button variant="danger" onClick={onDisconnect}>Disconnect</Button>
        ) : (
          <Button
            variant="primary"
            onClick={onConnect}
            disabled={connectionState === "connecting"}
          >
            {connectionState === "connecting"
              ? (isReconnecting ? "Reconnecting..." : "Connecting...")
              : "Connect"}
          </Button>
        )}
      </div>
      <div className="flex items-center gap-2 text-xs text-[var(--muted)]">
        <span className={`h-1.5 w-1.5 rounded-full ${
          connectionState === "connecting" || connectionState === "authenticating" ? "bg-[var(--warn)]"
            : connectionState === "error" ? "bg-[var(--bad)]"
              : "bg-[var(--muted)]"
        }`} />
        <span>
          {connectionState === "connecting" ? (isReconnecting ? "Reconnecting..." : "Connecting...")
            : connectionState === "authenticating" ? "Authenticating..."
              : connectionState === "error" ? "Connection failed"
                : "Ready to connect"}
        </span>
      </div>
    </Card>
  );
}
