"use client";

import dynamic from "next/dynamic";
import type { RefObject, ReactNode } from "react";
import { Card } from "./ui/Card";
import { Button } from "./ui/Button";
import type {
  SessionConnectionState,
  SessionStreamStatus,
  SessionType,
} from "../hooks/useSession";
import type { XTerminalHandle } from "./XTerminal";
import type { VNCViewerHandle, VNCCredentialRequest } from "./VNCViewer";
import type { GuacamoleViewerHandle } from "./GuacamoleViewer";
import type { SPICEViewerHandle } from "./SPICEViewer";
import type {
  WebRTCViewerHandle,
  WebRTCConnectionStats,
  WebRTCDisplayLayout,
} from "./WebRTCViewer";
import type { ScalingMode } from "./RemoteViewToolbar";
import type { ITheme } from "@xterm/xterm";

const XTerminal = dynamic(() => import("./XTerminal"), { ssr: false });
const VNCViewer = dynamic(() => import("./VNCViewer"), { ssr: false });
const GuacamoleViewer = dynamic(() => import("./GuacamoleViewer"), {
  ssr: false,
});
const SPICEViewer = dynamic(() => import("./SPICEViewer"), { ssr: false });
const WebRTCViewer = dynamic(() => import("./WebRTCViewer"), { ssr: false });

export type DesktopProtocol = "vnc" | "rdp" | "spice" | "webrtc";

export interface SpiceTicket {
  wsUrl: string;
  password: string;
  type?: string;
  ca?: string;
  proxy?: string;
}

export interface SessionPanelProps {
  type: SessionType;
  connectionState: SessionConnectionState;
  wsUrl: string | null;
  error: string | null;
  target: string;
  connectedAgentIds: Set<string>;
  onRetry: () => void;
  // Terminal refs/callbacks
  termRef?: RefObject<XTerminalHandle | null>;
  onTerminalConnected?: () => void;
  onTerminalDisconnected?: (reason?: string) => void;
  onTerminalError?: (message: string) => void;
  onTerminalStreamReady?: (message?: string) => void;
  onTerminalStreamStatus?: (status: SessionStreamStatus) => void;
  // Terminal appearance props (passed through to XTerminal)
  terminalTheme?: ITheme;
  terminalFontFamily?: string;
  terminalFontSize?: number;
  terminalCursorStyle?: "block" | "underline" | "bar";
  terminalCursorBlink?: boolean;
  terminalScrollback?: number;
  // Desktop refs/callbacks
  vncRef?: RefObject<VNCViewerHandle | null>;
  guacRef?: RefObject<GuacamoleViewerHandle | null>;
  spiceRef?: RefObject<SPICEViewerHandle | null>;
  webrtcRef?: RefObject<WebRTCViewerHandle | null>;
  protocol?: DesktopProtocol;
  spiceTicket?: SpiceTicket | null;
  quality?: string;
  scalingMode?: ScalingMode;
  viewOnly?: boolean;
  audioEnabled?: boolean;
  volume?: number;
  onWebRTCStats?: (stats: WebRTCConnectionStats) => void;
  onWebRTCStream?: (stream: MediaStream | null) => void;
  webrtcDisplayLayout?: WebRTCDisplayLayout[];
  onDesktopConnect?: () => void;
  onDesktopDisconnect?: (detail: { clean: boolean; reason?: string }) => void;
  onDesktopError?: (message: string) => void;
  onCredentialsRequired?: (request: VNCCredentialRequest) => void;
  // Credential overlay (desktop only, managed by parent)
  credentialOverlay?: ReactNode;
  // Idle overlay text override
  idleText?: string;
}

export function SessionPanel({
  type,
  connectionState,
  wsUrl,
  error,
  target,
  connectedAgentIds,
  onRetry,
  termRef,
  onTerminalConnected,
  onTerminalDisconnected,
  onTerminalError,
  onTerminalStreamReady,
  onTerminalStreamStatus,
  terminalTheme,
  terminalFontFamily,
  terminalFontSize,
  terminalCursorStyle,
  terminalCursorBlink,
  terminalScrollback,
  vncRef,
  guacRef,
  spiceRef,
  webrtcRef,
  protocol = "vnc",
  spiceTicket,
  quality,
  scalingMode,
  viewOnly,
  audioEnabled = true,
  volume = 1,
  onWebRTCStats,
  onWebRTCStream,
  webrtcDisplayLayout,
  onDesktopConnect,
  onDesktopDisconnect,
  onDesktopError,
  onCredentialsRequired,
  credentialOverlay,
  idleText,
}: SessionPanelProps) {
  const isIdle = connectionState === "idle";
  const isConnecting = connectionState === "connecting";
  const isError = connectionState === "error";
  const targetHasAgent = connectedAgentIds.has(target);

  const panelMinHeight =
    type === "terminal" ? "min-h-[400px]" : "min-h-[500px]";
  const desktopPanelHeight = type === "desktop" ? "h-full" : "";
  const emptyIcon = type === "terminal" ? "\u2318" : "\uD83D\uDDA5";

  const errorHelpText =
    type === "terminal"
      ? "Devices with a connected agent (\u2713) support instant terminal access. Other devices require SSH credentials."
      : "Remote view supports WebRTC, RDP, SPICE, and VNC. Agent-connected devices (\u2713) can negotiate the best mode automatically.";

  const defaultIdleText =
    type === "terminal"
      ? "Select a device and click Connect. Agent-connected devices (\u2713) use instant terminal access; other devices use SSH."
      : "Select a device from the dropdown and click Connect to start a remote view session.";

  // Suppress unused variable warning - targetHasAgent is used for future conditional logic
  void targetHasAgent;

  return (
    <>
      {/* Error banner */}
      {error && (
        <Card className="flex items-start gap-3 border-[var(--bad)]/20 mb-4">
          <span className="text-[var(--bad)] text-sm font-medium shrink-0">
            &#x26A0;
          </span>
          <div className="flex-1 space-y-1">
            <p className="text-sm text-[var(--bad)]">{error}</p>
            <p className="text-xs text-[var(--muted)]">{errorHelpText}</p>
          </div>
          <Button size="sm" onClick={onRetry} disabled={!target}>
            Retry
          </Button>
        </Card>
      )}

      {/* Panel with overlays */}
      <Card
        variant="flush"
        className={`relative ${panelMinHeight} ${desktopPanelHeight} overflow-hidden`}
      >
        {/* Idle overlay */}
        {isIdle && !isConnecting && !credentialOverlay && (
          <div className="absolute inset-0 flex items-center justify-center z-10 bg-[var(--panel)]">
            <div className="flex flex-col items-center justify-center py-12 gap-2">
              <div className="text-[var(--muted)]">{emptyIcon}</div>
              <p className="text-sm font-medium text-[var(--text)]">
                No active session
              </p>
              <p className="text-xs text-[var(--muted)] text-center max-w-sm">
                {idleText ?? defaultIdleText}
              </p>
            </div>
          </div>
        )}

        {/* Connecting overlay */}
        {isConnecting && !credentialOverlay && (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 z-10 bg-[var(--panel)]">
            <div className="h-5 w-5 rounded-full border-2 border-[var(--line)] border-t-[var(--accent)] animate-spin" />
            <p className="text-sm font-medium text-[var(--text)]">Connecting</p>
            <p className="text-xs text-[var(--muted)] text-center max-w-sm">
              Connecting to {target}...
            </p>
          </div>
        )}

        {/* Credential overlay (desktop only) */}
        {credentialOverlay}

        {/* Viewer */}
        {type === "terminal" ? (
          <XTerminal
            ref={termRef}
            wsUrl={wsUrl}
            onConnected={onTerminalConnected}
            onDisconnected={onTerminalDisconnected}
            onError={onTerminalError}
            onStreamReady={onTerminalStreamReady}
            onStreamStatus={onTerminalStreamStatus}
            theme={terminalTheme}
            fontFamily={terminalFontFamily}
            fontSize={terminalFontSize}
            cursorStyle={terminalCursorStyle}
            cursorBlink={terminalCursorBlink}
            scrollback={terminalScrollback}
          />
        ) : protocol === "rdp" ? (
          <GuacamoleViewer
            ref={guacRef}
            wsUrl={wsUrl}
            onConnect={onDesktopConnect}
            onDisconnect={onDesktopDisconnect}
          />
        ) : protocol === "spice" && spiceTicket ? (
          <SPICEViewer
            ref={spiceRef}
            wsUrl={spiceTicket.wsUrl}
            password={spiceTicket.password}
            onConnect={onDesktopConnect}
            onDisconnect={onDesktopDisconnect}
          />
        ) : protocol === "webrtc" ? (
          <WebRTCViewer
            key={wsUrl ?? "webrtc-idle"}
            ref={webrtcRef}
            wsUrl={wsUrl}
            scalingMode={scalingMode}
            audioEnabled={audioEnabled}
            volume={volume}
            onStats={onWebRTCStats}
            onStream={onWebRTCStream}
            displayLayout={webrtcDisplayLayout}
            onConnect={onDesktopConnect}
            onDisconnect={onDesktopDisconnect}
          />
        ) : (
          <VNCViewer
            ref={vncRef}
            wsUrl={wsUrl}
            onConnect={onDesktopConnect}
            onDisconnect={onDesktopDisconnect}
            onError={onDesktopError}
            onCredentialsRequired={onCredentialsRequired}
            quality={quality}
            scalingMode={scalingMode}
            viewOnly={viewOnly}
          />
        )}
      </Card>
    </>
  );
}
